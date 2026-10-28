package globalize

import (
	"math"
	"regexp"
	"strings"
)

func createErrorUnsupportedFeature(feature any) *Error {
	return createError("E_UNSUPPORTED", "Unsupported {feature}.", attrs("feature", feature))
}

func featureObj(s string) *object { return attrs("feature", s) }

func numberNumberingSystem(c *Cldr) any {
	nu := c.attributes.prop("u-nu")
	if jsTruthy(nu) {
		if nu == "traditio" {
			nu = "traditional"
		}
		if nu == "native" || nu == "traditional" || nu == "finance" {
			return c.main([]any{"numbers/otherNumberingSystems", nu})
		}
		return nu
	}
	return c.main("numbers/defaultNumberingSystem")
}

type compactMap struct {
	byExponent  map[int]*object
	maxExponent int
}

func numberCompact(compactType any, c *Cldr) *compactMap {
	data := c.main([]any{"numbers/decimalFormats-numberSystem-" + jsString(numberNumberingSystem(c)), compactType, "decimalFormat"})
	obj, ok := data.(*object)
	if !ok {
		throwTypeError("Cannot convert undefined or null to object")
	}
	m := &compactMap{byExponent: map[int]*object{}}
	for _, key := range obj.keys {
		exp := strings.Count(key, "0")
		pluralForm := Undefined
		if parts := strings.Split(key, "-"); len(parts) > 2 {
			pluralForm = parts[2]
		}
		if m.byExponent[exp] == nil {
			m.byExponent[exp] = newObject()
		}
		m.byExponent[exp].set(jsString(pluralForm), obj.vals[key])
		if exp > m.maxExponent {
			m.maxExponent = exp
		}
	}
	return m
}

func numberNumberingSystemDigitsMap(c *Cldr) any {
	nu := numberNumberingSystem(c)
	if nu == "latn" {
		return Undefined
	}
	aux := c.supplemental([]any{"numberingSystems", nu})
	if getProp(aux, "_type") != "numeric" {
		if aux == nil || isUndefined(aux) {
			throwTypeError("Cannot read properties of " + jsString(aux) + " (reading '_type')")
		}
		panic(createErrorUnsupportedFeature("`" + jsString(getProp(aux, "_type")) + "` numbering system"))
	}
	return getProp(aux, "_digits")
}

var numberPatternRe = regexp.MustCompile(`^(('([^']|'')*'|[^*#@0,.E])*)(\*.)?((([#,]*[0,]*0+)(\.0*[0-9]*#*)?)|([#,]*@+#*))(E\+?0+)?(('[^']+'|''|[^*#@0,.E])*)$`)

// numberPatternProps holds what numberPatternProperties returns (undefined stays Undefined).
type numberPatternProps struct {
	prefix, suffix                                     string
	padding                                            any
	minimumIntegerDigits                               any
	minimumFractionDigits, maximumFractionDigits       any
	minimumSignificantDigits, maximumSignificantDigits any
	roundIncrement                                     any
	primaryGroupingSize, secondaryGroupingSize         any
}

func numberPatternProperties(pattern string) numberPatternProps {
	idx := numberPatternRe.FindStringSubmatchIndex(pattern)
	if idx == nil {
		throwError("Error", "Invalid pattern: null")
	}
	group := func(i int) (string, bool) {
		if idx[2*i] < 0 {
			return "", false
		}
		return pattern[idx[2*i]:idx[2*i+1]], true
	}
	p := numberPatternProps{padding: Undefined, minimumIntegerDigits: Undefined, minimumFractionDigits: Undefined,
		maximumFractionDigits: Undefined, minimumSignificantDigits: Undefined, maximumSignificantDigits: Undefined,
		roundIncrement: Undefined, primaryGroupingSize: Undefined, secondaryGroupingSize: Undefined}
	p.prefix, _ = group(1)
	padding, hasPadding := group(4)
	if hasPadding {
		p.padding = padding
	}
	ifs, _ := group(5)
	significant, _ := group(9)
	scientific, _ := group(10)
	p.suffix, _ = group(11)
	if significant != "" {
		m := regexp.MustCompile(`(@+)(#*)`).FindStringSubmatch(significant)
		if m != nil {
			p.minimumSignificantDigits = float64(len(m[1]))
			p.maximumSignificantDigits = float64(len(m[1]) + len(m[2]))
		}
	} else {
		fraction, _ := group(8)
		integer, _ := group(7)
		if fraction != "" {
			digits := regexp.MustCompile(`[0-9]+`).FindString(fraction)
			if digits != "" {
				p.roundIncrement = stringToNumber("0." + digits)
				p.minimumFractionDigits = float64(len(digits))
			} else {
				p.minimumFractionDigits = 0.0
			}
			p.maximumFractionDigits = float64(len(fraction) - 1)
		} else {
			p.minimumFractionDigits = 0.0
			p.maximumFractionDigits = 0.0
		}
		if m := regexp.MustCompile(`0+$`).FindString(integer); m != "" {
			p.minimumIntegerDigits = float64(len(m))
		}
	}
	if scientific != "" {
		panic(createErrorUnsupportedFeature(featureObj("scientific notation (not implemented)")))
	}
	if padding != "" {
		panic(createErrorUnsupportedFeature(featureObj("padding (not implemented)")))
	}
	if aux1 := strings.LastIndex(ifs, ","); aux1 != -1 {
		aux2 := strings.Split(ifs, ".")[0]
		p.primaryGroupingSize = float64(len(aux2) - aux1 - 1)
		from := aux1 - 1
		if from < 0 {
			from = 0
		}
		if j := strings.LastIndex(ifs[:from+1], ","); j != -1 {
			p.secondaryGroupingSize = float64(aux1 - 1 - j)
		}
	}
	return p
}

func numberSymbol(name string, c *Cldr) any {
	return c.main([]any{"numbers/symbols-numberSystem-" + jsString(numberNumberingSystem(c)), name})
}

var numberSymbolNames = [][2]string{{".", "decimal"}, {",", "group"}, {"%", "percentSign"}, {"+", "plusSign"},
	{"-", "minusSign"}, {"E", "exponential"}, {"‰", "perMille"}}

func numberSymbolName(symbol string) string {
	for _, p := range numberSymbolNames {
		if p[0] == symbol {
			return p[1]
		}
	}
	return ""
}

func numberSymbolMap(c *Cldr) *object {
	m := newObject()
	for _, p := range numberSymbolNames {
		m.set(p[0], numberSymbol(p[1], c))
	}
	return m
}

type roundFunc func(value float64, incrementOrExp any) float64

func mathMethod(method any) func(float64) float64 {
	switch method {
	case "round":
		return mathRound
	case "ceil":
		return math.Ceil
	case "floor":
		return math.Floor
	case "truncate":
		return func(v float64) float64 {
			if math.IsNaN(v) {
				return math.NaN()
			}
			return mathTrunc(v)
		}
	case "trunc":
		return mathTrunc
	case "abs":
		return math.Abs
	}
	return nil
}

func numberRound(method any) roundFunc {
	if !jsTruthy(method) {
		method = "round"
	}
	fn := mathMethod(method)
	call := func(v float64) float64 {
		if fn == nil {
			throwTypeError("method is not a function")
		}
		return fn(v)
	}
	return func(value float64, incrementOrExp any) float64 {
		if math.IsNaN(value) {
			return math.NaN()
		}
		var exp, increment float64
		if o, ok := incrementOrExp.(*object); ok && jsTruthy(o.prop("exponent")) {
			exp = toNumber(o.prop("exponent"))
			increment = 1
			if exp == 0 {
				return call(value)
			}
			if math.IsNaN(exp) || math.IsInf(exp, 0) || math.Mod(exp, 1) != 0 {
				return math.NaN()
			}
		} else {
			increment = toNumber(incrementOrExp)
			if !jsTruthy(increment) {
				increment = 1
			}
			if increment == 1 {
				return call(value)
			}
			parts := strings.Split(toExponential(increment), "e")
			exp = stringToNumber(parts[1])
			increment = stringToNumber(parts[0])
		}
		v := strings.Split(numToString(value), "e")
		v0 := stringToNumber(v[0]) / increment
		var v1 float64
		if len(v) > 1 && v[1] != "" {
			v1 = stringToNumber(v[1]) - exp
		} else {
			v1 = -exp
		}
		r := call(stringToNumber(numToString(v0) + "e" + numToString(v1)))
		v = strings.Split(numToString(r), "e")
		w0 := stringToNumber(v[0]) * increment
		var w1 float64
		if len(v) > 1 && v[1] != "" {
			w1 = stringToNumber(v[1]) + exp
		} else {
			w1 = exp
		}
		return stringToNumber(numToString(w0) + "e" + numToString(w1))
	}
}

type numberProps struct {
	numberPatternProps
	positivePattern, negativePattern string
	negativePrefix, negativeSuffix   string
	round                            roundFunc
	infinitySymbol, nanSymbol        any
	symbolMap                        *object
	nuDigitsMap                      any
	compact                          *compactMap
}

func jsMathMax(a, b any) float64 {
	x, y := toNumber(a), toNumber(b)
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN()
	}
	return math.Max(x, y)
}

func jsMathMin(a, b any) float64 {
	x, y := toNumber(a), toNumber(b)
	if math.IsNaN(x) || math.IsNaN(y) {
		return math.NaN()
	}
	return math.Min(x, y)
}

func numberFormatProperties(patternArg any, c *Cldr, options *object) *numberProps {
	pattern, ok := patternArg.(string)
	if !ok {
		throwTypeError("pattern.split is not a function")
	}
	parts := strings.Split(pattern, ";")
	positivePattern := parts[0]
	negativePattern := "-" + positivePattern
	if len(parts) > 1 && parts[1] != "" {
		negativePattern = parts[1]
	}
	negative := numberPatternProperties(negativePattern)
	roundFn := numberRound(options.prop("round"))
	p := &numberProps{numberPatternProps: numberPatternProperties(positivePattern)}
	p.positivePattern = positivePattern
	p.negativePattern = negative.prefix + positivePattern + negative.suffix
	p.negativePrefix = negative.prefix
	p.negativeSuffix = negative.suffix
	p.round = roundFn
	p.infinitySymbol = numberSymbol("infinity", c)
	p.nanSymbol = numberSymbol("nan", c)
	p.symbolMap = numberSymbolMap(c)
	p.nuDigitsMap = numberNumberingSystemDigitsMap(c)
	if compact := options.prop("compact"); jsTruthy(compact) {
		p.minimumIntegerDigits = 1.0
		p.minimumFractionDigits = 0.0
		p.maximumFractionDigits = 0.0
		p.minimumSignificantDigits = Undefined
		p.maximumSignificantDigits = Undefined
		p.compact = numberCompact(compact, c)
	}
	if v, ok := options.get("minimumIntegerDigits"); ok {
		p.minimumIntegerDigits = v
	}
	if v, ok := options.get("minimumFractionDigits"); ok {
		p.minimumFractionDigits = v
	}
	if v, ok := options.get("maximumFractionDigits"); ok {
		p.maximumFractionDigits = v
	}
	if v, ok := options.get("minimumSignificantDigits"); ok {
		p.minimumSignificantDigits = v
	}
	if v, ok := options.get("maximumSignificantDigits"); ok {
		p.maximumSignificantDigits = v
	}
	if options.prop("useGrouping") == false {
		p.primaryGroupingSize = nil
	}
	hasMin, hasMax := options.has("minimumFractionDigits"), options.has("maximumFractionDigits")
	if hasMin && !hasMax {
		p.maximumFractionDigits = jsMathMax(p.minimumFractionDigits, p.maximumFractionDigits)
	} else if !hasMin && hasMax {
		p.minimumFractionDigits = jsMathMin(p.minimumFractionDigits, p.maximumFractionDigits)
	}
	return p
}

var (
	cfRanges   = [][2]rune{{0xAD, 0xAD}, {0x600, 0x605}, {0x61C, 0x61C}, {0x6DD, 0x6DD}, {0x70F, 0x70F}, {0x8E2, 0x8E2}, {0x180E, 0x180E}, {0x200B, 0x200F}, {0x202A, 0x202E}, {0x2060, 0x2064}, {0x2066, 0x206F}, {0xFEFF, 0xFEFF}, {0xFFF9, 0xFFFB}, {0x110BD, 0x110BD}, {0x110CD, 0x110CD}, {0x13430, 0x13438}, {0x1BCA0, 0x1BCA3}, {0x1D173, 0x1D17A}, {0xE0001, 0xE0001}, {0xE0020, 0xE007F}}
	dashRanges = [][2]rune{{0x2D, 0x2D}, {0x58A, 0x58A}, {0x5BE, 0x5BE}, {0x1400, 0x1400}, {0x1806, 0x1806}, {0x2010, 0x2015}, {0x2E17, 0x2E17}, {0x2E1A, 0x2E1A}, {0x2E3A, 0x2E3B}, {0x2E40, 0x2E40}, {0x301C, 0x301C}, {0x3030, 0x3030}, {0x30A0, 0x30A0}, {0xFE31, 0xFE32}, {0xFE58, 0xFE58}, {0xFE63, 0xFE63}, {0xFF0D, 0xFF0D}, {0x2212, 0x2212}, {0x10EAD, 0x10EAD}}
	zsRanges   = [][2]rune{{0x20, 0x20}, {0xA0, 0xA0}, {0x1680, 0x1680}, {0x2000, 0x200A}, {0x202F, 0x202F}, {0x205F, 0x205F}, {0x3000, 0x3000}}
)

func inRanges(r rune, ranges [][2]rune) bool {
	for _, rg := range ranges {
		if r >= rg[0] && r <= rg[1] {
			return true
		}
	}
	return false
}

func looseMatching(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case inRanges(r, cfRanges):
			return -1
		case inRanges(r, dashRanges):
			return '-'
		case inRanges(r, zsRanges):
			return ' '
		}
		return r
	}, s)
}

func removeLiteralQuotes(s string) string {
	u := utf16Units(s)
	if len(u) == 0 || u[0] != '\'' || u[len(u)-1] != '\'' {
		return s
	}
	if s == "''" {
		return "'"
	}
	s = strings.ReplaceAll(s, "''", "'")
	u = utf16Units(s)
	if len(u) < 2 {
		return ""
	}
	return fromUnits(u[1 : len(u)-1])
}

func numberParseProperties(pattern any, c *Cldr, options *object) *numberParseProps {
	fp := numberFormatProperties(pattern, c, options)
	prefix := looseMatching(fp.prefix)
	maximumFractionDigits := fp.maximumFractionDigits
	minSD, maxSD := fp.minimumSignificantDigits, fp.maximumSignificantDigits
	primary, secondary := fp.primaryGroupingSize, fp.secondaryGroupingSize
	suffix := looseMatching(fp.suffix)
	negativePrefix := looseMatching(fp.negativePrefix)
	negativeSuffix := looseMatching(fp.negativeSuffix)
	infinitySymbol := looseMatching(jsString(fp.infinitySymbol))
	nanSymbol := looseMatching(jsString(fp.nanSymbol))
	symbolMap := newObject()
	for _, k := range fp.symbolMap.keys {
		symbolMap.set(k, looseMatching(jsString(fp.symbolMap.vals[k])))
	}
	inverted := numberSymbolInvertedMap(c)
	invertedSymbolMap := newObject()
	for _, k := range inverted.keys {
		invertedSymbolMap.set(looseMatching(k), inverted.vals[k])
	}
	digitsRe := `\d`
	var invertedNuDigitsMap *object
	if s, ok := fp.nuDigitsMap.(string); ok && s != "" {
		digitsRe = "[" + s + "]"
		invertedNuDigitsMap = newObject()
		for i, u := range utf16Units(s) {
			invertedNuDigitsMap.set(fromUnits([]uint16{u}), numToString(float64(i)))
		}
	}
	groupingSeparatorRe := regexpEscape(jsString(symbolMap.prop(",")))
	decimalSymbolRe := regexpEscape(jsString(symbolMap.prop(".")))
	mapAffix := func(value string) string {
		var b strings.Builder
		last := 0
		literalTokensIdx(value, func(start, end int, literal bool) {
			b.WriteString(value[last:start])
			tok := value[start:end]
			if literal {
				b.WriteString(removeLiteralQuotes(tok))
			} else {
				switch tok {
				case "-", "+", "E", "%", "\u2030":
					b.WriteString(jsString(symbolMap.prop(tok)))
				default:
					b.WriteString(tok)
				}
			}
			last = end
		})
		b.WriteString(value[last:])
		return b.String()
	}
	prefix, suffix = mapAffix(prefix), mapAffix(suffix)
	negativePrefix, negativeSuffix = mapAffix(negativePrefix), mapAffix(negativeSuffix)
	numberTokenizer := digitsRe + "+"
	if jsTruthy(primary) {
		var aux string
		if jsTruthy(secondary) {
			aux = digitsRe + "{1," + jsString(secondary) + "}((" + groupingSeparatorRe + digitsRe + "{" + jsString(secondary) + "})*(" + groupingSeparatorRe + digitsRe + "{" + jsString(primary) + "}))"
		} else {
			aux = digitsRe + "{1," + jsString(primary) + "}(" + groupingSeparatorRe + digitsRe + "{" + jsString(primary) + "})+"
		}
		numberTokenizer = "(" + aux + "|" + numberTokenizer + ")"
	}
	if !math.IsNaN(toNumber(minSD)*toNumber(maxSD)) || jsTruthy(maximumFractionDigits) {
		aux := decimalSymbolRe + digitsRe + "+"
		numberTokenizer = numberTokenizer + "(" + aux + "|" + decimalSymbolRe + ")" + "?" + "|(" + numberTokenizer + ")?" + aux
		numberTokenizer = "(" + numberTokenizer + ")"
	}
	return &numberParseProps{
		invertedSymbolMap:   invertedSymbolMap,
		invertedNuDigitsMap: invertedNuDigitsMap,
		tokenizer: map[string]*jsRegexp{
			"infinity":       mustJSRegexp("^" + regexpEscape(infinitySymbol)),
			"nan":            mustJSRegexp("^" + regexpEscape(nanSymbol)),
			"negativePrefix": mustJSRegexp("^" + regexpEscape(negativePrefix)),
			"negativeSuffix": mustJSRegexp("^" + regexpEscape(negativeSuffix)),
			"number":         mustJSRegexp("^" + numberTokenizer),
			"prefix":         mustJSRegexp("^" + regexpEscape(prefix)),
			"suffix":         mustJSRegexp("^" + regexpEscape(suffix)),
		},
	}
}

type numberParseProps struct {
	invertedSymbolMap   *object
	invertedNuDigitsMap *object
	tokenizer           map[string]*jsRegexp
}

func numberSymbolInvertedMap(c *Cldr) *object {
	m := newObject()
	for _, p := range numberSymbolNames {
		m.set(jsString(numberSymbol(p[1], c)), p[0])
	}
	return m
}

func numberParse(value string, props *numberParseProps) float64 {
	grammar := [][]string{{"nan"}, {"prefix", "infinity", "suffix"}, {"prefix", "number", "suffix"},
		{"negativePrefix", "infinity", "negativeSuffix"}, {"negativePrefix", "number", "negativeSuffix"}}
	var number float64
	var prefix, suffix any = Undefined, Undefined
	negative := false
	value = looseMatching(value)
	parse := func(typ, lexeme string) {
		var b strings.Builder
		for _, u := range utf16Units(lexeme) {
			ch := fromUnits([]uint16{u})
			if v, ok := props.invertedSymbolMap.get(ch); ok && jsTruthy(v) {
				b.WriteString(jsString(v))
			} else if v, ok := props.invertedNuDigitsMap.get(ch); ok && jsTruthy(v) {
				b.WriteString(jsString(v))
			} else {
				b.WriteString(ch)
			}
		}
		lexeme = b.String()
		switch typ {
		case "infinity":
			number = math.Inf(1)
		case "nan":
			number = math.NaN()
		case "number":
			number = stringToNumber(strings.ReplaceAll(lexeme, ",", ""))
		case "prefix", "negativePrefix":
			prefix = lexeme
		case "suffix":
			suffix = lexeme
		case "negativeSuffix":
			suffix = lexeme
			negative = true
		}
	}
	valid := false
	for _, statement := range grammar {
		v := value
		all := true
		for _, typ := range statement {
			re := props.tokenizer[typ]
			loc := re.find(v)
			if loc == nil {
				all = false
				break
			}
			parse(typ, v[loc[0]:loc[1]])
			v = v[:loc[0]] + v[loc[1]:]
		}
		if all && v == "" {
			valid = true
			break
		}
	}
	if !valid || math.IsNaN(number) {
		return math.NaN()
	}
	prefixNSuffix := jsString(prefix) + jsString(suffix)
	if strings.Contains(prefixNSuffix, "%") {
		number /= 100
	} else if strings.Contains(prefixNSuffix, "‰") {
		number /= 1e3
	}
	if negative {
		number *= -1
	}
	return number
}

func numberPattern(style any, c *Cldr) any {
	if style != "decimal" && style != "percent" {
		throwError("Error", "Invalid style")
	}
	return c.main([]any{"numbers", jsString(style) + "Formats-numberSystem-" + jsString(numberNumberingSystem(c)), "standard"})
}

func numberFormatGroupingSeparator(number string, primary, secondary any) string {
	current := toNumber(primary)
	ret := ""
	switchToSecondary := jsTruthy(secondary)
	parts := strings.Split(number, ".")
	index := float64(len(parts[0]))
	for index > current {
		start := int(index - current)
		if start < 0 {
			start = 0
		}
		sep := ""
		if ret != "" {
			sep = ","
		}
		ret = parts[0][start:int(index)] + sep + ret
		index -= current
		if switchToSecondary {
			current = toNumber(secondary)
			switchToSecondary = false
		}
	}
	sep := ""
	if ret != "" {
		sep = ","
	}
	end := int(index)
	if end < 0 {
		end = 0
	}
	parts[0] = parts[0][:end] + sep + ret
	return strings.Join(parts, ".")
}

var eMinusRe = regexp.MustCompile(`e-`)

func numberFormatIntegerFractionDigits(number float64, minInt, minFrac, maxFrac any, round roundFunc, roundIncrement any) string {
	if jsTruthy(maxFrac) {
		if jsTruthy(roundIncrement) {
			number = round(number, roundIncrement)
		} else {
			number = round(number, attrs("exponent", -toNumber(maxFrac)))
		}
	} else {
		number = round(number, Undefined)
	}
	s := numToString(number)
	if jsTruthy(maxFrac) && eMinusRe.MatchString(s) {
		d := toIntegerOrInfinity(toNumber(maxFrac))
		if d < 0 || d > 100 {
			throwError("RangeError", "toFixed() digits argument must be between 0 and 100")
		}
		s = toFixed(stringToNumber(s), int(d))
		s = strings.TrimRight(s, "0")
		s = strings.TrimSuffix(s, ".")
	}
	if jsTruthy(minFrac) {
		parts := strings.Split(s, ".")
		frac := ""
		if len(parts) > 1 {
			frac = parts[1]
		}
		frac = stringPad(frac, minFrac, true)
		if len(parts) > 1 {
			parts[1] = frac
		} else {
			parts = append(parts, frac)
		}
		s = strings.Join(parts, ".")
	}
	if jsTruthy(minInt) {
		parts := strings.Split(s, ".")
		parts[0] = stringPad(parts[0], minInt, false)
		s = strings.Join(parts, ".")
	}
	return s
}

func numberToPrecision(number float64, precision any, round roundFunc) float64 {
	if number == 0 {
		return number
	}
	roundOrder := math.Ceil(jsLog(math.Abs(number)) / jsLog(10))
	roundOrder -= toNumber(precision)
	return round(number, attrs("exponent", roundOrder))
}

var leadingZerosOrDotRe = regexp.MustCompile(`^0+|\.`)

func numberFormatSignificantDigits(number float64, minSD, maxSD any, round roundFunc) string {
	if jsCompare(minSD, maxSD, ">") {
		maxSD = minSD
	}
	atMinimum := numberToPrecision(number, minSD, round)
	atMaximum := numberToPrecision(number, maxSD, round)
	n := atMaximum
	if atMinimum == atMaximum {
		n = atMinimum
	}
	s := numToString(n)
	if strings.Contains(s, "e") {
		panic(createErrorUnsupportedFeature(featureObj("integers out of (1e21, 1e-7)")))
	}
	if toNumber(minSD)-float64(len(leadingZerosOrDotRe.ReplaceAllString(s, ""))) > 0 {
		parts := strings.Split(s, ".")
		frac := ""
		if len(parts) > 1 {
			frac = parts[1]
		}
		intDigits := len(strings.TrimLeft(parts[0], "0"))
		frac = stringPad(frac, toNumber(minSD)-float64(intDigits), true)
		if len(parts) > 1 {
			parts[1] = frac
		} else {
			parts = append(parts, frac)
		}
		s = strings.Join(parts, ".")
	}
	return s
}

var numberCompactPatternRe = regexp.MustCompile(`^([^0]*)(0+)([^0]*)$`)

func numberFormat(number float64, p *numberProps, pluralGenerator func(any) string) []Part {
	if math.IsNaN(number) {
		return []Part{{Type: "nan", Value: jsString(p.nanSymbol)}}
	}
	var pattern, prefixS, suffixS string
	if number < 0 {
		pattern, prefixS, suffixS = p.negativePattern, p.negativePrefix, p.negativeSuffix
	} else {
		pattern, prefixS, suffixS = p.positivePattern, p.prefix, p.suffix
	}
	stringToParts := func(s string) []Part {
		numberType := "integer"
		var parts []Part
		literalTokens(s, func(tok string, literal bool) {
			if literal {
				parts = partsPush(parts, "literal", removeLiteralQuotes(tok))
				return
			}
			if tok == "¤" {
				parts = partsPush(parts, "currency", tok)
				return
			}
			switch tok {
			case ".", ",", "-", "+", "E", "%", "\u2030":
				if tok == "." {
					numberType = "fraction"
				}
				parts = partsPush(parts, numberSymbolName(tok), jsString(p.symbolMap.prop(tok)))
				return
			}
			if len(tok) == 1 && tok[0] >= '0' && tok[0] <= '9' {
				digit := tok
				if s, ok := p.nuDigitsMap.(string); ok {
					digit = jsString(stringIndex(s, int(tok[0]-'0')))
				} else if jsTruthy(p.nuDigitsMap) {
					digit = jsString(getProp(p.nuDigitsMap, tok))
				}
				parts = partsPush(parts, numberType, digit)
				return
			}
			parts = partsPush(parts, "literal", tok)
		})
		return parts
	}
	prefix := stringToParts(prefixS)
	suffix := stringToParts(suffixS)
	if math.IsInf(number, 0) {
		out := append([]Part{}, prefix...)
		out = append(out, Part{Type: "infinity", Value: jsString(p.infinitySymbol)})
		return append(out, suffix...)
	}
	if strings.Contains(pattern, "%") {
		number *= 100
	} else if strings.Contains(pattern, "‰") {
		number *= 1e3
	}
	var compactPattern any = Undefined
	numberExponent := 0
	if p.compact != nil {
		numberExponent = len(numToString(math.Abs(math.Floor(number)))) - 1
		if numberExponent > p.compact.maxExponent {
			numberExponent = p.compact.maxExponent
		}
		if numberExponent >= 3 {
			if o := p.compact.byExponent[numberExponent]; o != nil {
				compactPattern = o.prop("other")
			}
		}
		if compactPattern == "0" {
			compactPattern = nil
		} else if jsTruthy(compactPattern) {
			compactDigits := strings.Count(jsString(compactPattern), "0")
			divisor := numberExponent - (compactDigits - 1)
			number = number / pow10(divisor)
		}
	}
	var s string
	if !math.IsNaN(toNumber(p.minimumSignificantDigits) * toNumber(p.maximumSignificantDigits)) {
		s = numberFormatSignificantDigits(number, p.minimumSignificantDigits, p.maximumSignificantDigits, p.round)
	} else {
		s = numberFormatIntegerFractionDigits(number, p.minimumIntegerDigits, p.minimumFractionDigits, p.maximumFractionDigits, p.round, p.roundIncrement)
	}
	if p.compact != nil && jsTruthy(compactPattern) {
		pluralForm := "other"
		if pluralGenerator != nil {
			pluralForm = pluralGenerator(stringToNumber(s))
		}
		if v := p.compact.byExponent[numberExponent].prop(pluralForm); jsTruthy(v) {
			compactPattern = v
		}
		m := numberCompactPatternRe.FindStringSubmatch(jsString(compactPattern))
		if m == nil {
			throwTypeError("Cannot read properties of null (reading '1')")
		}
		aux := func(str string) []Part {
			var parts []Part
			compactAffixTokens(str, func(tok string, literal bool) {
				if literal {
					parts = partsPush(parts, "literal", removeLiteralQuotes(tok))
				} else {
					parts = partsPush(parts, "compact", tok)
				}
			})
			return parts
		}
		prefix = append(prefix, aux(m[1])...)
		suffix = append(aux(m[3]), suffix...)
	}
	s = strings.TrimPrefix(s, "-")
	if jsTruthy(p.primaryGroupingSize) {
		s = numberFormatGroupingSeparator(s, p.primaryGroupingSize, p.secondaryGroupingSize)
	}
	out := append([]Part{}, prefix...)
	out = append(out, stringToParts(s)...)
	return append(out, suffix...)
}

func validateDigits(p *numberProps) {
	minInt, minFrac, maxFrac := p.minimumIntegerDigits, p.minimumFractionDigits, p.maximumFractionDigits
	minSD, maxSD := p.minimumSignificantDigits, p.maximumSignificantDigits
	if !math.IsNaN(toNumber(minSD) * toNumber(maxSD)) {
		validateParameterRange(minSD, "minimumSignificantDigits", 1.0, 21.0)
		validateParameterRange(maxSD, "maximumSignificantDigits", minSD, 21.0)
	} else if !math.IsNaN(toNumber(minSD)) || !math.IsNaN(toNumber(maxSD)) {
		throwError("Error", "Neither or both the minimum and maximum significant digits must be present")
	} else {
		validateParameterRange(minInt, "minimumIntegerDigits", 1.0, 21.0)
		validateParameterRange(minFrac, "minimumFractionDigits", 0.0, 20.0)
		validateParameterRange(maxFrac, "maximumFractionDigits", minFrac, 20.0)
	}
}

func (g *Globalize) numberToPartsFormatter(options any) func(any) []Part {
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	c := g.cldr
	validateDefaultLocale(c)
	var p *numberProps
	func() {
		c.on(keyValidateCldr, validateCldrListener)
		defer c.off(keyValidateCldr)
		var pattern any
		if raw := o.prop("raw"); jsTruthy(raw) {
			pattern = raw
		} else {
			style := o.prop("style")
			if !jsTruthy(style) {
				style = "decimal"
			}
			pattern = numberPattern(style, c)
		}
		p = numberFormatProperties(pattern, c, o)
	}()
	validateDigits(p)
	var plural func(any) string
	if jsTruthy(o.prop("compact")) {
		plural = g.pluralGenerator(Undefined)
	}
	return func(value any) []Part {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeNumber(value, "value")
		return numberFormat(value.(float64), p, plural)
	}
}

func (g *Globalize) numberFormatter(options any) func(any) string {
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	toParts := g.numberToPartsFormatter(optionsArg(options, o))
	return func(value any) string { return partsJoin(toParts(value)) }
}

// optionsArg is what a function passes on after `options = options || {}`.
func optionsArg(original any, o *object) any {
	if _, ok := original.(*object); ok {
		return original
	}
	if !jsTruthy(original) {
		return o
	}
	return original
}

func (g *Globalize) numberParser(options any) func(any) float64 {
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	c := g.cldr
	validateDefaultLocale(c)
	if jsTruthy(o.prop("compact")) {
		panic(createErrorUnsupportedFeature(featureObj("compact number parsing (not implemented)")))
	}
	c.on(keyValidateCldr, validateCldrListener)
	var pattern any
	if raw := o.prop("raw"); jsTruthy(raw) {
		pattern = raw
	} else {
		style := o.prop("style")
		if !jsTruthy(style) {
			style = "decimal"
		}
		pattern = numberPattern(style, c)
	}
	props := numberParseProperties(pattern, c, o)
	c.off(keyValidateCldr)
	return func(value any) float64 {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeString(value, "value")
		return numberParse(value.(string), props)
	}
}

// NumberFormatter is .numberFormatter([options]).
func (g *Globalize) NumberFormatter(options ...any) func(value any) string {
	return g.numberFormatter(opt(options))
}

// NumberToPartsFormatter is .numberToPartsFormatter([options]).
func (g *Globalize) NumberToPartsFormatter(options ...any) func(value any) []Part {
	return g.numberToPartsFormatter(opt(options))
}

// NumberParser is .numberParser([options]).
func (g *Globalize) NumberParser(options ...any) func(value any) float64 {
	return g.numberParser(opt(options))
}

// FormatNumber is .formatNumber(value[, options]).
func (g *Globalize) FormatNumber(value any, options ...any) string {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	validateParameterTypeNumber(value, "value")
	return g.numberFormatter(opt(options))(value)
}

// FormatNumberToParts is .formatNumberToParts(value[, options]).
func (g *Globalize) FormatNumberToParts(value any, options ...any) []Part {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	validateParameterTypeNumber(value, "value")
	return g.numberToPartsFormatter(opt(options))(value)
}

// ParseNumber is .parseNumber(value[, options]).
func (g *Globalize) ParseNumber(value any, options ...any) float64 {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	validateParameterTypeString(value, "value")
	return g.numberParser(opt(options))(value)
}

// NumberFormatter is Globalize.numberFormatter([options]) on the default locale.
func NumberFormatter(options ...any) func(value any) string {
	return statics.NumberFormatter(options...)
}

// NumberToPartsFormatter is Globalize.numberToPartsFormatter([options]).
func NumberToPartsFormatter(options ...any) func(value any) []Part {
	return statics.NumberToPartsFormatter(options...)
}

// NumberParser is Globalize.numberParser([options]).
func NumberParser(options ...any) func(value any) float64 { return statics.NumberParser(options...) }

// FormatNumber is Globalize.formatNumber(value[, options]).
func FormatNumber(value any, options ...any) string { return statics.FormatNumber(value, options...) }

// FormatNumberToParts is Globalize.formatNumberToParts(value[, options]).
func FormatNumberToParts(value any, options ...any) []Part {
	return statics.FormatNumberToParts(value, options...)
}

// ParseNumber is Globalize.parseNumber(value[, options]).
func ParseNumber(value any, options ...any) float64 { return statics.ParseNumber(value, options...) }
