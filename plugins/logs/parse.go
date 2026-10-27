package logs

import (
	"bufio"
	"encoding/json"
	"strconv"
	"strings"
)

// Entry is one journal line of a GoDeb unit.
type Entry struct {
	Time     int64   `json:"time"` // unix milliseconds
	Priority int     `json:"priority"`
	Unit     string  `json:"unit"`
	Message  string  `json:"message"`
	Fields   []Field `json:"fields"`
}

// Field is a key=value pair of a structured (slog) line, in log order.
type Field struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// ParseJournal reads `journalctl --output=json`, one object per line.
// Lines that are not valid JSON are skipped.
func ParseJournal(out string) []Entry {
	entries := []Entry{}
	sc := bufio.NewScanner(strings.NewReader(out))
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		var raw map[string]json.RawMessage
		if json.Unmarshal(sc.Bytes(), &raw) != nil {
			continue
		}
		e := Entry{Priority: 6}
		if us, err := strconv.ParseInt(jsonString(raw["__REALTIME_TIMESTAMP"]), 10, 64); err == nil {
			e.Time = us / 1000
		}
		if p, err := strconv.Atoi(jsonString(raw["PRIORITY"])); err == nil {
			e.Priority = p
		}
		e.Unit = jsonString(raw["_SYSTEMD_UNIT"])
		msg := jsonString(raw["MESSAGE"])
		e.Message, e.Fields = splitStructured(msg)
		entries = append(entries, e)
	}
	return entries
}

// jsonString decodes a journal field. Non-UTF-8 messages come as a byte
// array; those are decoded byte by byte.
func jsonString(v json.RawMessage) string {
	if len(v) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s
	}
	var b []byte
	var ints []int
	if json.Unmarshal(v, &ints) == nil {
		for _, i := range ints {
			b = append(b, byte(i))
		}
		return strings.ToValidUTF8(string(b), "�")
	}
	return ""
}

// splitStructured turns an slog text line into its message and fields.
// level, msg and komponente are dropped from the fields (priority and unit
// carry them). Anything that is not logfmt is returned as the message.
func splitStructured(line string) (string, []Field) {
	pairs, ok := parseLogfmt(line)
	if !ok {
		// Plain lines (systemd's "Started …", panics) have no fields. An
		// empty list, not null: the UI iterates it.
		return line, []Field{}
	}
	msg := ""
	fields := make([]Field, 0, len(pairs))
	for _, p := range pairs {
		switch p.Key {
		case "msg":
			msg = p.Value
		case "level", "komponente", "time":
		default:
			fields = append(fields, p)
		}
	}
	return msg, fields
}

// parseLogfmt parses key=value pairs as written by slog.TextHandler. A line
// is only accepted when it starts with level= or time=, so free text that
// happens to contain "=" stays a plain message.
func parseLogfmt(line string) ([]Field, bool) {
	if !strings.HasPrefix(line, "level=") && !strings.HasPrefix(line, "time=") {
		return nil, false
	}
	var out []Field
	s := line
	for {
		s = strings.TrimLeft(s, " ")
		if s == "" {
			return out, true
		}
		eq := strings.IndexByte(s, '=')
		if eq <= 0 || strings.ContainsAny(s[:eq], " \"") {
			return nil, false
		}
		key := s[:eq]
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := closingQuote(s)
			if end < 0 {
				return nil, false
			}
			v, err := strconv.Unquote(s[:end+1])
			if err != nil {
				return nil, false
			}
			val, s = v, s[end+1:]
		} else {
			sp := strings.IndexByte(s, ' ')
			if sp < 0 {
				sp = len(s)
			}
			val, s = s[:sp], s[sp:]
		}
		out = append(out, Field{Key: key, Value: val})
	}
}

// closingQuote returns the index of the quote that ends the Go-quoted
// string at the start of s, or -1.
func closingQuote(s string) int {
	for i := 1; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

// Filter keeps entries containing text (case-insensitive, message and field
// values), limits them to the newest n and returns them newest first.
func Filter(entries []Entry, text string, n int) []Entry {
	needle := strings.ToLower(text)
	out := make([]Entry, 0, min(len(entries), n))
	for i := len(entries) - 1; i >= 0 && len(out) < n; i-- {
		if needle == "" || matches(entries[i], needle) {
			out = append(out, entries[i])
		}
	}
	return out
}

func matches(e Entry, needle string) bool {
	if strings.Contains(strings.ToLower(e.Message), needle) {
		return true
	}
	for _, f := range e.Fields {
		if strings.Contains(strings.ToLower(f.Key+"="+f.Value), needle) {
			return true
		}
	}
	return false
}

// sampleJournal is shown when journalctl is not available.
const sampleJournal = `{"__REALTIME_TIMESTAMP":"1761170000000000","PRIORITY":"6","_SYSTEMD_UNIT":"godeb-agent.service","MESSAGE":"level=INFO msg=\"GoDeb-Agent lauscht\" komponente=agent version=0.1.0 socket=/run/godeb-agent/agent.sock web_benutzer=godeb"}
{"__REALTIME_TIMESTAMP":"1761170001000000","PRIORITY":"6","_SYSTEMD_UNIT":"godeb.service","MESSAGE":"level=INFO msg=\"GoDeb lauscht\" komponente=web adresse=10.8.0.1:8080"}
{"__REALTIME_TIMESTAMP":"1761170060000000","PRIORITY":"4","_SYSTEMD_UNIT":"godeb.service","MESSAGE":"level=WARN msg=\"Anmeldung fehlgeschlagen\" komponente=web benutzer=admin ip=203.0.113.9"}
{"__REALTIME_TIMESTAMP":"1761170120000000","PRIORITY":"6","_SYSTEMD_UNIT":"godeb.service","MESSAGE":"level=INFO msg=\"Anmeldung erfolgreich\" komponente=web benutzer=admin ip=10.8.0.5"}
{"__REALTIME_TIMESTAMP":"1761170180000000","PRIORITY":"6","_SYSTEMD_UNIT":"godeb-agent.service","MESSAGE":"level=INFO msg=Aktion komponente=agent methode=POST pfad=/api/plugins/wireguard/peers status=200 dauer=412ms benutzer=admin ip=10.8.0.5 2fa=bestätigt"}
{"__REALTIME_TIMESTAMP":"1761170240000000","PRIORITY":"4","_SYSTEMD_UNIT":"godeb-agent.service","MESSAGE":"level=WARN msg=\"Befehl fehlgeschlagen\" komponente=agent befehl=\"[systemctl restart enshrouded.service]\" exit=1 dauer=87ms benutzer=admin fehler=\"Job for enshrouded.service failed.\""}
{"__REALTIME_TIMESTAMP":"1761170241000000","PRIORITY":"3","_SYSTEMD_UNIT":"godeb-agent.service","MESSAGE":"level=ERROR msg=\"Aktion fehlgeschlagen\" komponente=agent methode=POST pfad=/api/plugins/systemd/action status=502 dauer=95ms benutzer=admin ip=10.8.0.5"}
`
