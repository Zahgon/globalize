package globalize

import (
	"math"
	"regexp"
	"strings"
)

// Globalize is a Globalize instance: a locale whose formatters and parsers are built from the
// loaded CLDR data.
type Globalize struct {
	cldr *Cldr
}

// statics is the Globalize function object itself: its static methods use the default locale.
var statics = &Globalize{}

// keys identifying the JavaScript functions Globalize registers as cldr "get" listeners.
var (
	keyValidateCldr         = new(int)
	keyValidateRequiredCldr = new(int)
)

// toStr is Globalize's toString(variable) used by formatMessage.
func toStr(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case float64:
		return numToString(x)
	}
	if s, ok := jsonStringify(v); ok {
		return s
	}
	return "undefined"
}

// scanPlaceholders finds the matches of /{[0-9a-zA-Z-_. ]+}/g in s and calls fn with the byte
// offsets of each match; it returns s with each match replaced by fn's result.
func scanPlaceholders(s string, fn func(start, end int, name string) string) string {
	var b strings.Builder
	last := 0
	for i := 0; i < len(s); i++ {
		if s[i] != '{' {
			continue
		}
		j := i + 1
		for j < len(s) && isPlaceholderChar(s[j]) {
			j++
		}
		if j > i+1 && j < len(s) && s[j] == '}' {
			b.WriteString(s[last:i])
			b.WriteString(fn(i, j+1, s[i+1:j]))
			last = j + 1
			i = j
		}
	}
	b.WriteString(s[last:])
	return b.String()
}

func isPlaceholderChar(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '-' || c == '_' || c == '.' || c == ' '
}

func formatMessage(message string, data any) string {
	return scanPlaceholders(message, func(_, _ int, name string) string {
		return toStr(getProp(data, name))
	})
}

func partsPush(parts []Part, typ, value string) []Part {
	if len(parts) > 0 && parts[len(parts)-1].Type == typ {
		parts[len(parts)-1].Value += value
		return parts
	}
	return append(parts, Part{Type: typ, Value: value})
}

func partsJoin(parts []Part) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p.Value)
	}
	return b.String()
}

func createError(code, message string, attributes *object) *Error {
	if message != "" {
		message = code + ": " + formatMessage(message, attributes)
	} else {
		message = code
	}
	return &Error{Name: errorName(attributes), Message: message}
}

// errorName is error.name once Globalize or cldrjs has copied the attributes onto the Error.
func errorName(attributes *object) string {
	if v, ok := attributes.get("name"); ok {
		return jsString(v)
	}
	return "Error"
}

func validate(code, message string, check bool, attributes *object) {
	if !check {
		panic(createError(code, message, attributes))
	}
}

func attrs(kv ...any) *object { return cldrAttrs(kv...) }

var cldrSkipNone []*regexp.Regexp

func validateCldr(path string, value any, skip []*regexp.Regexp) {
	skipped := false
	for _, re := range skip {
		if re.MatchString(path) {
			skipped = true
			break
		}
	}
	validate("E_MISSING_CLDR", "Missing required CLDR content `{path}`.", jsTruthy(value) || skipped, attrs("path", path))
}

func validateCldrListener(path string, value any) { validateCldr(path, value, cldrSkipNone) }

func validateDefaultLocale(c *Cldr) {
	validate("E_DEFAULT_LOCALE_NOT_DEFINED", "Default locale has not been defined.", c != nil, newObject())
}

func validateParameterPresence(value any, name string) {
	validate("E_MISSING_PARAMETER", "Missing required parameter `{name}`.", !isUndefined(value), attrs("name", name))
}

func validateParameterRange(value any, name string, minimum, maximum any) {
	ok := isUndefined(value) || (jsCompare(value, minimum, ">=") && jsCompare(value, maximum, "<="))
	validate("E_PAR_OUT_OF_RANGE", "Parameter `{name}` has value `{value}` out of range [{minimum}, {maximum}].", ok,
		attrs("maximum", maximum, "minimum", minimum, "name", name, "value", value))
}

func validateParameterType(value any, name string, check bool, expected string) {
	validate("E_INVALID_PAR_TYPE", "Invalid `{name}` parameter ({value}). {expected} expected.", check,
		attrs("expected", expected, "name", name, "value", value))
}

func validateParameterTypeLocale(value any, name string) {
	_, isStr := value.(string)
	_, isCldr := value.(*Cldr)
	validateParameterType(value, name, isUndefined(value) || isStr || isCldr, "String or Cldr instance")
}

func validateParameterTypePlainObject(value any, name string) {
	validateParameterType(value, name, isUndefined(value) || isPlainObject(value), "Plain Object")
}

func validateParameterTypeNumber(value any, name string) {
	_, ok := value.(float64)
	validateParameterType(value, name, isUndefined(value) || ok, "Number")
}

func validateParameterTypeString(value any, name string) {
	_, ok := value.(string)
	validateParameterType(value, name, isUndefined(value) || ok, "a string")
}

// jsToPrimitive converts objects the way the relational operators do (hint number).
func jsToPrimitive(v any) any {
	switch x := v.(type) {
	case *object, []any, *Cldr:
		return jsString(x)
	case Date:
		return float64(x)
	}
	return v
}

// jsCompare is the JavaScript relational comparison a op b for op in <, <=, >, >=.
func jsCompare(a, b any, op string) bool {
	pa, pb := jsToPrimitive(a), jsToPrimitive(b)
	sa, aStr := pa.(string)
	sb, bStr := pb.(string)
	if aStr && bStr {
		ua, ub := utf16Units(sa), utf16Units(sb)
		cmp := 0
		for i := 0; i < len(ua) && i < len(ub); i++ {
			if ua[i] != ub[i] {
				if ua[i] < ub[i] {
					cmp = -1
				} else {
					cmp = 1
				}
				break
			}
		}
		if cmp == 0 {
			switch {
			case len(ua) < len(ub):
				cmp = -1
			case len(ua) > len(ub):
				cmp = 1
			}
		}
		switch op {
		case "<":
			return cmp < 0
		case "<=":
			return cmp <= 0
		case ">":
			return cmp > 0
		}
		return cmp >= 0
	}
	x, y := toNumber(pa), toNumber(pb)
	if math.IsNaN(x) || math.IsNaN(y) {
		return false
	}
	switch op {
	case "<":
		return x < y
	case "<=":
		return x <= y
	case ">":
		return x > y
	}
	return x >= y
}

func alwaysCldr(localeOrCldr any) *Cldr {
	if c, ok := localeOrCldr.(*Cldr); ok {
		return c
	}
	return newCldr(localeOrCldr)
}

var regexpEscapeRe = regexp.MustCompile(`([.*+?^=!:${}()|\[\]/\\])`)

func regexpEscape(s string) string {
	return regexpEscapeRe.ReplaceAllString(s, `\$1`)
}

// stringPad is Globalize's stringPad(str, count, right).
func stringPad(str any, count any, right bool) string {
	s, ok := str.(string)
	if !ok {
		s = jsString(str)
	}
	for length := jsLength(s); jsCompare(float64(length), count, "<"); length++ {
		if right {
			s += "0"
		} else {
			s = "0" + s
		}
	}
	return s
}

func validateLikelySubtags(c *Cldr) {
	c.once(keyValidateCldr, validateCldrListener)
	c.get("supplemental/likelySubtags")
}

// New is new Globalize(locale) (or Globalize(locale)): locale is a string or a *Cldr.
func New(locale any) *Globalize {
	locale = fromPublic(locale)
	validateParameterPresence(locale, "locale")
	validateParameterTypeLocale(locale, "locale")
	g := &Globalize{cldr: alwaysCldr(locale)}
	validateLikelySubtags(g.cldr)
	return g
}

// Cldr returns the instance's cldr property.
func (g *Globalize) Cldr() *Cldr { return g.cldr }

// Load is Globalize.load(json, ...): it merges CLDR JSON data into the shared data set.
func Load(json ...any) {
	args := make([]any, len(json))
	for i, j := range json {
		args[i] = fromPublic(j)
	}
	cldrLoad(args)
}

// Locale is Globalize.locale([locale]): with an argument it sets the default locale (a string
// or a *Cldr); it returns the default locale's Cldr instance, nil when none is set.
func Locale(locale ...any) *Cldr {
	var arg any = Undefined
	if len(locale) > 0 {
		arg = fromPublic(locale[0])
	}
	validateParameterTypeLocale(arg, "locale")
	if len(locale) > 0 {
		statics.cldr = alwaysCldr(arg)
		validateLikelySubtags(statics.cldr)
	}
	return statics.cldr
}

// opt returns the first optional argument converted to the internal model, or undefined.
func opt(args []any) any {
	if len(args) == 0 {
		return Undefined
	}
	return fromPublic(args[0])
}

// optionsObject is `options = options || {}`.
func optionsObject(options any) *object {
	if o, ok := options.(*object); ok {
		return o
	}
	if !jsTruthy(options) {
		return newObject()
	}
	return arrayAsOptions(options)
}

// arrayAsOptions models an array that passed the plain-object check ([{...}]): property reads on
// it see only its index keys.
func arrayAsOptions(v any) *object {
	o := newObject()
	forIn(v, func(k string, val any) { o.set(k, val) })
	return o
}
