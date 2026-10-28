package globalize

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

// requireString is the TypeError V8 raises when Globalize calls a string method (expr.method)
// on a value that is not a string.
func requireString(v any, expr, method string) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		throwTypeError("Cannot read properties of null (reading '" + method + "')")
	}
	if isUndefined(v) {
		throwTypeError("Cannot read properties of undefined (reading '" + method + "')")
	}
	throwTypeError(expr + "." + method + " is not a function")
	return ""
}

func looseMatchingAny(v any) string { return looseMatching(requireString(v, "value", "replace")) }

// objectKeys is Object.keys(v).
func objectKeys(v any) []string {
	switch t := v.(type) {
	case *object:
		return append([]string(nil), t.keys...)
	case []any:
		keys := make([]string, len(t))
		for i := range t {
			keys[i] = numToString(float64(i))
		}
		return keys
	case string:
		n := jsLength(t)
		keys := make([]string, n)
		for i := 0; i < n; i++ {
			keys[i] = numToString(float64(i))
		}
		return keys
	}
	return nil
}

type lookupItem struct {
	key string
	re  *jsRegexp
}

type dateTokProps struct {
	pattern                  string
	digitsRe                 *jsRegexp
	vals                     map[string]any
	standardOrDaylightTzName *jsRegexp
	genericTzName            *jsRegexp
	timeZoneName             any
	timeZoneNameRe           *jsRegexp
	gmtZeroFormatRe          *jsRegexp
	x                        []*jsRegexp
}

var (
	hhmmssRe        = regexp.MustCompile(`HH|mm|ss`)
	hOrMRe          = regexp.MustCompile(`H|m`)
	populateSkipRe  = regexp.MustCompile(`(timeZoneNames/zone|supplemental/metaZones|timeZoneNames/metazone|timeZoneNames/regionFormat|timeZoneNames/gmtFormat)`)
	datesPrefixRe   = mustJSRegexp(`^.*\/dates\/`)
	amPmKeyRe       = regexp.MustCompile(`^am|^pm`)
	xHourFormatsSet = [][]string{{"+HHmm;-HHmm", "+HH;-HH"}, {"+HHmm;-HHmm"}, {"+HH:mm;-HH:mm"}, {"+HHmmss;-HHmmss", "+HHmm;-HHmm"}, {"+HH:mm:ss;-HH:mm:ss", "+HH:mm;-HH:mm"}}
)

func hourFormatRe(hourFormat string, gmtFormat any, digitsReSource string, timeSeparator any) *jsRegexp {
	if digitsReSource == "" {
		digitsReSource = `\d`
	}
	gmt := "{0}"
	if jsTruthy(gmtFormat) {
		gmt = jsString(gmtFormat)
	}
	re := strings.Replace(hourFormat, "+", `\+`, 1)
	re = hhmmssRe.ReplaceAllLiteralString(re, "(("+digitsReSource+"){2})")
	re = hOrMRe.ReplaceAllLiteralString(re, "(("+digitsReSource+"){1,2})")
	if jsTruthy(timeSeparator) {
		re = strings.ReplaceAll(re, ":", jsReplacement(jsString(timeSeparator)))
	}
	parts := strings.Split(re, ";")
	for i, part := range parts {
		parts[i] = strings.Replace(gmt, "{0}", jsReplacement(part), 1)
	}
	return mustJSRegexp("^" + strings.Join(parts, "|"))
}

func objectFilter(v any, re *regexp.Regexp) *object {
	cp := newObject()
	forIn(v, func(k string, val any) {
		if re.MatchString(k) {
			cp.set(k, val)
		}
	})
	return cp
}

func regexpSourceSomeTerm(terms []any) string {
	var kept []string
	for _, t := range terms {
		if jsTruthy(t) {
			kept = append(kept, jsString(t))
		}
	}
	return "(" + strings.Join(kept, "|") + ")"
}

func dateTokenizerProperties(patternArg any, c *Cldr, timeZone any) *dateTokProps {
	pattern := requireString(patternArg, "value", "replace")
	p := &dateTokProps{pattern: looseMatching(pattern), vals: map[string]any{}, timeZoneName: Undefined}
	timeSeparator := numberSymbol("timeSeparator", c)
	digitsReSource := `\d`
	if dm := numberNumberingSystemDigitsMap(c); jsTruthy(dm) {
		digitsReSource = "[" + jsString(dm) + "]"
	}
	p.digitsRe = mustJSRegexp(digitsReSource)
	vals := p.vals
	populate := func(path string, value any) {
		if populateSkipRe.MatchString(path) || !jsTruthy(value) {
			return
		}
		if loc := datesPrefixRe.find(path); loc != nil {
			path = path[:loc[0]] + path[loc[1]:]
		}
		path = strings.Replace(path, "calendars/", "", 1)
		if path == "gregorian/dayPeriods/format/wide" {
			value = objectFilter(value, amPmKeyRe)
		}
		if isPlainObject(value) {
			var items []lookupItem
			for _, k := range objectKeys(value) {
				items = append(items, lookupItem{k, mustJSRegexp("^" + regexpEscape(looseMatchingAny(getProp(value, k))))})
			}
			sort.SliceStable(items, func(i, j int) bool { return items[i].re.sourceLength() > items[j].re.sourceLength() })
			vals[path] = items
		} else {
			vals[path] = looseMatchingAny(value)
		}
	}
	key := new(int)
	c.on(key, populate)
	toks := datePatternMatch(pattern)
	if toks == nil {
		throwTypeError("Cannot read properties of null (reading 'forEach')")
	}
	for _, current := range toks {
		chr := charAt0(current)
		length := jsLength(current)
		if chr == "Z" {
			if length < 5 {
				chr, length = "O", 4
			} else {
				chr, length = "X", 5
			}
		}
		if chr == "z" {
			var standard any = dateGetTimeZoneName(length, "standard", timeZone, c)
			var daylight any = dateGetTimeZoneName(length, "daylight", timeZone, c)
			if jsTruthy(standard) {
				standard = regexpEscape(looseMatchingAny(standard))
			}
			if jsTruthy(daylight) {
				daylight = regexpEscape(looseMatchingAny(daylight))
			}
			if jsTruthy(standard) || jsTruthy(daylight) {
				p.standardOrDaylightTzName = mustJSRegexp("^" + regexpSourceSomeTerm([]any{standard, daylight}))
			}
			if !jsTruthy(standard) || !jsTruthy(daylight) {
				chr = "O"
				if length < 4 {
					length = 1
				}
			}
		}
		if chr == "v" {
			if length != 1 && length != 4 {
				panic(createErrorUnsupportedFeature(featureObj("timezone pattern `" + pattern + "`")))
			}
			if generic := dateGetTimeZoneName(length, "generic", timeZone, c); jsTruthy(generic) {
				p.genericTzName = mustJSRegexp("^" + regexpEscape(looseMatchingAny(generic)))
				chr = "O"
			} else {
				chr, length = "V", 4
			}
		}
		fallO, fallX := false, false
		switch chr {
		case "G":
			kind := "eraNarrow"
			if length <= 3 {
				kind = "eraAbbr"
			} else if length == 4 {
				kind = "eraNames"
			}
			c.main([]any{"dates/calendars/gregorian/eras", kind})
		case "u", "U":
			panic(createErrorUnsupportedFeature(featureObj("year pattern `" + chr + "`")))
		case "Q", "q":
			if length > 2 {
				kind := "stand-alone"
				if chr == "Q" {
					kind = "format"
				}
				c.main([]any{"dates/calendars/gregorian/quarters", kind, widthAt(length - 3)})
			}
		case "M", "L":
			if length > 2 {
				kind := "stand-alone"
				if chr == "M" {
					kind = "format"
				}
				c.main([]any{"dates/calendars/gregorian/months", kind, widthAt(length - 3)})
			}
		case "g":
			panic(createErrorUnsupportedFeature(featureObj("Julian day pattern `g`")))
		case "e", "c", "E":
			if (chr == "e" || chr == "c") && length <= 2 {
				break
			}
			kind := "format"
			if chr == "c" {
				kind = "stand-alone"
			}
			if length == 6 {
				if !jsTruthy(c.main([]any{"dates/calendars/gregorian/days", []any{kind}, "short"})) {
					c.main([]any{"dates/calendars/gregorian/days", []any{kind}, "abbreviated"})
				}
			} else {
				w := 0
				if length >= 3 {
					w = length - 3
				}
				c.main([]any{"dates/calendars/gregorian/days", []any{kind}, widthAt(w)})
			}
		case "a":
			c.main("dates/calendars/gregorian/dayPeriods/format/wide")
		case "V":
			if length == 1 {
				panic(createErrorUnsupportedFeature(featureObj("timezone pattern `" + pattern + "`")))
			}
			if jsTruthy(timeZone) {
				if length == 2 {
					p.timeZoneName = timeZone
					p.timeZoneNameRe = mustJSRegexp("^" + regexpEscape(jsString(timeZone)))
					break
				}
				var name any = Undefined
				exemplarCity := c.main([]any{"dates/timeZoneNames/zone", timeZone, "exemplarCity"})
				if length == 3 {
					if !jsTruthy(exemplarCity) {
						exemplarCity = c.main([]any{"dates/timeZoneNames/zone/Etc/Unknown/exemplarCity"})
					}
					name = exemplarCity
				}
				if jsTruthy(exemplarCity) && length == 4 {
					name = formatMessage(requireString(c.main("dates/timeZoneNames/regionFormat"), "message", "replace"), []any{exemplarCity})
				}
				if jsTruthy(name) {
					n := looseMatchingAny(name)
					p.timeZoneName = n
					p.timeZoneNameRe = mustJSRegexp("^" + regexpEscape(n))
				}
			}
			if current == "v" {
				length = 1
			}
			fallO = true
		case "z", "O":
			fallO = true
		case "X", "x":
			fallX = true
		}
		if fallO {
			gmtFormat := c.main("dates/timeZoneNames/gmtFormat")
			c.main("dates/timeZoneNames/gmtZeroFormat")
			c.main("dates/timeZoneNames/hourFormat")
			p.gmtZeroFormatRe = mustJSRegexp("^" + regexpEscape(requireString(vals["timeZoneNames/gmtZeroFormat"], "string", "replace")))
			aux, isStr := vals["timeZoneNames/hourFormat"].(string)
			if !isStr {
				requireString(vals["timeZoneNames/hourFormat"], "hourFormat", "split")
			}
			var formats []string
			if length < 4 {
				formats = []string{dateTimezoneHourFormatHm(aux, "H"), dateTimezoneHourFormatH(aux)}
			} else {
				formats = []string{dateTimezoneHourFormatHm(aux, "HH")}
			}
			res := make([]*jsRegexp, len(formats))
			for i, f := range formats {
				res[i] = hourFormatRe(f, gmtFormat, digitsReSource, timeSeparator)
			}
			vals["timeZoneNames/hourFormat"] = res
			fallX = true
		}
		if fallX {
			if length < 1 || length > len(xHourFormatsSet) {
				throwTypeError("Cannot read properties of undefined (reading 'map')")
			}
			set := xHourFormatsSet[length-1]
			p.x = make([]*jsRegexp, len(set))
			for i, f := range set {
				p.x[i] = hourFormatRe(f, nil, "", nil)
			}
		}
	}
	c.off(key)
	return p
}

type dateToken struct {
	typ    string
	value  any
	lexeme string
}

func dateTokenizer(input string, numberParser func(any) float64, p *dateTokProps) []*dateToken {
	digitsRe := p.digitsRe
	value := looseMatching(input)
	toks := datePatternMatch(p.pattern)
	if toks == nil {
		throwTypeError("Cannot read properties of null (reading 'every')")
	}
	var tokens []*dateToken
	widths := dateWidths
	valid := true
	for _, current := range toks {
		token := &dateToken{typ: current, value: Undefined}
		var tokenRe *jsRegexp
		numeric := false
		chr := charAt0(current)
		length := jsLength(current)
		hourFormatParse := func(re *jsRegexp, parse func(any) float64) bool {
			m := re.re.FindStringSubmatch(value)
			if m == nil {
				return false
			}
			idx := re.re.FindStringSubmatchIndex(value)
			group := func(i int) any {
				if idx[2*i] < 0 {
					return Undefined
				}
				return m[i]
			}
			if parse == nil {
				parse = func(v any) float64 { return toNumber(v) }
			}
			isPositive := jsTruthy(group(1))
			var v float64
			switch {
			case len(m) < 6:
				a := 3
				if isPositive {
					a = 1
				}
				v = parse(group(a)) * 60
			case len(m) < 10:
				a := [2]int{5, 7}
				if isPositive {
					a = [2]int{1, 3}
				}
				v = float64(parse(group(a[0]))*60) + parse(group(a[1]))
			default:
				a := [3]int{7, 9, 11}
				if isPositive {
					a = [3]int{1, 3, 5}
				}
				v = float64(parse(group(a[0]))*60) + parse(group(a[1])) + parse(group(a[2]))/60
			}
			if isPositive {
				v *= -1
			}
			token.value = v
			return true
		}
		digitsTimes := func(q string) *jsRegexp {
			numeric = true
			tokenRe = mustJSRegexp("^(" + digitsRe.source + ")" + q)
			return tokenRe
		}
		oneDigitIfLengthOne := func() bool {
			if length == 1 {
				numeric = true
				tokenRe = digitsRe
				return true
			}
			return false
		}
		oneOrTwoDigitsIfLengthOne := func() bool {
			if length == 1 {
				digitsTimes("{1,2}")
				return true
			}
			return false
		}
		oneOrTwoDigitsIfLengthOneOrTwo := func() bool {
			if length == 1 || length == 2 {
				digitsTimes("{1,2}")
				return true
			}
			return false
		}
		twoDigitsIfLengthTwo := func() bool {
			if length == 2 {
				digitsTimes("{2}")
				return true
			}
			return false
		}
		lookup := func(path ...any) {
			var segs []string
			for _, s := range path {
				if isUndefined(s) {
					segs = append(segs, "")
				} else {
					segs = append(segs, jsString(s))
				}
			}
			array, ok := vals(p, strings.Join(segs, "/"))
			if !ok {
				return
			}
			items, isItems := array.([]lookupItem)
			if !isItems {
				throwTypeError("array.some is not a function")
			}
			for _, item := range items {
				if item.re.test(value) {
					token.value = item.key
					tokenRe = item.re
					return
				}
			}
		}
		widthOf := func(i int) any {
			if i >= 0 && i < len(widths) {
				return widths[i]
			}
			return Undefined
		}
		if chr == "Z" {
			switch {
			case length < 4:
				chr, length = "x", 4
			case length < 5:
				chr, length = "O", 4
			default:
				chr, length = "X", 5
			}
		}
		if chr == "z" && p.standardOrDaylightTzName != nil {
			token.value = nil
			tokenRe = p.standardOrDaylightTzName
		}
		if chr == "v" {
			if p.genericTzName != nil {
				token.value = nil
				tokenRe = p.genericTzName
			} else {
				chr, length = "V", 4
			}
		}
		if chr == "V" && jsTruthy(p.timeZoneName) {
			if length == 2 {
				token.value = p.timeZoneName
			} else {
				token.value = nil
			}
			tokenRe = p.timeZoneNameRe
		}
		stop := false
		fallO, fallX := false, false
		switch chr {
		case "G":
			kind := "eraNarrow"
			if length <= 3 {
				kind = "eraAbbr"
			} else if length == 4 {
				kind = "eraNames"
			}
			lookup("gregorian/eras", kind)
		case "y", "Y":
			switch length {
			case 1:
				digitsTimes("+")
			case 2:
				digitsTimes("{1,2}")
			default:
				digitsTimes("{" + numToString(float64(length)) + ",}")
			}
		case "Q", "q":
			if !oneDigitIfLengthOne() && !twoDigitsIfLengthTwo() {
				kind := "stand-alone"
				if chr == "Q" {
					kind = "format"
				}
				lookup("gregorian/quarters", kind, widthOf(length-3))
			}
		case "M", "L":
			if !oneOrTwoDigitsIfLengthOneOrTwo() {
				kind := "stand-alone"
				if chr == "M" {
					kind = "format"
				}
				lookup("gregorian/months", kind, widthOf(length-3))
			}
		case "D":
			if length <= 3 {
				digitsTimes("{" + numToString(float64(length)) + ",3}")
			}
		case "W", "F":
			oneDigitIfLengthOne()
		case "e", "c", "E":
			if (chr == "e" || chr == "c") && length <= 2 {
				if !oneDigitIfLengthOne() {
					twoDigitsIfLengthTwo()
				}
				break
			}
			kind := "format"
			if chr == "c" {
				kind = "stand-alone"
			}
			if length == 6 {
				lookup("gregorian/days", kind, "short")
				lookup("gregorian/days", kind, "abbreviated")
			} else {
				w := 0
				if length >= 3 {
					w = length - 3
				}
				lookup("gregorian/days", kind, widthOf(w))
			}
		case "a":
			lookup("gregorian/dayPeriods/format/wide")
		case "w":
			if !oneOrTwoDigitsIfLengthOne() {
				twoDigitsIfLengthTwo()
			}
		case "d", "h", "H", "K", "k", "j", "m", "s":
			oneOrTwoDigitsIfLengthOneOrTwo()
		case "S":
			digitsTimes("{" + numToString(float64(length)) + "}")
		case "A":
			digitsTimes("{" + numToString(float64(length+5)) + "}")
		case "v", "V", "z":
			if tokenRe != nil && tokenRe.test(value) {
				break
			}
			if chr == "V" && length == 2 {
				break
			}
			fallO = true
		case "O":
			fallO = true
		case "X":
			if value == "Z" {
				token.value = 0.0
				tokenRe = mustJSRegexp("^Z")
				break
			}
			fallX = true
		case "x":
			fallX = true
		case "'":
			token.typ = "literal"
			tokenRe = mustJSRegexp("^" + regexpEscape(removeLiteralQuotes(current)))
		default:
			token.typ = "literal"
			tokenRe = mustJSRegexp("^" + regexpEscape(current))
		}
		if fallO {
			if gz, ok := vals(p, "timeZoneNames/gmtZeroFormat"); ok && value == gz {
				token.value = 0.0
				tokenRe = p.gmtZeroFormatRe
			} else {
				hf, _ := vals(p, "timeZoneNames/hourFormat")
				res, ok := hf.([]*jsRegexp)
				if !ok {
					throwTypeError("Cannot read properties of undefined (reading 'some')")
				}
				found := false
				for _, re := range res {
					if hourFormatParse(re, numberParser) {
						tokenRe = re
						found = true
						break
					}
				}
				if !found {
					stop = true
				}
			}
		}
		if fallX {
			if p.x == nil {
				throwTypeError("Cannot read properties of undefined (reading 'some')")
			}
			found := false
			for _, re := range p.x {
				if hourFormatParse(re, nil) {
					tokenRe = re
					found = true
					break
				}
			}
			if !found {
				stop = true
			}
		}
		if stop || tokenRe == nil {
			valid = false
			break
		}
		if loc := tokenRe.find(value); loc != nil {
			token.lexeme = value[loc[0]:loc[1]]
			if numeric {
				token.value = numberParser(token.lexeme)
			}
			value = value[:loc[0]] + value[loc[1]:]
		}
		if token.lexeme == "" {
			valid = false
			break
		}
		if numeric && math.IsNaN(toNumber(token.value)) {
			valid = false
			break
		}
		tokens = append(tokens, token)
	}
	if value != "" {
		valid = false
	}
	if !valid {
		return nil
	}
	return tokens
}

func vals(p *dateTokProps, key string) (any, bool) {
	v, ok := p.vals[key]
	return v, ok
}

type dateParseProps struct {
	preferredTimeData any
	timeZoneData      *tzData
}

func dateParseProperties(c *Cldr, timeZone any) *dateParseProps {
	p := &dateParseProps{preferredTimeData: c.timeDataPreferred()}
	if jsTruthy(timeZone) {
		d := ianaData(c, timeZone)
		p.timeZoneData = &d
	}
	return p
}

func outOfRange(value any, low, high float64) bool {
	v := toNumber(value)
	return v < low || v > high
}

func dateLastDayOfMonth(d dateLike) float64 {
	return newJSDate(newLocalDate(d.getFullYear(), d.getMonth()+1, 0)).getDate()
}

func dateIsLeapYear(year float64) bool {
	return newJSDate(newLocalDate(year, 1, 29)).getMonth() == 1
}

func dateSetDate(d dateLike, day float64) {
	lastDay := dateLastDayOfMonth(d)
	switch {
	case day < 1:
		d.setDate(1)
	case day < lastDay:
		d.setDate(day)
	default:
		d.setDate(lastDay)
	}
}

func dateSetMonth(d dateLike, month float64) {
	originalDate := d.getDate()
	d.setDate(1)
	d.setMonth(month)
	dateSetDate(d, originalDate)
}

var hourBounds = map[string][2]float64{"h": {1, 12}, "K": {0, 11}, "k": {1, 24}, "H": {0, 23}}

var dateUnits = []string{"year", "month", "day", "hour", "minute", "second", "milliseconds"}

func dateParse(tokens []*dateToken, p *dateParseProps) any {
	var d dateLike = newJSDate(float64(Now()))
	if p.timeZoneData != nil {
		d = newZonedDateTime(d.getTime(), *p.timeZoneData)
	}
	if len(tokens) == 0 {
		return nil
	}
	var amPm, day, daysOfYear, month, era, timezoneOffset any = Undefined, Undefined, Undefined, Undefined, Undefined, Undefined
	hour, hour12 := false, false
	var truncateAt []int
	const (
		year = iota
		monthU
		dayU
		hourU
		minuteU
		secondU
		millisecondsU
	)
	for _, token := range tokens {
		if token.typ == "literal" {
			continue
		}
		chr := charAt0(token.typ)
		length := jsLength(token.typ)
		if chr == "j" {
			chr = jsString(p.preferredTimeData)
		}
		ok := true
		switch chr {
		case "G":
			truncateAt = append(truncateAt, year)
			era = toNumber(token.value)
		case "y":
			value := toNumber(token.value)
			if length == 2 {
				if outOfRange(value, 0, 99) {
					ok = false
					break
				}
				century := math.Floor(d.getFullYear()/100) * 100
				value += century
				if value > d.getFullYear()+20 {
					value -= 100
				}
			}
			d.setFullYear(value)
			truncateAt = append(truncateAt, year)
		case "Y":
			panic(createErrorUnsupportedFeature(featureObj("year pattern `" + chr + "`")))
		case "M", "L":
			value := toNumber(token.value)
			if outOfRange(value, 1, 12) {
				ok = false
				break
			}
			month = value
			truncateAt = append(truncateAt, monthU)
		case "d":
			day = token.value
			truncateAt = append(truncateAt, dayU)
		case "D":
			daysOfYear = token.value
			truncateAt = append(truncateAt, dayU)
		case "a":
			amPm = token.value
		case "h", "K", "k", "H":
			value := toNumber(token.value)
			bounds := hourBounds[chr]
			if outOfRange(value, bounds[0], bounds[1]) {
				ok = false
				break
			}
			hour = true
			if chr == "h" || chr == "K" {
				hour12 = true
			}
			if (chr == "h" && value == 12) || (chr == "k" && value == 24) {
				value = 0
			}
			d.setHours(value)
			truncateAt = append(truncateAt, hourU)
		case "m":
			value := toNumber(token.value)
			if outOfRange(value, 0, 59) {
				ok = false
				break
			}
			d.setMinutes(value)
			truncateAt = append(truncateAt, minuteU)
		case "s":
			value := toNumber(token.value)
			if outOfRange(value, 0, 59) {
				ok = false
				break
			}
			d.setSeconds(value)
			truncateAt = append(truncateAt, secondU)
		case "A", "S":
			if chr == "A" {
				d.setHours(0)
				d.setMinutes(0)
				d.setSeconds(0)
			}
			d.setMilliseconds(mathRound(float64(toNumber(token.value) * pow10(3-length))))
			truncateAt = append(truncateAt, millisecondsU)
		case "z", "Z", "O", "v", "V", "X", "x":
			if n, isNum := token.value.(float64); isNum {
				timezoneOffset = n
			}
		}
		if !ok {
			return nil
		}
	}
	if hour && jsTruthy(amPm) != hour12 {
		return nil
	}
	if e, isNum := era.(float64); isNum && e == 0 {
		d.setFullYear(float64(d.getFullYear()*-1) + 1)
	}
	if !isUndefined(month) {
		dateSetMonth(d, toNumber(month)-1)
	}
	if !isUndefined(day) {
		if outOfRange(day, 1, dateLastDayOfMonth(d)) {
			return nil
		}
		d.setDate(toNumber(day))
	} else if !isUndefined(daysOfYear) {
		limit := 365.0
		if dateIsLeapYear(d.getFullYear()) {
			limit = 366
		}
		if outOfRange(daysOfYear, 1, limit) {
			return nil
		}
		d.setMonth(0)
		d.setDate(toNumber(daysOfYear))
	}
	if hour12 && amPm == "pm" {
		d.setHours(d.getHours() + 12)
	}
	if !isUndefined(timezoneOffset) {
		d.setMinutes(d.getMinutes() + timezoneOffset.(float64) - d.getTimezoneOffset())
	}
	unit := ""
	if len(truncateAt) > 0 {
		m := truncateAt[0]
		for _, t := range truncateAt[1:] {
			if t > m {
				m = t
			}
		}
		unit = dateUnits[m]
	}
	d = dateStartOf(d, unit)
	if z, isZoned := d.(*zonedDateTime); isZoned {
		return Date(z.toDate())
	}
	return Date(d.getTime())
}

func (g *Globalize) dateParser(options any) func(any) any {
	validateParameterTypePlainObject(options, "options")
	c := g.cldr
	o := optionsObject(options)
	if !optionsHasStyle(o) {
		o.set("skeleton", "yMd")
	}
	validateOptionsPreset(o)
	validateDefaultLocale(c)
	timeZone := o.prop("timeZone")
	validateParameterTypeString(timeZone, "options.timeZone")
	var tp *dateTokProps
	var pp *dateParseProps
	func() {
		defer c.off(keyValidateRequiredCldr)
		c.on(keyValidateRequiredCldr, validateRequiredCldr)
		if jsTruthy(timeZone) {
			c.on(new(int), validateRequiredIana(timeZone))
		}
		pattern := dateExpandPattern(o, c)
		validateOptionsSkeleton(pattern, o.prop("skeleton"))
		tp = dateTokenizerProperties(pattern, c, timeZone)
		pp = dateParseProperties(c, timeZone)
	}()
	numberParser := g.numberParser(attrs("raw", "0"))
	return func(value any) any {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeString(value, "value")
		tokens := dateTokenizer(value.(string), numberParser, tp)
		return dateParse(tokens, pp)
	}
}

func parsedDate(v any) *Date {
	if d, ok := v.(Date); ok {
		return &d
	}
	return nil
}

// DateParser is .dateParser([options]); the parser returns nil where Globalize returns null.
func (g *Globalize) DateParser(options ...any) func(value any) *Date {
	parse := g.dateParser(opt(options))
	return func(value any) *Date { return parsedDate(parse(value)) }
}

// ParseDate is .parseDate(value[, options]).
func (g *Globalize) ParseDate(value any, options ...any) *Date {
	v := fromPublic(value)
	validateParameterPresence(v, "value")
	validateParameterTypeString(v, "value")
	return parsedDate(g.dateParser(opt(options))(value))
}

// DateParser is Globalize.dateParser([options]) on the default locale.
func DateParser(options ...any) func(value any) *Date { return statics.DateParser(options...) }

// ParseDate is Globalize.parseDate(value[, options]).
func ParseDate(value any, options ...any) *Date { return statics.ParseDate(value, options...) }
