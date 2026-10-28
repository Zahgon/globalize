package globalize

import (
	"regexp"
	"strings"
)

var unitCategories = []string{"acceleration", "angle", "area", "digital", "duration", "length", "mass", "power",
	"pressure", "speed", "temperature", "volume"}

var unitPatternCountRe = regexp.MustCompile(`unitPattern-count-`)

func stripPluralGarbage(data any) any {
	if !jsTruthy(data) {
		return Undefined
	}
	aux := newObject()
	forIn(data, func(k string, v any) {
		if loc := unitPatternCountRe.FindStringIndex(k); loc != nil {
			k = k[:loc[0]] + k[loc[1]:]
		}
		aux.set(k, v)
	})
	return aux
}

func unitGet(unit string, form any, c *Cldr) any {
	unit = strings.Replace(unit, "/", "-per-", 1)
	var ret any = Undefined
	for _, category := range append([]string{""}, unitCategories...) {
		key := unit
		if category != "" {
			key = category + "-" + unit
		}
		ret = c.main([]any{"units", form, key})
		if jsTruthy(ret) {
			break
		}
	}
	ret = stripPluralGarbage(ret)
	if !jsTruthy(ret) && strings.Contains(unit, "-per-") {
		pieces := strings.Split(unit, "-per-")
		arr := make([]any, len(pieces))
		for i, p := range pieces {
			arr[i] = unitGet(p, form, c)
		}
		if !jsTruthy(arr[0]) || !jsTruthy(arr[1]) {
			return Undefined
		}
		return arr
	}
	return ret
}

type unitProps struct {
	compoundUnitPattern any
	unitProperties      any
}

// jsTrim is String.prototype.trim.
func jsTrim(s string) string {
	return strings.TrimFunc(s, isStrWhiteSpace)
}

func callNumberFormatter(f any, value any) string {
	if fn, ok := f.(func(any) string); ok {
		return fn(value)
	}
	throwTypeError("numberFormatter is not a function")
	return ""
}

func unitFormat(value any, numberFormatter any, plural func(any) string, p *unitProps) string {
	formatted := callNumberFormatter(numberFormatter, value)
	pluralValue := plural(value)
	if arr, ok := p.unitProperties.([]any); ok {
		dividendProps, divisorProps := arr[0], arr[1]
		oneProperty := "other"
		if o, isObj := divisorProps.(*object); isObj && o.has("one") {
			oneProperty = "one"
		}
		dividend := formatMessage(requireString(propOf(dividendProps, pluralValue), "message", "replace"), []any{formatted})
		divisor := jsTrim(formatMessage(requireString(propOf(divisorProps, oneProperty), "message", "replace"), []any{""}))
		return formatMessage(requireString(p.compoundUnitPattern, "message", "replace"), []any{dividend, divisor})
	}
	message := propOf(p.unitProperties, pluralValue)
	return formatMessage(requireString(message, "message", "replace"), []any{formatted})
}

func (g *Globalize) unitFormatter(unit, options any) func(any) string {
	validateParameterPresence(unit, "unit")
	validateParameterTypeString(unit, "unit")
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	form := o.prop("form")
	if !jsTruthy(form) {
		form = "long"
	}
	c := g.cldr
	if c == nil {
		throwTypeError("Cannot read properties of undefined (reading 'main')")
	}
	p := &unitProps{
		compoundUnitPattern: c.main([]any{"units", form, "per/compoundUnitPattern"}),
	}
	p.unitProperties = unitGet(unit.(string), form, c)
	var nf any = o.prop("numberFormatter")
	if !jsTruthy(nf) {
		nf = g.numberFormatter(Undefined)
	}
	plural := g.pluralGenerator(Undefined)
	return func(value any) string {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeNumber(value, "value")
		return unitFormat(value, nf, plural, p)
	}
}

// UnitFormatter is .unitFormatter(unit[, options]). options.numberFormatter, when given, is a
// formatter such as the one NumberFormatter returns.
func (g *Globalize) UnitFormatter(unit any, options ...any) func(value any) string {
	return g.unitFormatter(fromPublic(unit), opt(options))
}

// FormatUnit is .formatUnit(value, unit[, options]).
func (g *Globalize) FormatUnit(value, unit any, options ...any) string {
	v := fromPublic(value)
	validateParameterPresence(v, "value")
	validateParameterTypeNumber(v, "value")
	return g.unitFormatter(fromPublic(unit), opt(options))(value)
}

// UnitFormatter is Globalize.unitFormatter(unit[, options]) on the default locale.
func UnitFormatter(unit any, options ...any) func(value any) string {
	return statics.UnitFormatter(unit, options...)
}

// FormatUnit is Globalize.formatUnit(value, unit[, options]).
func FormatUnit(value, unit any, options ...any) string {
	return statics.FormatUnit(value, unit, options...)
}
