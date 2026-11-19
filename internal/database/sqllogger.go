package database

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// PrettyLogger la gorm logger in SQL nhieu dong, co mau, kem thoi gian / so row /
// vi tri goi. Chi dung cho dev: gia tri tham so duoc in thang vao cau SQL nen
// log co the chua du lieu that (PII).
//
//	┌─ SQL  2.13ms  rows=10  conversation_repo.go:57
//	│ SELECT ...
//	└─
type PrettyLogger struct {
	out   io.Writer
	level gormlogger.LogLevel
	slow  time.Duration
	color bool
}

// NewPrettyLogger: out=nil => os.Stdout. slow<=0 => tat canh bao slow query.
func NewPrettyLogger(out io.Writer, level gormlogger.LogLevel, slow time.Duration) *PrettyLogger {
	if out == nil {
		out = os.Stdout
	}
	return &PrettyLogger{out: out, level: level, slow: slow, color: isTTY(out)}
}

// WithColor bat/tat mau thu cong (vi du khi ghi ra file hoac CI).
func (l *PrettyLogger) WithColor(on bool) *PrettyLogger {
	c := *l
	c.color = on
	return &c
}

func (l *PrettyLogger) LogMode(level gormlogger.LogLevel) gormlogger.Interface {
	c := *l
	c.level = level
	return &c
}

func (l *PrettyLogger) Info(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Info {
		slog.Info("gorm: " + fmt.Sprintf(msg, data...))
	}
}

func (l *PrettyLogger) Warn(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Warn {
		slog.Warn("gorm: " + fmt.Sprintf(msg, data...))
	}
}

func (l *PrettyLogger) Error(_ context.Context, msg string, data ...any) {
	if l.level >= gormlogger.Error {
		slog.Error("gorm: " + fmt.Sprintf(msg, data...))
	}
}

func (l *PrettyLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	if l.level <= gormlogger.Silent {
		return
	}
	elapsed := time.Since(begin)
	notFound := errors.Is(err, gorm.ErrRecordNotFound)

	switch {
	case err != nil && !notFound:
		if l.level < gormlogger.Error {
			return
		}
	case l.slow > 0 && elapsed > l.slow:
		if l.level < gormlogger.Warn {
			return
		}
	default:
		if l.level < gormlogger.Info {
			return
		}
	}

	sql, rows := fc()
	rowsText := "-"
	if rows >= 0 {
		rowsText = fmt.Sprintf("%d", rows)
	}

	tag, tagColor := "SQL", cCyan
	switch {
	case err != nil && !notFound:
		tag, tagColor = "SQL ERROR", cRed
	case l.slow > 0 && elapsed > l.slow:
		tag, tagColor = "SLOW SQL", cYellow
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s  %s  rows=%s  %s\n",
		l.paint("┌─", cDim),
		l.paint(tag, tagColor),
		l.paint(fmt.Sprintf("%.2fms", float64(elapsed.Nanoseconds())/1e6), cBold),
		rowsText,
		l.paint(shortCaller(), cDim),
	)
	for _, line := range strings.Split(PrettySQL(sql), "\n") {
		fmt.Fprintf(&b, "%s %s\n", l.paint("│", cDim), l.highlight(line))
	}
	if err != nil && !notFound {
		fmt.Fprintf(&b, "%s %s\n", l.paint("│", cDim), l.paint("ERR: "+err.Error(), cRed))
	}
	b.WriteString(l.paint("└─", cDim) + "\n")

	_, _ = io.WriteString(l.out, b.String())
}

// ---- mau sac ----

const (
	cReset  = "\033[0m"
	cDim    = "\033[90m"
	cBold   = "\033[1m"
	cCyan   = "\033[36;1m"
	cYellow = "\033[33;1m"
	cRed    = "\033[31;1m"
	cBlue   = "\033[34;1m"
)

func (l *PrettyLogger) paint(s, color string) string {
	if !l.color || s == "" {
		return s
	}
	return color + s + cReset
}

// highlight to mau keyword dung dau dong (PrettySQL luon dat keyword o dau dong).
func (l *PrettyLogger) highlight(line string) string {
	if !l.color {
		return line
	}
	indent := line[:len(line)-len(strings.TrimLeft(line, " "))]
	body := line[len(indent):]
	toks := tokenize(body)
	kw, n := matchKeyword(toks, mainKeywords)
	if n == 0 {
		kw, n = matchKeyword(toks, subKeywords)
	}
	if n == 0 || !strings.HasPrefix(body, kw) {
		return line
	}
	return indent + cBlue + kw + cReset + body[len(kw):]
}

// selfFile dung de bo qua chinh file nay khi tim caller.
var selfFile = func() string { _, f, _, _ := runtime.Caller(0); return f }()

// shortCaller tra ve "repository/postgres/conversation_repo.go:57": frame dau tien
// khong thuoc gorm va khong thuoc file nay. (Khong dung utils.FileWithLineNum vi
// no hard-code so frame can skip theo stack cua gorm.)
func shortCaller() string {
	pcs := make([]uintptr, 24)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		f, more := frames.Next()
		if f.File != "" && f.File != selfFile && !strings.Contains(f.File, "/gorm.io/") {
			file := f.File
			if parts := strings.Split(file, "/"); len(parts) > 3 {
				file = strings.Join(parts[len(parts)-3:], "/")
			}
			return file + ":" + strconv.Itoa(f.Line)
		}
		if !more {
			return ""
		}
	}
}

func isTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

// ---- in query ad-hoc, khong can bat log toan cuc ----

// ExplainSQL build cau SQL cua mot query ma KHONG chay query (DryRun) va tra ve
// dang da format. Dung khi chi muon xem SQL cua dung mot cho:
//
//	fmt.Println(database.ExplainSQL(r.db(ctx), func(tx *gorm.DB) *gorm.DB {
//	    var out []domain.Conversation
//	    return tx.Model(&domain.Conversation{}).Where("id = ?", id).Find(&out)
//	}))
func ExplainSQL(db *gorm.DB, fn func(tx *gorm.DB) *gorm.DB) string {
	return PrettySQL(db.ToSQL(fn))
}

// PrintSQL in ket qua cua ExplainSQL ra stdout, co khung cho de doc.
func PrintSQL(db *gorm.DB, fn func(tx *gorm.DB) *gorm.DB) {
	l := NewPrettyLogger(os.Stdout, gormlogger.Info, 0)
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s  %s\n", l.paint("┌─", cDim), l.paint("SQL", cCyan), l.paint(shortCaller(), cDim))
	for _, line := range strings.Split(ExplainSQL(db, fn), "\n") {
		fmt.Fprintf(&b, "%s %s\n", l.paint("│", cDim), l.highlight(line))
	}
	b.WriteString(l.paint("└─", cDim) + "\n")
	_, _ = io.WriteString(os.Stdout, b.String())
}
