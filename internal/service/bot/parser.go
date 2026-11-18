// Package bot xu ly lenh "@bot ..." trong tin nhan va luu "mau" (form) xuong DB.
package bot

import (
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/abc-workspace/chat-service/internal/domain"
)

// mentionRe khop "@bot", "@bot-design", "@bot_v2" o dau tin nhan.
var mentionRe = regexp.MustCompile(`^\s*@([a-zA-Z0-9][a-zA-Z0-9._-]{0,63})\b\s*`)

// Mention la ket qua parse mot dong lenh goi bot.
type Mention struct {
	Slug string // "bot"
	Body string // phan con lai sau "@bot"
}

// ParseMention tra (mention, true) neu tin nhan bat dau bang @slug.
func ParseMention(content string) (Mention, bool) {
	m := mentionRe.FindStringSubmatch(content)
	if m == nil {
		return Mention{}, false
	}
	return Mention{
		Slug: strings.ToLower(m[1]),
		Body: strings.TrimSpace(content[len(m[0]):]),
	}, true
}

var templateCodeRe = regexp.MustCompile(`^([a-zA-Z0-9][a-zA-Z0-9._-]{0,63})`)

// kvRe tach "key=value" hoac `key="value co dau cach"`.
var kvRe = regexp.MustCompile(`([a-zA-Z0-9_.\-]+)\s*=\s*("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*'|[^\s]+)`)

// ParseBody ho tro 2 cu phap:
//
//	@bot yeu_cau so_luong=3 to=design ghi_chu="gap trong hom nay"
//	@bot yeu_cau {"so_luong":3,"to":"design"}
//
// Tra ve template code + payload (map). Neu khong co code => code = "".
func ParseBody(body string) (code string, payload domain.JSONMap, rawArgs string) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", nil, ""
	}

	if m := templateCodeRe.FindString(body); m != "" {
		code = strings.ToLower(m)
		rawArgs = strings.TrimSpace(body[len(m):])
	} else {
		rawArgs = body
	}

	if rawArgs == "" {
		return code, nil, ""
	}

	// Cu phap JSON.
	if strings.HasPrefix(rawArgs, "{") {
		var m map[string]any
		if err := json.Unmarshal([]byte(rawArgs), &m); err == nil {
			return code, domain.JSONMap(m), rawArgs
		}
	}

	// Cu phap key=value.
	payload = domain.JSONMap{}
	for _, match := range kvRe.FindAllStringSubmatch(rawArgs, -1) {
		key := match[1]
		val := strings.Trim(match[2], `"'`)
		payload[key] = coerce(val)
	}
	if len(payload) == 0 {
		// Khong parse duoc field nao -> coi nhu ghi chu tu do.
		payload = domain.JSONMap{"note": rawArgs}
	}
	return code, payload, rawArgs
}

// coerce doi string sang kieu hop ly (so/bool) de luu jsonb dung kieu.
func coerce(s string) any {
	if s == "" {
		return ""
	}
	if b, err := strconv.ParseBool(s); err == nil && (s == "true" || s == "false") {
		return b
	}
	if i, err := strconv.ParseInt(s, 10, 64); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return s
}
