package globalize

import (
	"math"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// object is a JavaScript ordinary object: array-index keys come first in ascending numeric
// order, every other key follows in insertion order.
type object struct {
	keys []string
	nidx int
	vals map[string]any
}

func newObject() *object {
	return &object{vals: map[string]any{}}
}

func arrayIndex(k string) (uint64, bool) {
	if k == "" || len(k) > 10 || (len(k) > 1 && k[0] == '0') {
		return 0, false
	}
	var n uint64
	for i := 0; i < len(k); i++ {
		c := k[i]
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + uint64(c-'0')
	}
	if n >= 1<<32-1 {
		return 0, false
	}
	return n, true
}

func (o *object) get(k string) (any, bool) {
	if o == nil {
		return nil, false
	}
	v, ok := o.vals[k]
	return v, ok
}

// prop is the JavaScript o[k]: undefined when the property is absent.
func (o *object) prop(k string) any {
	if v, ok := o.get(k); ok {
		return v
	}
	return Undefined
}

func (o *object) has(k string) bool {
	_, ok := o.vals[k]
	return ok
}

func (o *object) set(k string, v any) {
	if _, ok := o.vals[k]; ok {
		o.vals[k] = v
		return
	}
	o.vals[k] = v
	if n, ok := arrayIndex(k); ok {
		i := 0
		for i < o.nidx {
			m, _ := arrayIndex(o.keys[i])
			if m > n {
				break
			}
			i++
		}
		o.keys = append(o.keys, "")
		copy(o.keys[i+1:], o.keys[i:])
		o.keys[i] = k
		o.nidx++
		return
	}
	o.keys = append(o.keys, k)
}

func (o *object) del(k string) {
	if _, ok := o.vals[k]; !ok {
		return
	}
	delete(o.vals, k)
	for i, kk := range o.keys {
		if kk == k {
			o.keys = append(o.keys[:i], o.keys[i+1:]...)
			if i < o.nidx {
				o.nidx--
			}
			return
		}
	}
}

func (o *object) clone() *object {
	c := newObject()
	for _, k := range o.keys {
		c.set(k, o.vals[k])
	}
	return c
}

func toObject(o Obj) *object {
	r := newObject()
	for i := 0; i+1 < len(o); i += 2 {
		k := propertyKey(o[i])
		r.set(k, fromPublic(o[i+1]))
	}
	return r
}

func propertyKey(k any) string {
	if s, ok := k.(string); ok {
		return s
	}
	return jsString(normalizeNumber(k))
}

func normalizeNumber(v any) any {
	switch x := v.(type) {
	case int:
		return float64(x)
	case int8:
		return float64(x)
	case int16:
		return float64(x)
	case int32:
		return float64(x)
	case int64:
		return float64(x)
	case uint:
		return float64(x)
	case uint8:
		return float64(x)
	case uint16:
		return float64(x)
	case uint32:
		return float64(x)
	case uint64:
		return float64(x)
	case float32:
		return float64(x)
	}
	return v
}

// fromPublic converts an API value (Obj, []any, ...) into the internal model.
func fromPublic(v any) any {
	switch x := v.(type) {
	case Obj:
		return toObject(x)
	case *Date:
		if x == nil {
			return nil
		}
		return *x
	case []any:
		r := make([]any, len(x))
		for i, e := range x {
			r[i] = fromPublic(e)
		}
		return r
	case []string:
		r := make([]any, len(x))
		for i, e := range x {
			r[i] = e
		}
		return r
	case func(any) string:
		return x
	case func(any) []Part:
		return x
	}
	return normalizeNumber(v)
}

// toPublic converts an internal value into the API model.
func toPublic(v any) any {
	switch x := v.(type) {
	case *object:
		r := make(Obj, 0, 2*len(x.keys))
		for _, k := range x.keys {
			r = append(r, k, toPublic(x.vals[k]))
		}
		return r
	case []any:
		r := make([]any, len(x))
		for i, e := range x {
			r[i] = toPublic(e)
		}
		return r
	}
	return v
}

func isFunction(v any) bool {
	switch v.(type) {
	case func(any) string, func(any) []Part, func(...any) any, func(any) float64, func(any) *Date:
		return true
	}
	return false
}

// jsonStringify is JSON.stringify(v); ok is false when the result is undefined.
func jsonStringify(v any) (string, bool) {
	var b strings.Builder
	if !jsonWrite(&b, v) {
		return "", false
	}
	return b.String(), true
}

func jsonWrite(b *strings.Builder, v any) bool {
	switch x := v.(type) {
	case undefinedType:
		return false
	case nil:
		b.WriteString("null")
	case bool:
		if x {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			b.WriteString("null")
		} else {
			b.WriteString(numToString(x))
		}
	case string:
		b.WriteString(jsonQuote(x))
	case Date:
		if math.IsNaN(float64(x)) {
			b.WriteString("null")
		} else {
			b.WriteString(jsonQuote(dateToISOString(float64(x))))
		}
	case []any:
		b.WriteByte('[')
		for i, e := range x {
			if i > 0 {
				b.WriteByte(',')
			}
			if isFunction(e) || !jsonWrite(b, e) {
				b.WriteString("null")
			}
		}
		b.WriteByte(']')
	case *object:
		b.WriteByte('{')
		first := true
		for _, k := range x.keys {
			e := x.vals[k]
			if isFunction(e) || isUndefined(e) {
				continue
			}
			var sub strings.Builder
			if !jsonWrite(&sub, e) {
				continue
			}
			if !first {
				b.WriteByte(',')
			}
			first = false
			b.WriteString(jsonQuote(k))
			b.WriteByte(':')
			b.WriteString(sub.String())
		}
		b.WriteByte('}')
	case *Cldr:
		return jsonWrite(b, x.jsonView())
	default:
		if isFunction(v) {
			return false
		}
		b.WriteString("{}")
	}
	return true
}

// jsonQuote is JSON.stringify applied to a string (QuoteJSONString).
func jsonQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			if cu, n := decodeWTF8Surrogate(s[i:]); n > 0 {
				b.WriteString(`\u` + strconv.FormatInt(int64(cu), 16))
				i += n
				continue
			}
		}
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 {
				h := strconv.FormatInt(int64(r), 16)
				b.WriteString(`\u` + strings.Repeat("0", 4-len(h)) + h)
			} else {
				b.WriteString(s[i : i+size])
			}
		}
		i += size
	}
	b.WriteByte('"')
	return b.String()
}

// decodeWTF8Surrogate decodes a lone surrogate written in generalized UTF-8 (ED A0..BF xx).
func decodeWTF8Surrogate(s string) (uint16, int) {
	if len(s) >= 3 && s[0] == 0xED && s[1] >= 0xA0 && s[1] <= 0xBF && s[2] >= 0x80 && s[2] <= 0xBF {
		return uint16(0xD000 | uint16(s[1]&0x3F)<<6 | uint16(s[2]&0x3F)), 3
	}
	return 0, 0
}

// utf16Units returns the UTF-16 code units of s (lone surrogates in generalized UTF-8 included).
func utf16Units(s string) []uint16 {
	units := make([]uint16, 0, len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size <= 1 {
			if cu, n := decodeWTF8Surrogate(s[i:]); n > 0 {
				units = append(units, cu)
				i += n
				continue
			}
			units = append(units, 0xFFFD)
			i++
			continue
		}
		if r >= 0x10000 {
			a, c := utf16.EncodeRune(r)
			units = append(units, uint16(a), uint16(c))
		} else {
			units = append(units, uint16(r))
		}
		i += size
	}
	return units
}

// fromUnits turns UTF-16 code units back into a string; a lone surrogate is kept in
// generalized UTF-8.
func fromUnits(u []uint16) string {
	var b strings.Builder
	for i := 0; i < len(u); i++ {
		c := u[i]
		if c >= 0xD800 && c <= 0xDBFF && i+1 < len(u) && u[i+1] >= 0xDC00 && u[i+1] <= 0xDFFF {
			b.WriteRune(utf16.DecodeRune(rune(c), rune(u[i+1])))
			i++
			continue
		}
		if c >= 0xD800 && c <= 0xDFFF {
			b.WriteByte(0xED)
			b.WriteByte(byte(0x80 | (c>>6)&0x3F))
			b.WriteByte(byte(0x80 | c&0x3F))
			continue
		}
		b.WriteRune(rune(c))
	}
	return b.String()
}

// jsLength is the JavaScript string length (UTF-16 code units).
func jsLength(s string) int {
	n := 0
	for _, r := range s {
		if r >= 0x10000 {
			n += 2
		} else {
			n++
		}
	}
	return n
}

func stringIndex(s string, i int) any {
	u := utf16Units(s)
	if i < 0 || i >= len(u) {
		return Undefined
	}
	return fromUnits(u[i : i+1])
}

// getProp is the JavaScript property access v[k] on the values Globalize meets.
func getProp(v any, k string) any {
	switch x := v.(type) {
	case *object:
		return x.prop(k)
	case []any:
		if k == "length" {
			return float64(len(x))
		}
		if n, ok := arrayIndex(k); ok && int(n) < len(x) {
			return x[n]
		}
		return Undefined
	case string:
		if k == "length" {
			return float64(jsLength(x))
		}
		if n, ok := arrayIndex(k); ok {
			return stringIndex(x, int(n))
		}
		return Undefined
	}
	return Undefined
}
