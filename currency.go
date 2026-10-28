package globalize

import (
	"regexp"
	"strings"
)

var (
	currencyCodeRe     = regexp.MustCompile(`^[A-Za-z]{3}$`)
	currencyFractionRe = regexp.MustCompile(`\.(#+|0*[0-9]|0+[0-9]?)`)
	unitPatternKeyRe   = regexp.MustCompile(`^unitPattern`)
	displayNameKeyRe   = regexp.MustCompile(`^displayName`)
	patternDigitCharRe = regexp.MustCompile(`[#@,.]`)
	keyCurrencyCldr    = new(int)
)

// propOf is the JavaScript v[key] with the TypeError V8 raises for an undefined or null base.
func propOf(v any, key string) any {
	if v == nil {
		throwTypeError("Cannot read properties of null (reading '" + key + "')")
	}
	if isUndefined(v) {
		throwTypeError("Cannot read properties of undefined (reading '" + key + "')")
	}
	return getProp(v, key)
}

func validateParameterTypeCurrency(value any, name string) {
	s, ok := value.(string)
	validateParameterType(value, name, isUndefined(value) || ok && currencyCodeRe.MatchString(s),
		"3-letter currency code string as defined by ISO 4217")
}

func currencySupplementalOverride(currency, pattern any, c *Cldr) string {
	fractionData := c.supplemental([]any{"currencyData/fractions", currency})
	if !jsTruthy(fractionData) {
		fractionData = c.supplemental("currencyData/fractions/DEFAULT")
	}
	digits := toNumber(propOf(fractionData, "_digits"))
	fraction := ""
	if digits != 0 && digits == digits {
		padded := stringPad("0", digits, false)
		u := utf16Units(padded)
		if len(u) > 0 {
			u = u[:len(u)-1]
		}
		fraction = "." + fromUnits(u) + jsString(getProp(fractionData, "_rounding"))
	}
	return currencyFractionRe.ReplaceAllLiteralString(requireString(pattern, "pattern", "replace"), fraction)
}

func currencyUnitPatterns(c *Cldr) *object {
	return objectFilter(c.main([]any{"numbers", "currencyFormats-numberSystem-" + jsString(numberNumberingSystem(c))}), unitPatternKeyRe)
}

type currencyProps struct {
	pattern      string
	symbol       any
	displayNames *object
	unitPatterns *object
	name         bool
}

func currencyNameProperties(currency any, c *Cldr) *currencyProps {
	pattern := currencySupplementalOverride(currency, numberPattern("decimal", c), c)
	return &currencyProps{
		name:         true,
		displayNames: objectFilter(c.main([]any{"numbers/currencies", currency}), displayNameKeyRe),
		pattern:      pattern,
		unitPatterns: currencyUnitPatterns(c),
	}
}

func inUnitRanges(u uint16, ranges []uint16) bool {
	for i := 0; i+1 < len(ranges); i += 2 {
		if u >= ranges[i] && u <= ranges[i+1] {
			return true
		}
	}
	return false
}

// requireSpacingClass is the lookup of regexp[name].test, which fails before its argument is read.
func requireSpacingClass(name any) {
	switch name {
	case "[:digit:]", "[:^S:]", "[[:^S:]&[:^Z:]]":
		return
	}
	throwTypeError("Cannot read properties of undefined (reading 'test')")
}

// currencySpacingMatch is regexp[name].test(s) for the classes Globalize's currency spacing
// uses; s holds at most one UTF-16 code unit.
func currencySpacingMatch(name any, s string) bool {
	requireSpacingClass(name)
	u := utf16Units(s)
	switch name {
	case "[:digit:]":
		return len(u) == 1 && u[0] >= '0' && u[0] <= '9'
	case "[:^S:]":
		return len(u) == 1 && inUnitRanges(u[0], notSRanges)
	}
	return len(u) == 1 && inUnitRanges(u[0], notSAndZRanges)
}

func charAtUnits(s string, i int) string {
	u := utf16Units(s)
	if i < 0 || i >= len(u) {
		return ""
	}
	return fromUnits(u[i : i+1])
}

func currencySymbolProperties(currency any, c *Cldr, options *object) *currencyProps {
	var symbol any = Undefined
	if options.prop("style") == "code" {
		symbol = currency
	} else {
		entries := []string{"symbol"}
		if options.prop("symbolForm") == "narrow" {
			entries = []string{"symbol-alt-narrow", "symbol"}
		}
		for _, e := range entries {
			symbol = c.main([]any{"numbers/currencies", currency, e})
			if jsTruthy(symbol) {
				break
			}
		}
	}
	spacing := make([]any, 2)
	for i, position := range []string{"beforeCurrency", "afterCurrency"} {
		spacing[i] = c.main([]any{"numbers", "currencyFormats-numberSystem-" + jsString(numberNumberingSystem(c)), "currencySpacing", position})
	}
	kind := "standard"
	if options.prop("style") == "accounting" {
		kind = "accounting"
	}
	pattern := c.main([]any{"numbers", "currencyFormats-numberSystem-" + jsString(numberNumberingSystem(c)), kind})
	subs := strings.Split(currencySupplementalOverride(currency, pattern, c), ";")
	for si, sub := range subs {
		parts := strings.Split(sub, "¤")
		for i, part := range parts {
			var sp any = Undefined
			if i < len(spacing) {
				sp = spacing[i]
			}
			currencyMatchName := propOf(sp, "currencyMatch")
			surroundingMatchName := propOf(sp, "surroundingMatch")
			requireSpacingClass(currencyMatchName)
			sym := requireString(symbol, "symbol", "charAt")
			pos := 0
			if i > 0 {
				pos = jsLength(sym) - 1
			}
			currencyMatch := currencySpacingMatch(currencyMatchName, charAtUnits(sym, pos))
			ppos := jsLength(part) - 1
			if i > 0 {
				ppos = 0
			}
			surrounding := patternDigitCharRe.ReplaceAllLiteralString(charAtUnits(part, ppos), "0")
			surroundingMatch := currencySpacingMatch(surroundingMatchName, surrounding)
			insertBetween := ""
			if currencyMatch && part != "" && surroundingMatch {
				insertBetween = jsString(propOf(sp, "insertBetween"))
			}
			if i > 0 {
				parts[i] = insertBetween + part
			} else {
				parts[i] = part + insertBetween
			}
		}
		subs[si] = strings.Join(parts, "¤")
	}
	return &currencyProps{pattern: strings.Join(subs, ";"), symbol: symbol}
}

type messagePart struct {
	typ   string
	value any
	name  string
}

// formatMessageToParts is Globalize's formatMessageToParts, including how it advances
// lastOffset (by offset plus match length, cumulatively) and never emits the trailing text.
func formatMessageToParts(messageArg any, data []any) []messagePart {
	message := requireString(messageArg, "message", "replace")
	units := utf16Units(message)
	var parts []messagePart
	push := func(typ string, value any) {
		if n := len(parts); n > 0 && parts[n-1].typ == typ {
			parts[n-1].value = jsAdd(parts[n-1].value, value)
			return
		}
		parts = append(parts, messagePart{typ: typ, value: value})
	}
	lastOffset := 0
	scanPlaceholders(message, func(start, end int, name string) string {
		offset := jsLength(message[:start])
		length := jsLength(message[start:end])
		lo, hi := lastOffset, offset
		if lo > len(units) {
			lo = len(units)
		}
		if hi > len(units) {
			hi = len(units)
		}
		lit := ""
		if lo < hi {
			lit = fromUnits(units[lo:hi])
		}
		push("literal", lit)
		var v any = Undefined
		if idx, ok := arrayIndex(name); ok && int(idx) < len(data) {
			v = data[idx]
		}
		push("variable", v)
		parts[len(parts)-1].name = name
		lastOffset += offset + length
		return ""
	})
	kept := parts[:0]
	for _, p := range parts {
		if s, ok := p.value.(string); ok && s == "" {
			continue
		}
		kept = append(kept, p)
	}
	return kept
}

// jsAdd is the JavaScript a + b.
func jsAdd(a, b any) any {
	pa, pb := jsToPrimitiveDefault(a), jsToPrimitiveDefault(b)
	_, aStr := pa.(string)
	_, bStr := pb.(string)
	if aStr || bStr {
		return jsString(pa) + jsString(pb)
	}
	return toNumber(pa) + toNumber(pb)
}

// jsToPrimitiveDefault is ToPrimitive(v) with no hint: a Date becomes its string form.
func jsToPrimitiveDefault(v any) any {
	switch x := v.(type) {
	case *object, []any, *Cldr, Date:
		return jsString(x)
	case []Part:
		items := make([]string, len(x))
		for i := range x {
			items[i] = "[object Object]"
		}
		return strings.Join(items, ",")
	}
	return v
}

func currencyNameFormat(formatted []Part, pluralForm string, p *currencyProps) []Part {
	displayNames := p.displayNames
	if displayNames == nil {
		displayNames = newObject()
	}
	displayName := displayNames.prop("displayName-count-" + pluralForm)
	if !jsTruthy(displayName) {
		displayName = displayNames.prop("displayName-count-other")
	}
	if !jsTruthy(displayName) {
		displayName = displayNames.prop("displayName")
	}
	if !jsTruthy(displayName) {
		displayName = Undefined
	}
	if p.unitPatterns == nil {
		throwTypeError("Cannot read properties of undefined (reading 'unitPattern-count-" + pluralForm + "')")
	}
	unitPattern := p.unitPatterns.prop("unitPattern-count-" + pluralForm)
	if !jsTruthy(unitPattern) {
		unitPattern = p.unitPatterns.prop("unitPattern-count-other")
	}
	var parts []Part
	for _, part := range formatMessageToParts(unitPattern, []any{formatted, displayName}) {
		switch {
		case part.typ == "variable" && part.name == "0":
			sub, ok := part.value.([]Part)
			if !ok {
				throwTypeError("part.value.forEach is not a function")
			}
			for _, s := range sub {
				parts = partsPush(parts, s.Type, s.Value)
			}
		case part.typ == "variable" && part.name == "1":
			parts = partsPushAny(parts, "currency", part.value)
		default:
			parts = partsPushAny(parts, "literal", part.value)
		}
	}
	return parts
}

func validateRequiredCldrCurrency(path string, value any) {
	validateCldr(path, value, currencyCldrSkip)
}

var currencyCldrSkip = []*regexp.Regexp{
	regexp.MustCompile(`numbers/currencies/[^/]+/symbol-alt-`),
	regexp.MustCompile(`supplemental/currencyData/fractions/[A-Za-z]{3}$`),
}

func objectOmit(o *object, key string) *object {
	cp := newObject()
	forIn(o, func(k string, v any) {
		if k != key {
			cp.set(k, v)
		}
	})
	return cp
}

func (g *Globalize) currencyToPartsFormatter(currency, options any) func(any) []Part {
	validateParameterPresence(currency, "currency")
	validateParameterTypeCurrency(currency, "currency")
	validateParameterTypePlainObject(options, "options")
	c := g.cldr
	o := optionsObject(options)
	style := o.prop("style")
	if !jsTruthy(style) {
		style = "symbol"
	}
	validateDefaultLocale(c)
	var p *currencyProps
	func() {
		c.on(keyCurrencyCldr, validateRequiredCldrCurrency)
		defer c.off(keyCurrencyCldr)
		switch jsString(jsToPrimitiveDefault(style)) {
		case "accounting", "code", "symbol":
			p = currencySymbolProperties(currency, c, o)
		case "name":
			p = currencyNameProperties(currency, c)
		default:
			throwTypeError("{(intermediate value)(intermediate value)(intermediate value)(intermediate value)}[style] is not a function")
		}
	}()
	numberOptions := objectOmit(o, "style")
	numberOptions.set("raw", p.pattern)
	if style == "symbol" || style == "accounting" || style == "code" {
		toParts := g.numberToPartsFormatter(numberOptions)
		symbol := p.symbol
		return func(value any) []Part {
			parts := toParts(value)
			for i := range parts {
				if parts[i].Type == "currency" {
					parts[i].Value = jsString(symbol)
				}
			}
			return parts
		}
	}
	toParts := g.numberToPartsFormatter(numberOptions)
	plural := g.pluralGenerator(Undefined)
	return func(value any) []Part {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeNumber(value, "value")
		return currencyNameFormat(toParts(value), plural(value), p)
	}
}

func (g *Globalize) currencyFormatter(currency, options any) func(any) string {
	validateParameterPresence(currency, "currency")
	validateParameterTypeCurrency(currency, "currency")
	validateParameterTypePlainObject(options, "options")
	toParts := g.currencyToPartsFormatter(currency, optionsArg(options, optionsObject(options)))
	return func(value any) string { return partsJoin(toParts(value)) }
}

// CurrencyFormatter is .currencyFormatter(currency[, options]).
func (g *Globalize) CurrencyFormatter(currency any, options ...any) func(value any) string {
	return g.currencyFormatter(fromPublic(currency), opt(options))
}

// CurrencyToPartsFormatter is .currencyToPartsFormatter(currency[, options]).
func (g *Globalize) CurrencyToPartsFormatter(currency any, options ...any) func(value any) []Part {
	return g.currencyToPartsFormatter(fromPublic(currency), opt(options))
}

// FormatCurrency is .formatCurrency(value, currency[, options]).
func (g *Globalize) FormatCurrency(value, currency any, options ...any) string {
	v := fromPublic(value)
	validateParameterPresence(v, "value")
	validateParameterTypeNumber(v, "value")
	return g.currencyFormatter(fromPublic(currency), opt(options))(value)
}

// FormatCurrencyToParts is .formatCurrencyToParts(value, currency[, options]).
func (g *Globalize) FormatCurrencyToParts(value, currency any, options ...any) []Part {
	v := fromPublic(value)
	validateParameterPresence(v, "value")
	validateParameterTypeNumber(v, "value")
	return g.currencyToPartsFormatter(fromPublic(currency), opt(options))(value)
}

// CurrencyFormatter is Globalize.currencyFormatter(currency[, options]) on the default locale.
func CurrencyFormatter(currency any, options ...any) func(value any) string {
	return statics.CurrencyFormatter(currency, options...)
}

// CurrencyToPartsFormatter is Globalize.currencyToPartsFormatter(currency[, options]).
func CurrencyToPartsFormatter(currency any, options ...any) func(value any) []Part {
	return statics.CurrencyToPartsFormatter(currency, options...)
}

// FormatCurrency is Globalize.formatCurrency(value, currency[, options]).
func FormatCurrency(value, currency any, options ...any) string {
	return statics.FormatCurrency(value, currency, options...)
}

// FormatCurrencyToParts is Globalize.formatCurrencyToParts(value, currency[, options]).
func FormatCurrencyToParts(value, currency any, options ...any) []Part {
	return statics.FormatCurrencyToParts(value, currency, options...)
}
