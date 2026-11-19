package database

import (
	"strings"
)

// PrettySQL format mot cau SQL 1 dong thanh nhieu dong de doc:
// - xuong dong truoc cac keyword chinh (SELECT / FROM / JOIN / WHERE / ORDER BY...)
// - thut le cho AND / OR / ON
// - moi cot trong select-list (va SET) mot dong
// Khong doi noi dung string literal ('...') va identifier ("...").
func PrettySQL(sql string) string {
	toks := tokenize(sql)
	if len(toks) == 0 {
		return ""
	}

	var (
		lines []string
		cur   strings.Builder
		depth int
		// listMode: dang o vung select-list / SET => break theo dau phay
		listMode bool
		prev     tok
		hasPrev  bool
	)

	flush := func() {
		if s := strings.TrimRight(cur.String(), " "); s != "" {
			lines = append(lines, s)
		}
		cur.Reset()
	}
	// write dat token/keyword vao dong hien tai, tu quyet dinh co can khoang trang.
	write := func(t tok) {
		noSpace := !hasPrev ||
			strings.TrimSpace(cur.String()) == "" ||
			t.s == "," || t.s == ")" || t.s == "." || t.s == "::" ||
			prev.s == "(" || prev.s == "." || prev.s == "::" ||
			(t.s == "(" && !prev.quoted && !needSpaceBeforeParen(prev.s))
		if !noSpace {
			cur.WriteString(" ")
		}
		cur.WriteString(t.s)
		prev, hasPrev = t, true
	}
	startLine := func(indent string) {
		flush()
		cur.WriteString(indent)
	}

	for i := 0; i < len(toks); i++ {
		t := toks[i]

		switch t.s {
		case "(":
			depth++
			write(t)
			continue
		case ")":
			if depth > 0 {
				depth--
			}
			write(t)
			continue
		}

		if depth == 0 && !t.quoted {
			// keyword chinh -> dong moi, khong thut le
			if kw, n := matchKeyword(toks[i:], mainKeywords); n > 0 {
				startLine("")
				write(tok{s: kw})
				i += n - 1
				switch kw {
				case "SELECT", "SET", "INSERT INTO", "DO UPDATE SET":
					listMode = true
				default:
					listMode = false
				}
				continue
			}
			// keyword phu -> dong moi, thut le
			if kw, n := matchKeyword(toks[i:], subKeywords); n > 0 {
				startLine(subIndent)
				write(tok{s: kw})
				i += n - 1
				listMode = false
				continue
			}
			// dau phay trong select-list / SET -> moi item mot dong
			if t.s == "," && listMode {
				write(t)
				startLine(listIndent)
				continue
			}
		}

		if !t.quoted {
			if up := strings.ToUpper(t.s); inlineKeywords[up] {
				t.s = up
			}
		}
		write(t)
	}
	flush()

	// select-list bat dau ngay sau "SELECT" -> day xuong dong rieng cho thang hang
	for i, l := range lines {
		if strings.HasPrefix(l, "SELECT ") && listMode0(lines, i) {
			lines[i] = "SELECT"
			rest := strings.TrimPrefix(l, "SELECT ")
			lines = append(lines[:i+1], append([]string{listIndent + rest}, lines[i+1:]...)...)
			break
		}
	}
	return strings.Join(lines, "\n")
}

const (
	subIndent  = "  "
	listIndent = "    "
)

// listMode0 chi tach "SELECT" ra dong rieng khi select-list that su nhieu cot.
func listMode0(lines []string, i int) bool {
	return i+1 < len(lines) && strings.HasPrefix(lines[i+1], listIndent)
}

// mainKeywords: moi phan tu la mot keyword (co the nhieu tu). Sap theo do dai
// giam dan de match tham lam ("LEFT JOIN" truoc "JOIN").
var mainKeywords = sortByWords([]string{
	"WITH", "SELECT", "INSERT INTO", "UPDATE", "DELETE FROM", "FROM", "SET", "VALUES",
	"LEFT OUTER JOIN", "RIGHT OUTER JOIN", "FULL OUTER JOIN",
	"LEFT JOIN", "RIGHT JOIN", "INNER JOIN", "CROSS JOIN", "NATURAL JOIN", "JOIN",
	"WHERE", "GROUP BY", "HAVING", "WINDOW", "ORDER BY", "LIMIT", "OFFSET",
	"RETURNING", "ON CONFLICT DO NOTHING", "ON CONFLICT", "DO UPDATE SET", "DO NOTHING",
	"UNION ALL", "UNION", "EXCEPT", "INTERSECT", "FOR UPDATE", "FOR SHARE",
})

var subKeywords = sortByWords([]string{"AND", "OR", "ON"})

// inlineKeywords: viet hoa cho de doc nhung khong xuong dong.
var inlineKeywords = map[string]bool{
	"AS": true, "ASC": true, "DESC": true, "IN": true, "IS": true, "NOT": true,
	"NULL": true, "LIKE": true, "ILIKE": true, "BETWEEN": true, "EXISTS": true,
	"CASE": true, "WHEN": true, "THEN": true, "ELSE": true, "END": true,
	"DISTINCT": true, "COUNT": true, "COALESCE": true, "NULLS": true,
	"FIRST": true, "LAST": true, "TRUE": true, "FALSE": true, "ALL": true, "ANY": true,
}

// needSpaceBeforeParen: sau nhung tu nay thi "(" can khoang trang (IN (...)),
// con sau ten ham thi khong (COALESCE(...)).
func needSpaceBeforeParen(prev string) bool {
	switch strings.ToUpper(prev) {
	case "IN", "VALUES", "AND", "OR", "ON", "NOT", "EXISTS", "ALL", "ANY", "=", "<", ">", "<=", ">=", "<>", "!=", ",", "(":
		return true
	}
	return false
}

func sortByWords(kws []string) [][]string {
	out := make([][]string, 0, len(kws))
	for _, k := range kws {
		out = append(out, strings.Fields(k))
	}
	// insertion sort theo so tu giam dan (giu on dinh, danh sach nho)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && len(out[j]) > len(out[j-1]); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

// matchKeyword tra ve keyword (da viet hoa) va so token da dung.
func matchKeyword(toks []tok, kws [][]string) (string, int) {
	for _, words := range kws {
		if len(words) > len(toks) {
			continue
		}
		ok := true
		for i, w := range words {
			if toks[i].quoted || !strings.EqualFold(toks[i].s, w) {
				ok = false
				break
			}
		}
		if ok {
			return strings.Join(words, " "), len(words)
		}
	}
	return "", 0
}

type tok struct {
	s      string
	quoted bool // string literal hoac quoted identifier => khong duoc doi
}

// tokenize cat SQL thanh token, tach rieng ( ) va dau phay, giu nguyen phan trong quote.
func tokenize(sql string) []tok {
	var (
		out []tok
		buf strings.Builder
	)
	push := func() {
		if buf.Len() > 0 {
			out = append(out, tok{s: buf.String()})
			buf.Reset()
		}
	}

	rs := []rune(sql)
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case c == '\'' || c == '"' || c == '`':
			push()
			q := c
			var lit strings.Builder
			lit.WriteRune(q)
			i++
			for i < len(rs) {
				if rs[i] == q {
					// '' hoac "" la escape
					if i+1 < len(rs) && rs[i+1] == q {
						lit.WriteRune(q)
						lit.WriteRune(q)
						i += 2
						continue
					}
					lit.WriteRune(q)
					break
				}
				if rs[i] == '\\' && i+1 < len(rs) {
					lit.WriteRune(rs[i])
					i++
				}
				lit.WriteRune(rs[i])
				i++
			}
			out = append(out, tok{s: lit.String(), quoted: true})
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			push()
		case c == '(' || c == ')' || c == ',':
			push()
			out = append(out, tok{s: string(c)})
		default:
			buf.WriteRune(c)
		}
	}
	push()
	return out
}
