package globalize

import (
	"math"
	"regexp"
)

var relativeTypeKeyRe = regexp.MustCompile(`relative-type-(-?[0-9]+)`)

func relativeTimeProperties(unit string, c *Cldr, options any) *object {
	if form := getProp(options, "form"); jsTruthy(form) {
		unit = unit + "-" + jsString(form)
	}
	raw := c.main([]any{"dates", "fields", unit})
	p := newObject()
	p.set("relativeTime-type-future", propOf(raw, "relativeTime-type-future"))
	p.set("relativeTime-type-past", propOf(raw, "relativeTime-type-past"))
	forIn(raw, func(k string, v any) {
		if relativeTypeKeyRe.MatchString(k) {
			p.set(k, v)
		}
	})
	return p
}

func relativeTimeFormat(value float64, numberFormatter func(any) string, plural func(any) string, p *object) string {
	if message := p.prop("relative-type-" + numToString(value)); jsTruthy(message) {
		return jsString(message)
	}
	relativeTime := p.prop("relativeTime-type-future")
	if value <= 0 {
		relativeTime = p.prop("relativeTime-type-past")
	}
	value = math.Abs(value)
	key := "relativeTimePattern-count-" + plural(value)
	message := propOf(relativeTime, key)
	return formatMessage(requireString(message, "message", "replace"), []any{numberFormatter(value)})
}

func (g *Globalize) relativeTimeFormatter(unit, options any) func(any) string {
	validateParameterPresence(unit, "unit")
	validateParameterTypeString(unit, "unit")
	c := g.cldr
	if !jsTruthy(options) {
		options = newObject()
	}
	validateDefaultLocale(c)
	var p *object
	func() {
		c.on(keyValidateCldr, validateCldrListener)
		defer c.off(keyValidateCldr)
		p = relativeTimeProperties(unit.(string), c, options)
	}()
	nf := g.numberFormatter(options)
	plural := g.pluralGenerator(Undefined)
	return func(value any) string {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeNumber(value, "value")
		return relativeTimeFormat(value.(float64), nf, plural, p)
	}
}

// RelativeTimeFormatter is .relativeTimeFormatter(unit[, options]).
func (g *Globalize) RelativeTimeFormatter(unit any, options ...any) func(value any) string {
	return g.relativeTimeFormatter(fromPublic(unit), opt(options))
}

// FormatRelativeTime is .formatRelativeTime(value, unit[, options]).
func (g *Globalize) FormatRelativeTime(value, unit any, options ...any) string {
	v := fromPublic(value)
	validateParameterPresence(v, "value")
	validateParameterTypeNumber(v, "value")
	return g.relativeTimeFormatter(fromPublic(unit), opt(options))(value)
}

// RelativeTimeFormatter is Globalize.relativeTimeFormatter(unit[, options]) on the default locale.
func RelativeTimeFormatter(unit any, options ...any) func(value any) string {
	return statics.RelativeTimeFormatter(unit, options...)
}

// FormatRelativeTime is Globalize.formatRelativeTime(value, unit[, options]).
func FormatRelativeTime(value, unit any, options ...any) string {
	return statics.FormatRelativeTime(value, unit, options...)
}
