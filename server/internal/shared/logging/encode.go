package logging

import (
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"
)

const timeLayout = "2006-01-02T15:04:05.000000Z07:00"

// appendEntry appends one line: time, level and msg, then the fields, as key=value pairs.
func appendEntry(b []byte, e Entry) []byte {
	b = append(b, "time="...)
	b = e.Time.UTC().AppendFormat(b, timeLayout)
	b = append(b, " level="...)
	b = append(b, e.Level.String()...)
	b = append(b, " msg="...)
	b = appendString(b, e.Message)
	for i := 0; i < len(e.Fields); i += 2 {
		b = append(b, ' ')
		b = appendKey(b, e.Fields[i])
		b = append(b, '=')
		if i+1 < len(e.Fields) {
			b = appendValue(b, e.Fields[i+1])
		} else {
			b = append(b, "!MISSING"...)
		}
	}
	return append(b, '\n')
}

// appendKey writes a key on one token: a key that is not a string, or has a space, `=` or `"`, is
// made safe instead of breaking the line.
func appendKey(b []byte, k any) []byte {
	s, ok := k.(string)
	if !ok {
		s = "!BADKEY:" + fmt.Sprint(k)
	}
	if s == "" {
		return append(b, "!EMPTYKEY"...)
	}
	for _, r := range s {
		if r <= ' ' || r == '=' || r == '"' || r == utf8.RuneError || r == 0x7f {
			r = '_'
		}
		b = utf8.AppendRune(b, r)
	}
	return b
}

func appendValue(b []byte, v any) []byte {
	switch x := v.(type) {
	case nil:
		return append(b, "null"...)
	case string:
		return appendString(b, x)
	case error:
		return appendString(b, x.Error())
	case bool:
		return strconv.AppendBool(b, x)
	case int:
		return strconv.AppendInt(b, int64(x), 10)
	case int8:
		return strconv.AppendInt(b, int64(x), 10)
	case int16:
		return strconv.AppendInt(b, int64(x), 10)
	case int32:
		return strconv.AppendInt(b, int64(x), 10)
	case int64:
		return strconv.AppendInt(b, x, 10)
	case uint:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint8:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint16:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint32:
		return strconv.AppendUint(b, uint64(x), 10)
	case uint64:
		return strconv.AppendUint(b, x, 10)
	case float32:
		return strconv.AppendFloat(b, float64(x), 'g', -1, 32)
	case float64:
		return strconv.AppendFloat(b, x, 'g', -1, 64)
	case time.Duration:
		return append(b, x.String()...)
	case time.Time:
		return x.UTC().AppendFormat(b, timeLayout)
	}
	return appendString(b, fmt.Sprint(v))
}

// appendString writes s as it is when it is a plain token, and quoted otherwise: empty, or holding a
// space, `=`, `"`, a control character or bytes that are not UTF-8.
func appendString(b []byte, s string) []byte {
	if needsQuote(s) {
		return strconv.AppendQuote(b, s)
	}
	return append(b, s...)
}

func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && n == 1 || r <= ' ' || r == '=' || r == '"' || r == 0x7f {
			return true
		}
		i += n
	}
	return false
}
