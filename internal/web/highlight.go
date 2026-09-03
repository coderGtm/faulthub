// Report-page display helpers: ACRA device-clock dates and lightweight,
// dependency-free syntax highlighting for stack traces and raw JSON.
//
// The highlighters escape all input before wrapping tokens in spans, so the
// returned HTML is safe to inject as template.HTML by construction.
package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html"
	"html/template"
	"strings"
	"time"
)

// parseDeviceTime parses ACRA device-clock dates such as
// "Thu Sep  3 09:58:00 GMT+05:30 2026". The GMT±hh:mm zone is not understood
// by Go's MST layout, so the GMT prefix is stripped before parsing.
func parseDeviceTime(s string) (time.Time, bool) {
	if t, err := time.Parse("Mon Jan _2 15:04:05 MST 2006", s); err == nil {
		return t, true
	}
	n := strings.ReplaceAll(strings.ReplaceAll(s, "GMT+", "+"), "GMT-", "-")
	for _, layout := range []string{
		"Mon Jan _2 15:04:05 -07:00 2006",
		"Mon Jan _2 15:04:05 -0700 2006",
	} {
		if t, err := time.Parse(layout, n); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// fmtDeviceTime renders an ACRA device-clock date like the received time
// ("Sep 3, 2026 09:58 GMT+05:30") plus a placeholder span that client-side JS
// fills with the browser-local equivalent (" (local: …)"). Unparseable input
// falls back to escaped raw text.
func fmtDeviceTime(s string) template.HTML {
	if strings.TrimSpace(s) == "" {
		return "—"
	}
	t, ok := parseDeviceTime(s)
	if !ok {
		return template.HTML(html.EscapeString(s))
	}
	zone := "UTC"
	if _, off := t.Zone(); off != 0 {
		sign := "+"
		if off < 0 {
			sign = "-"
			off = -off
		}
		zone = fmt.Sprintf("GMT%s%02d:%02d", sign, off/3600, (off%3600)/60)
	}
	text := t.Format("Jan 2, 2006 15:04 ") + zone
	rfc := t.Format(time.RFC3339)
	var b strings.Builder
	b.WriteString(`<time class="tz-device" datetime="`)
	b.WriteString(rfc)
	b.WriteString(`">`)
	b.WriteString(html.EscapeString(text))
	b.WriteString(`</time><span class="tz-local" data-dt="`)
	b.WriteString(rfc)
	b.WriteString(`"></span>`)
	return template.HTML(b.String())
}

// hlStack highlights a Java stack trace: the exception header, "Caused by"
// lines, "at …" frames (method vs file:line), and "... N more" lines.
func hlStack(s string) template.HTML {
	if strings.TrimSpace(s) == "" {
		return `<span class="muted">(no stack trace)</span>`
	}
	lines := strings.Split(s, "\n")
	var b strings.Builder
	headerDone := false
	for i, ln := range lines {
		if i > 0 {
			b.WriteByte('\n')
		}
		trimmed := strings.TrimSpace(ln)
		esc := html.EscapeString(ln)
		switch {
		case strings.HasPrefix(trimmed, "Caused by:"):
			b.WriteString(`<span class="tok-ex">` + esc + `</span>`)
		case strings.HasPrefix(trimmed, "at "):
			b.WriteString(hlFrame(ln))
		case strings.HasPrefix(trimmed, "..."):
			b.WriteString(`<span class="muted">` + esc + `</span>`)
		case !headerDone && trimmed != "":
			b.WriteString(`<span class="tok-ex">` + esc + `</span>`)
			headerDone = true
		default:
			b.WriteString(esc)
		}
	}
	return template.HTML(b.String())
}

// hlFrame highlights one "at com.foo.Bar.method(File.kt:42)" line.
func hlFrame(ln string) string {
	rest := strings.TrimSpace(ln)[len("at "):]
	open := strings.LastIndex(rest, "(")
	if open < 0 {
		return `<span class="tok-frame">at </span><span class="tok-fn">` +
			html.EscapeString(rest) + `</span>`
	}
	method, loc := rest[:open], rest[open:]
	return `<span class="tok-frame">at </span><span class="tok-fn">` +
		html.EscapeString(method) + `</span><span class="tok-loc">` +
		html.EscapeString(loc) + `</span>`
}

// hlJSON pretty-prints valid JSON and highlights keys, strings, numbers,
// booleans and null. Invalid input is rendered escaped as-is.
func hlJSON(s string) template.HTML {
	if strings.TrimSpace(s) == "" {
		return `<span class="muted">(empty)</span>`
	}
	src := s
	if json.Valid([]byte(s)) {
		var buf bytes.Buffer
		if err := json.Indent(&buf, []byte(s), "", "  "); err == nil {
			src = buf.String()
		}
	}
	return template.HTML(highlightJSON(src))
}

// highlightJSON tokenizes src (already pretty or raw) into spans.
func highlightJSON(src string) string {
	var b strings.Builder
	i := 0
	for i < len(src) {
		c := src[i]
		switch {
		case c == '"':
			j := i + 1
			for j < len(src) {
				if src[j] == '\\' {
					j += 2
					continue
				}
				if src[j] == '"' {
					break
				}
				j++
			}
			if j >= len(src) { // unterminated: escape the rest plain
				b.WriteString(html.EscapeString(src[i:]))
				i = len(src)
				continue
			}
			lit := src[i : j+1]
			k := j + 1
			for k < len(src) && (src[k] == ' ' || src[k] == '\t' || src[k] == '\n' || src[k] == '\r') {
				k++
			}
			cls := "tok-str"
			if k < len(src) && src[k] == ':' {
				cls = "tok-key"
			}
			b.WriteString(`<span class="` + cls + `">` + html.EscapeString(lit) + `</span>`)
			i = j + 1
		case c == '{' || c == '}' || c == '[' || c == ']' || c == ':' || c == ',':
			b.WriteString(html.EscapeString(src[i : i+1]))
			i++
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			b.WriteByte(c)
			i++
		default:
			j := i
			for j < len(src) && !strings.ContainsRune(`{}[]:, "`, rune(src[j])) &&
				src[j] != ' ' && src[j] != '\t' && src[j] != '\n' && src[j] != '\r' {
				j++
			}
			word := src[i:j]
			cls := ""
			switch word {
			case "true", "false":
				cls = "tok-bool"
			case "null":
				cls = "tok-null"
			default:
				if isJSONNumber(word) {
					cls = "tok-num"
				}
			}
			esc := html.EscapeString(word)
			if cls == "" {
				b.WriteString(esc)
			} else {
				b.WriteString(`<span class="` + cls + `">` + esc + `</span>`)
			}
			i = j
		}
	}
	return b.String()
}

func isJSONNumber(s string) bool {
	if s == "" {
		return false
	}
	i := 0
	if s[0] == '-' {
		i = 1
	}
	if i >= len(s) {
		return false
	}
	if s[i] == '0' {
		i++
	} else if s[i] >= '1' && s[i] <= '9' {
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	} else {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if i >= len(s) || s[i] < '0' || s[i] > '9' {
			return false
		}
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
		}
	}
	return i == len(s)
}
