package logger

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync/atomic"
	"time"
)

// Entry: 1 dong log da phang hoa (attr cua With / WithGroup / record gop chung,
// nhom long nhau noi bang dau cham: "a.b").
type Entry struct {
	Time  time.Time
	Level slog.Level
	Msg   string
	Attrs map[string]any
}

// Sink nhan ban sao cua moi dong log (vd ghi vao DB). Write KHONG duoc chan:
// handler goi Write tren goroutine dang log.
//
// Moi chuoi trong Entry da duoc SAO CHEP: Fiber tra c.Path(), c.Method()... la chuoi
// tro vao buffer se bi dung lai cho request sau — sink xu ly bat dong bo ma giu
// nguyen chuoi do thi se doc phai du lieu cua request khac.
type Sink interface {
	Write(e Entry)
}

type sinkState struct {
	sink Sink
	min  slog.Level
}

// sinkHolder dung chung cho moi handler con (With/WithGroup) cua cung 1 logger.
type sinkHolder struct{ cur atomic.Pointer[sinkState] }

// global: Setup tao handler goc truoc khi co DB; AttachSink gan sink sau.
var global = &sinkHolder{}

// AttachSink: tu nay moi dong log >= min con duoc gui them sang s
// (stdout / LOG_FILE van y nguyen). s = nil de go.
func AttachSink(s Sink, min slog.Level) {
	if s == nil {
		global.cur.Store(nil)
		return
	}
	global.cur.Store(&sinkState{sink: s, min: min})
}

// ParseLevel doi chuoi level (debug|info|warn|error) sang slog.Level, mac dinh info.
func ParseLevel(level string) slog.Level { return parseLevel(level) }

// teeHandler boc handler goc: ghi nhu cu roi gui them 1 ban sang sink (neu co).
type teeHandler struct {
	base   slog.Handler
	holder *sinkHolder
	attrs  []slog.Attr // attr da gan qua With, da kem tien to nhom
	prefix string      // nhom hien tai ("a.b.")
}

func newTee(base slog.Handler, holder *sinkHolder) *teeHandler {
	return &teeHandler{base: base, holder: holder}
}

func (h *teeHandler) Enabled(ctx context.Context, l slog.Level) bool {
	if h.base.Enabled(ctx, l) {
		return true
	}
	st := h.holder.cur.Load()
	return st != nil && l >= st.min
}

func (h *teeHandler) Handle(ctx context.Context, r slog.Record) error {
	var err error
	if h.base.Enabled(ctx, r.Level) {
		err = h.base.Handle(ctx, r)
	}
	st := h.holder.cur.Load()
	if st == nil || r.Level < st.min {
		return err
	}
	attrs := make(map[string]any, len(h.attrs)+r.NumAttrs()+1)
	for _, a := range h.attrs {
		flatten(attrs, "", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		flatten(attrs, h.prefix, a)
		return true
	})
	// request_id gan qua context (logger.FromContext) da nam trong attrs; con
	// log goi thang slog.InfoContext(ctx, ...) thi lay tu ctx.
	if _, ok := attrs["request_id"]; !ok {
		if id := RequestID(ctx); id != "" {
			attrs["request_id"] = strings.Clone(id)
		}
	}
	st.sink.Write(Entry{Time: r.Time, Level: r.Level, Msg: strings.Clone(r.Message), Attrs: attrs})
	return err
}

func (h *teeHandler) WithAttrs(as []slog.Attr) slog.Handler {
	next := *h
	next.base = h.base.WithAttrs(as)
	next.attrs = append(append([]slog.Attr{}, h.attrs...), prefixed(h.prefix, as)...)
	return &next
}

func (h *teeHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	next := *h
	next.base = h.base.WithGroup(name)
	next.prefix = h.prefix + name + "."
	return &next
}

func prefixed(prefix string, as []slog.Attr) []slog.Attr {
	if prefix == "" {
		return as
	}
	out := make([]slog.Attr, len(as))
	for i, a := range as {
		out[i] = slog.Attr{Key: prefix + a.Key, Value: a.Value}
	}
	return out
}

// flatten ghi attr vao map; group long nhau thanh khoa "a.b".
func flatten(dst map[string]any, prefix string, a slog.Attr) {
	v := a.Value.Resolve()
	if v.Kind() == slog.KindGroup {
		p := prefix
		if a.Key != "" {
			p = prefix + a.Key + "."
		}
		for _, ga := range v.Group() {
			flatten(dst, p, ga)
		}
		return
	}
	if a.Key == "" {
		return
	}
	key := strings.Clone(prefix + a.Key)
	switch v.Kind() {
	case slog.KindString:
		dst[key] = strings.Clone(v.String())
	case slog.KindAny:
		switch x := v.Any().(type) {
		case error:
			dst[key] = x.Error()
		case []byte:
			dst[key] = string(x) // string() da sao chep
		case fmt.Stringer:
			dst[key] = x.String()
		default:
			dst[key] = x
		}
	case slog.KindDuration:
		dst[key] = v.Duration().String()
	case slog.KindTime:
		dst[key] = v.Time()
	default:
		dst[key] = v.Any()
	}
}
