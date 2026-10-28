package globalize

import (
	"math"
	"regexp"
	"sort"
	"strings"
)

func createErrorInvalidParameterValue(name, value any) *Error {
	return createError("E_INVALID_PAR_VALUE", "Invalid `{name}` value ({value}).", attrs("name", name, "value", value))
}

const skeletonFields = "GyYuUrQqMLlwWEecdDFghHKkmsSAzZOvVXx"

func validateSkeleton(skeleton string) {
	units := utf16Units(skeleton)
	for _, u := range units {
		if u >= 0x80 || !strings.ContainsRune(skeletonFields, rune(u)) {
			panic(createError("E_INVALID_OPTIONS", "Invalid field `{invalidField}` of skeleton `{value}`",
				attrs("invalidField", fromUnits([]uint16{u}), "type", "skeleton", "value", skeleton)))
		}
	}
	last := -1
	for _, u := range units {
		pos := strings.IndexRune(skeletonFields, rune(u))
		if last >= 0 && pos < last {
			panic(createError("E_INVALID_OPTIONS", "Invalid order `{invalidField}` of skeleton `{value}`",
				attrs("invalidField", string(rune(u)), "type", "skeleton", "value", skeleton)))
		}
		last = pos
	}
}

func normalizePatternType(ch string) string {
	switch ch {
	case "e", "E", "c":
		return "e"
	case "M", "L":
		return "L"
	}
	return ch
}

func isASCIILetter(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

func asciiLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 32
	}
	return c
}

// datePatternMatch is pattern.match(/([a-z])\1*|'([^']|”)+'|”|./gi); nil when nothing matches.
func datePatternMatch(s string) []string {
	var toks []string
	for i := 0; i < len(s); {
		c := s[i]
		if isASCIILetter(c) {
			j := i + 1
			for j < len(s) && isASCIILetter(s[j]) && asciiLower(s[j]) == asciiLower(c) {
				j++
			}
			toks = append(toks, s[i:j])
			i = j
			continue
		}
		if end := quotedLiteralEnd(s, i); end > 0 {
			toks = append(toks, s[i:end])
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "''") {
			toks = append(toks, "''")
			i += 2
			continue
		}
		r, size := decodeRune(s[i:])
		if !isJSLineTerminator(r) {
			toks = append(toks, s[i:i+size])
		}
		i += size
	}
	return toks
}

// charAt0 is token.charAt(0) for the tokens datePatternMatch returns.
func charAt0(tok string) string {
	u := utf16Units(tok)
	if len(u) == 0 {
		return ""
	}
	return fromUnits(u[:1])
}

func expandBestMatchFormat(skeleton, bestMatchFormat string) string {
	requested := datePatternMatch(skeleton)
	if requested == nil {
		throwTypeError("Cannot read properties of null (reading 'length')")
	}
	parts := datePatternMatch(bestMatchFormat)
	for i := range parts {
		matchedType := charAt0(parts[i])
		matchedLength := jsLength(parts[i])
		for _, req := range requested {
			if normalizePatternType(matchedType) == normalizePatternType(charAt0(req)) && matchedLength < jsLength(req) {
				parts[i] = strings.Repeat(matchedType, jsLength(req))
			}
		}
	}
	return strings.Join(parts, "")
}

func dateExpandPatternAugmentFormat(requestedSkeleton, bestMatchFormat string, decimalSeparator any) string {
	count := strings.Count(requestedSkeleton, "S")
	withoutS := strings.ReplaceAll(requestedSkeleton, "S", "")
	bestMatchFormat = expandBestMatchFormat(withoutS, bestMatchFormat)
	if idx := strings.LastIndex(bestMatchFormat, "s"); idx != -1 && count != 0 {
		bestMatchFormat = bestMatchFormat[:idx+1] + jsString(decimalSeparator) + strings.Repeat("S", count) + bestMatchFormat[idx+1:]
	}
	return bestMatchFormat
}

func dateExpandPatternCompareFormats(formatA, formatB string) int {
	if formatA == formatB {
		return 0
	}
	a := datePatternMatch(formatA)
	b := datePatternMatch(formatB)
	if len(a) != len(b) {
		return -1
	}
	distance := 1
	for i := range a {
		ca := charAt0(a[i])
		typeA := normalizePatternType(ca)
		j := -1
		var cb string
		for k := range b {
			cb = charAt0(b[k])
			if typeA == normalizePatternType(cb) {
				j = k
				break
			}
		}
		if j == -1 {
			return -1
		}
		lenA, lenB := jsLength(a[i]), jsLength(b[j])
		if lenA > lenB {
			distance += lenA - lenB
		} else {
			distance += lenB - lenA
		}
		if ca != cb {
			distance++
		}
		if lenA < 3 && lenB >= 3 || lenA >= 3 && lenB < 3 {
			distance += 20
		}
	}
	return distance
}

const availableFormatsPath = "dates/calendars/gregorian/dateTimeFormats/availableFormats"

func dateExpandPatternGetBestMatchPattern(c *Cldr, askedSkeleton string) any {
	pattern := c.main([]any{availableFormatsPath, askedSkeleton})
	if askedSkeleton != "" && !jsTruthy(pattern) {
		available := c.main([]any{availableFormatsPath})
		type rated struct {
			pattern any
			rate    int
		}
		var ratedFormats []rated
		forIn(available, func(skeleton string, p any) {
			ratedFormats = append(ratedFormats, rated{p, dateExpandPatternCompareFormats(askedSkeleton, skeleton)})
		})
		filtered := ratedFormats[:0]
		for _, f := range ratedFormats {
			if f.rate > -1 {
				filtered = append(filtered, f)
			}
		}
		sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].rate < filtered[j].rate })
		if len(filtered) > 0 {
			decimalSeparator := numberSymbol("decimal", c)
			pattern = dateExpandPatternAugmentFormat(askedSkeleton, jsString(filtered[0].pattern), decimalSeparator)
		}
	}
	return pattern
}

var (
	timeSkeletonSplitRe = regexp.MustCompile(`[^hHKkmsSAzZOvVXx]`)
	dateSkeletonSplitRe = regexp.MustCompile(`[^GyYuUrQqMLlwWdDFgEec]`)
	fullTypeRe          = regexp.MustCompile(`(MMMM|LLLL).*[Ec]`)
	longTypeRe          = regexp.MustCompile(`MMMM|LLLL`)
	mediumTypeRe        = regexp.MustCompile(`MMM|LLL`)
)

func dateExpandPattern(options *object, c *Cldr) any {
	combine := func(typ, datePattern, timePattern any) any {
		msg := requireString(c.main([]any{"dates/calendars/gregorian/dateTimeFormats", typ}), "message", "replace")
		return formatMessage(msg, []any{timePattern, datePattern})
	}
	switch {
	case options.has("skeleton"):
		sk := requireString(options.prop("skeleton"), "skeleton", "replace")
		if strings.Contains(sk, "j") {
			pieces := strings.Split(sk, "j")
			var b strings.Builder
			b.WriteString(pieces[0])
			for _, piece := range pieces[1:] {
				b.WriteString(jsString(c.timeDataPreferred()))
				b.WriteString(piece)
			}
			sk = b.String()
		}
		validateSkeleton(sk)
		result := dateExpandPatternGetBestMatchPattern(c, sk)
		if jsTruthy(result) {
			return result
		}
		timeParts := timeSkeletonSplitRe.Split(sk, -1)
		timeSkeleton := timeParts[len(timeParts)-1]
		dateSkeleton := dateSkeletonSplitRe.Split(sk, -1)[0]
		datePattern := dateExpandPatternGetBestMatchPattern(c, dateSkeleton)
		timePattern := dateExpandPatternGetBestMatchPattern(c, timeSkeleton)
		ds := jsString(datePattern)
		var typ string
		switch {
		case fullTypeRe.MatchString(ds):
			typ = "full"
		case longTypeRe.MatchString(ds):
			typ = "long"
		case mediumTypeRe.MatchString(ds):
			typ = "medium"
		default:
			typ = "short"
		}
		if jsTruthy(datePattern) && jsTruthy(timePattern) {
			return combine(typ, datePattern, timePattern)
		}
		if jsTruthy(datePattern) {
			return datePattern
		}
		return timePattern
	case options.has("date") || options.has("time"):
		kind := "timeFormats"
		if options.has("date") {
			kind = "dateFormats"
		}
		v := options.prop("date")
		if !jsTruthy(v) {
			v = options.prop("time")
		}
		return c.main([]any{"dates/calendars/gregorian", kind, v})
	case options.has("datetime"):
		dt := options.prop("datetime")
		return combine(dt, c.main([]any{"dates/calendars/gregorian/dateFormats", dt}), c.main([]any{"dates/calendars/gregorian/timeFormats", dt}))
	case options.has("raw"):
		return options.prop("raw")
	}
	panic(createErrorInvalidParameterValue(attrs("name", "options", "value", options), Undefined))
}

var dateWeekDays = []string{"sun", "mon", "tue", "wed", "thu", "fri", "sat"}

func dateFirstDayOfWeek(c *Cldr) float64 {
	fd := c.weekDataFirstDay()
	for i, d := range dateWeekDays {
		if fd == d {
			return float64(i)
		}
	}
	return -1
}

func dateGetTimeZoneName(length int, typ string, timeZone any, c *Cldr) any {
	if !jsTruthy(timeZone) {
		return Undefined
	}
	form := "long"
	if length < 4 {
		form = "short"
	}
	if r := c.main([]any{"dates/timeZoneNames/zone", timeZone, form, typ}); jsTruthy(r) {
		return r
	}
	metaZone := c.supplemental([]any{"metaZones/metazoneInfo/timezone", timeZone, 0.0, "usesMetazone/_mzone"})
	return c.main([]any{"dates/timeZoneNames/metazone", metaZone, form, typ})
}

func dateTimezoneHourFormatH(hourFormat string) string {
	parts := strings.Split(hourFormat, ";")
	for i, f := range parts {
		parts[i] = f[:strings.Index(f, "H")+1]
	}
	return strings.Join(parts, ";")
}

var hRunRe = regexp.MustCompile(`H+`)

func dateTimezoneHourFormatHm(hourFormat, hFormat string) string {
	parts := strings.Split(hourFormat, ";")
	for i, f := range parts {
		sub := hRunRe.Split(f, -1)
		out := append([]string{sub[0], hFormat}, sub[1:]...)
		parts[i] = strings.Join(out, "")
	}
	return strings.Join(parts, ";")
}

func ianaData(c *Cldr, timeZone any) tzData {
	return tzData{
		offsets: c.get([]any{"globalize-iana/zoneData", timeZone, "offsets"}),
		untils:  c.get([]any{"globalize-iana/zoneData", timeZone, "untils"}),
		isdsts:  c.get([]any{"globalize-iana/zoneData", timeZone, "isdsts"}),
	}
}

type dateFormatProps struct {
	numberFormatters map[int]string
	pattern          string
	timeSeparator    any
	timeZoneData     *tzData
	preferredTime    any
	standardTzName   any
	daylightTzName   any
	genericTzName    any
	timeZoneName     any
	eras             any
	firstDay         float64
	minDays          float64
	quarters         map[string]map[int]any
	months           map[string]map[int]any
	days             map[string]map[int]any
	dayPeriods       *object
	gmtFormat        any
	gmtZeroFormat    any
	hourFormat       any
}

var dateWidths = []string{"abbreviated", "wide", "narrow"}

func widthAt(i int) any {
	if i >= 0 && i < len(dateWidths) {
		return dateWidths[i]
	}
	return Undefined
}

func dateFormatProperties(patternArg any, c *Cldr, timeZone any) *dateFormatProps {
	p := &dateFormatProps{numberFormatters: map[int]string{}, timeSeparator: numberSymbol("timeSeparator", c),
		standardTzName: Undefined, daylightTzName: Undefined, genericTzName: Undefined, timeZoneName: Undefined}
	setPad := func(pad int) { p.numberFormatters[pad] = stringPad("", float64(pad), false) }
	if jsTruthy(timeZone) {
		d := ianaData(c, timeZone)
		p.timeZoneData = &d
	}
	pattern := requireString(patternArg, "pattern", "replace")
	p.pattern = pattern
	for _, current := range datePatternMatch(pattern) {
		chr := charAt0(current)
		length := jsLength(current)
		formatNumber := false
		var genericTzName any = Undefined
		if chr == "j" {
			p.preferredTime = c.timeDataPreferred()
			chr = jsString(p.preferredTime)
		}
		if chr == "Z" && length == 4 {
			chr, length = "O", 4
		}
		if chr == "z" {
			standard := dateGetTimeZoneName(length, "standard", timeZone, c)
			daylight := dateGetTimeZoneName(length, "daylight", timeZone, c)
			if jsTruthy(standard) {
				p.standardTzName = standard
			}
			if jsTruthy(daylight) {
				p.daylightTzName = daylight
			}
			if !jsTruthy(standard) || !jsTruthy(daylight) {
				chr = "O"
				if length < 4 {
					length = 1
				}
			}
		}
		if chr == "v" {
			genericTzName = dateGetTimeZoneName(length, "generic", timeZone, c)
			if !jsTruthy(genericTzName) {
				chr, length = "V", 4
			}
		}
		fallO := false
		switch chr {
		case "G":
			kind := "eraNarrow"
			if length <= 3 {
				kind = "eraAbbr"
			} else if length == 4 {
				kind = "eraNames"
			}
			p.eras = c.main([]any{"dates/calendars/gregorian/eras", kind})
		case "y":
			formatNumber = true
		case "Y":
			p.firstDay = dateFirstDayOfWeek(c)
			p.minDays = c.weekDataMinDays()
			formatNumber = true
		case "u", "U":
			panic(createErrorUnsupportedFeature(featureObj("year pattern `" + chr + "`")))
		case "Q", "q":
			if length > 2 {
				if p.quarters == nil {
					p.quarters = map[string]map[int]any{}
				}
				if p.quarters[chr] == nil {
					p.quarters[chr] = map[int]any{}
				}
				kind := "stand-alone"
				if chr == "Q" {
					kind = "format"
				}
				p.quarters[chr][length] = c.main([]any{"dates/calendars/gregorian/quarters", kind, widthAt(length - 3)})
			} else {
				formatNumber = true
			}
		case "M", "L":
			if length > 2 {
				if p.months == nil {
					p.months = map[string]map[int]any{}
				}
				if p.months[chr] == nil {
					p.months[chr] = map[int]any{}
				}
				kind := "stand-alone"
				if chr == "M" {
					kind = "format"
				}
				p.months[chr][length] = c.main([]any{"dates/calendars/gregorian/months", kind, widthAt(length - 3)})
			} else {
				formatNumber = true
			}
		case "w", "W":
			p.firstDay = dateFirstDayOfWeek(c)
			p.minDays = c.weekDataMinDays()
			formatNumber = true
		case "d", "D", "F":
			formatNumber = true
		case "g":
			panic(createErrorUnsupportedFeature(featureObj("Julian day pattern `g`")))
		case "e", "c", "E":
			if (chr == "e" || chr == "c") && length <= 2 {
				p.firstDay = dateFirstDayOfWeek(c)
				formatNumber = true
				break
			}
			if p.days == nil {
				p.days = map[string]map[int]any{}
			}
			if p.days[chr] == nil {
				p.days[chr] = map[int]any{}
			}
			kind := "format"
			if chr == "c" {
				kind = "stand-alone"
			}
			if length == 6 {
				v := c.main([]any{"dates/calendars/gregorian/days", kind, "short"})
				if !jsTruthy(v) {
					v = c.main([]any{"dates/calendars/gregorian/days", kind, "abbreviated"})
				}
				p.days[chr][length] = v
			} else {
				w := 0
				if length >= 3 {
					w = length - 3
				}
				p.days[chr][length] = c.main([]any{"dates/calendars/gregorian/days", kind, widthAt(w)})
			}
		case "a":
			dp := newObject()
			dp.set("am", c.main("dates/calendars/gregorian/dayPeriods/format/wide/am"))
			dp.set("pm", c.main("dates/calendars/gregorian/dayPeriods/format/wide/pm"))
			p.dayPeriods = dp
		case "h", "H", "K", "k", "m", "s", "S", "A":
			formatNumber = true
		case "v":
			if length != 1 && length != 4 {
				panic(createErrorUnsupportedFeature(featureObj("timezone pattern `" + pattern + "`")))
			}
			p.genericTzName = genericTzName
		case "V":
			if length == 1 {
				panic(createErrorUnsupportedFeature(featureObj("timezone pattern `" + pattern + "`")))
			}
			if jsTruthy(timeZone) {
				if length == 2 {
					p.timeZoneName = timeZone
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
					p.timeZoneName = name
					break
				}
			}
			if current == "v" {
				length = 1
			}
			fallO = true
		case "O":
			fallO = true
		case "Z", "X", "x":
			setPad(1)
			setPad(2)
		}
		if fallO {
			p.gmtFormat = c.main("dates/timeZoneNames/gmtFormat")
			p.gmtZeroFormat = c.main("dates/timeZoneNames/gmtZeroFormat")
			aux := jsString(c.main("dates/timeZoneNames/hourFormat"))
			if length < 4 {
				p.hourFormat = []string{dateTimezoneHourFormatH(aux), dateTimezoneHourFormatHm(aux, "H")}
			} else {
				p.hourFormat = dateTimezoneHourFormatHm(aux, "HH")
			}
			setPad(1)
			setPad(2)
		}
		if formatNumber {
			setPad(length)
		}
	}
	return p
}

func dateDayOfWeek(d dateLike, firstDay float64) float64 {
	return math.Mod(d.getDay()-firstDay+7, 7)
}

func dateStartOf(d dateLike, unit string) dateLike {
	var r dateLike
	if z, ok := d.(*zonedDateTime); ok {
		r = z.clone()
	} else {
		r = newJSDate(d.getTime())
	}
	switch unit {
	case "year":
		r.setMonth(0)
		fallthrough
	case "month":
		r.setDate(1)
		fallthrough
	case "day":
		r.setHours(0)
		fallthrough
	case "hour":
		r.setMinutes(0)
		fallthrough
	case "minute":
		r.setSeconds(0)
		fallthrough
	case "second":
		r.setMilliseconds(0)
	}
	return r
}

func dateDayOfYear(d dateLike) float64 {
	return math.Floor((d.getTime() - dateStartOf(d, "year").getTime()) / 864e5)
}

func dateTimezoneHourFormat(d dateLike, formatArg any, timeSeparator string, formatNumber map[int]func(any) string) string {
	format := requireString(formatArg, "format", "split")
	offset := d.getTimezoneOffset()
	absOffset := math.Abs(offset)
	if formatNumber == nil {
		formatNumber = map[int]func(any) string{
			1: func(v any) string { return stringPad(v, 1.0, false) },
			2: func(v any) string { return stringPad(v, 2.0, false) },
		}
	}
	parts := strings.Split(format, ";")
	idx := 0
	if offset > 0 {
		idx = 1
	}
	if idx >= len(parts) {
		throwTypeError("Cannot read properties of undefined (reading 'replace')")
	}
	s := strings.Replace(parts[idx], ":", jsReplacement(timeSeparator), 1)
	if loc := hhRe.FindStringIndex(s); loc != nil {
		s = s[:loc[0]] + formatNumber[loc[1]-loc[0]](math.Floor(absOffset/60)) + s[loc[1]:]
	}
	if i := strings.Index(s, "mm"); i >= 0 {
		s = s[:i] + formatNumber[2](math.Floor(math.Mod(absOffset, 60))) + s[i+2:]
	}
	if i := strings.Index(s, "ss"); i >= 0 {
		s = s[:i] + formatNumber[2](math.Floor(float64(math.Mod(absOffset, 1)*60))) + s[i+2:]
	}
	return s
}

var hhRe = regexp.MustCompile(`HH?`)

// jsReplacement expands the $ patterns a JavaScript replacement string may hold for a plain
// string search (only $$ can occur in the values Globalize passes).
func jsReplacement(s string) string { return strings.ReplaceAll(s, "$$", "$") }

var dateFieldsMap = map[string]string{
	"G": "era", "y": "year", "Y": "year", "q": "quarter", "Q": "quarter", "M": "month", "L": "month",
	"w": "week", "W": "week", "d": "day", "D": "day", "F": "day", "e": "weekday", "c": "weekday", "E": "weekday",
	"a": "dayperiod", "h": "hour", "H": "hour", "k": "hour", "K": "hour", "m": "minute", "s": "second", "S": "second",
	"A": "second", "z": "zone", "v": "zone", "V": "zone", "O": "zone", "x": "zone", "X": "zone",
}

func lookupIndex(v any, i float64) any { return getProp(v, numToString(i)) }

func dateFormat(value Date, numberFormatters map[int]func(any) string, p *dateFormatProps) []Part {
	var d dateLike = newJSDate(float64(value))
	var zoned *zonedDateTime
	if p.timeZoneData != nil {
		zoned = newZonedDateTime(float64(value), *p.timeZoneData)
		d = zoned
	}
	timeSeparator := jsString(p.timeSeparator)
	var parts []Part
	for _, current := range datePatternMatch(p.pattern) {
		chr := charAt0(current)
		length := jsLength(current)
		var v any = Undefined
		if chr == "j" {
			chr = jsString(p.preferredTime)
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
		if chr == "z" {
			if zoned != nil {
				if zoned.isDST() {
					v = p.daylightTzName
				} else {
					v = p.standardTzName
				}
			}
			if !jsTruthy(v) {
				chr = "O"
				if length < 4 {
					length = 1
				}
			}
		}
		fallO, fallX := false, false
		switch chr {
		case "G":
			idx := 1.0
			if d.getFullYear() < 0 {
				idx = 0
			}
			v = lookupIndex(p.eras, idx)
		case "y":
			y := d.getFullYear()
			v = y
			if length == 2 {
				s := numToString(y)
				v = stringToNumber(jsSubstrFrom(s, jsLength(s)-2))
			}
		case "Y":
			w := newJSDate(d.getTime())
			w.setDate(w.getDate() + 7 - dateDayOfWeek(d, p.firstDay) - p.firstDay - p.minDays)
			y := w.getFullYear()
			v = y
			if length == 2 {
				s := numToString(y)
				v = stringToNumber(jsSubstrFrom(s, jsLength(s)-2))
			}
		case "Q", "q":
			q := math.Ceil((d.getMonth() + 1) / 3)
			v = q
			if length > 2 {
				v = lookupIndex(p.quarters[chr][length], q)
			}
		case "M", "L":
			m := d.getMonth() + 1
			v = m
			if length > 2 {
				v = lookupIndex(p.months[chr][length], m)
			}
		case "w":
			dow := dateDayOfWeek(dateStartOf(d, "year"), p.firstDay)
			adj := 1.0
			if 7-dow >= p.minDays {
				adj = 0
			}
			v = math.Ceil((dateDayOfYear(d)+1+dow)/7) - adj
		case "W":
			dow := dateDayOfWeek(dateStartOf(d, "month"), p.firstDay)
			adj := 1.0
			if 7-dow >= p.minDays {
				adj = 0
			}
			v = math.Ceil((d.getDate()+dow)/7) - adj
		case "d":
			v = d.getDate()
		case "D":
			v = dateDayOfYear(d) + 1
		case "F":
			v = math.Floor(d.getDate()/7) + 1
		case "e", "c", "E":
			if (chr == "e" || chr == "c") && length <= 2 {
				v = dateDayOfWeek(d, p.firstDay) + 1
				break
			}
			day := Undefined
			if wd := d.getDay(); !math.IsNaN(wd) {
				day = dateWeekDays[int(wd)]
			}
			v = getProp(p.days[chr][length], jsString(day))
		case "a":
			key := "pm"
			if d.getHours() < 12 {
				key = "am"
			}
			v = p.dayPeriods.prop(key)
		case "h":
			h := math.Mod(d.getHours(), 12)
			if h == 0 || math.IsNaN(h) {
				h = 12
			}
			v = h
		case "H":
			v = d.getHours()
		case "K":
			v = math.Mod(d.getHours(), 12)
		case "k":
			h := d.getHours()
			if h == 0 || math.IsNaN(h) {
				h = 24
			}
			v = h
		case "m":
			v = d.getMinutes()
		case "s":
			v = d.getSeconds()
		case "S":
			v = mathRound(float64(d.getMilliseconds() * pow10(length-3)))
		case "A":
			v = mathRound(float64((d.getTime() - dateStartOf(d, "day").getTime()) * pow10(length-3)))
		case "z":
		case "v":
			if jsTruthy(p.genericTzName) {
				v = p.genericTzName
				break
			}
			if jsTruthy(p.timeZoneName) {
				v = p.timeZoneName
				break
			}
			if current == "v" {
				length = 1
			}
			fallO = true
		case "V":
			if jsTruthy(p.timeZoneName) {
				v = p.timeZoneName
				break
			}
			if current == "v" {
				length = 1
			}
			fallO = true
		case "O":
			fallO = true
		case "X":
			if d.getTimezoneOffset() == 0 {
				v = "Z"
				break
			}
			fallX = true
		case "x":
			fallX = true
		case ":":
			v = timeSeparator
		case "'":
			v = removeLiteralQuotes(current)
		default:
			v = current
		}
		if fallO {
			if d.getTimezoneOffset() == 0 {
				v = p.gmtZeroFormat
			} else {
				var format any
				if length < 4 {
					aux := d.getTimezoneOffset()
					i := 1
					if math.Mod(aux, 60)-math.Mod(aux, 1) == 0 {
						i = 0
					}
					if hf, ok := p.hourFormat.([]string); ok {
						format = hf[i]
					} else {
						format = getProp(p.hourFormat, numToString(float64(i)))
					}
				} else {
					format = p.hourFormat
				}
				s := dateTimezoneHourFormat(d, format, timeSeparator, numberFormatters)
				v = strings.Replace(jsString(p.gmtFormat), "{0}", jsReplacement(s), 1)
			}
		}
		if fallX {
			aux := d.getTimezoneOffset()
			if length == 1 && math.Mod(aux, 60)-math.Mod(aux, 1) != 0 {
				length++
			}
			if (length == 4 || length == 5) && math.Mod(aux, 1) == 0 {
				length -= 2
			}
			formats := []string{"+HH;-HH", "+HHmm;-HHmm", "+HH:mm;-HH:mm", "+HHmmss;-HHmmss", "+HH:mm:ss;-HH:mm:ss"}
			var format any = Undefined
			if length >= 1 && length <= len(formats) {
				format = formats[length-1]
			}
			v = dateTimezoneHourFormat(d, format, ":", nil)
		}
		if n, ok := v.(float64); ok {
			f := numberFormatters[length]
			if f == nil {
				throwTypeError("numberFormatters[length] is not a function")
			}
			v = f(n)
		}
		typ := "literal"
		if f, ok := dateFieldsMap[chr]; ok {
			typ = f
		}
		parts = partsPushAny(parts, typ, v)
	}
	return parts
}

// partsPushAny is partsPush with a value that may not be a string.
func partsPushAny(parts []Part, typ string, v any) []Part {
	if s, ok := v.(string); ok {
		return partsPush(parts, typ, s)
	}
	if len(parts) > 0 && parts[len(parts)-1].Type == typ {
		parts[len(parts)-1].Value += jsString(v)
		return parts
	}
	return append(parts, Part{Type: typ})
}

// jsSubstrFrom is s.substr(start) with start possibly negative.
func jsSubstrFrom(s string, start int) string {
	u := utf16Units(s)
	if start < 0 {
		start += len(u)
		if start < 0 {
			start = 0
		}
	}
	if start > len(u) {
		return ""
	}
	return fromUnits(u[start:])
}

func optionsHasStyle(o *object) bool {
	for _, k := range []string{"skeleton", "date", "time", "datetime", "raw"} {
		if !isUndefined(o.prop(k)) {
			return true
		}
	}
	return false
}

var requiredCldrSkip = []*regexp.Regexp{
	regexp.MustCompile(`dates/calendars/gregorian/dateTimeFormats/availableFormats`),
	regexp.MustCompile(`dates/calendars/gregorian/days/.*/short`),
	regexp.MustCompile(`dates/timeZoneNames/zone`),
	regexp.MustCompile(`dates/timeZoneNames/metazone`),
	regexp.MustCompile(`globalize-iana`),
	regexp.MustCompile(`supplemental/metaZones`),
	notFollowedBy("supplemental/timeData/", "001"),
	notFollowedBy("supplemental/weekData/", "001"),
}

// notFollowedBy builds the equivalent of /prefix(?!next)/ for MatchString.
func notFollowedBy(prefix, next string) *regexp.Regexp {
	return regexp.MustCompile(regexp.QuoteMeta(prefix) + `(?:$|[^0]|0$|0[^0]|00$|00[^1])`)
}

func validateRequiredCldr(path string, value any) { validateCldr(path, value, requiredCldrSkip) }

func validateOptionsPreset(o *object) {
	for _, typ := range []string{"date", "time", "datetime"} {
		v := o.prop(typ)
		ok := isUndefined(v) || v == "short" || v == "medium" || v == "long" || v == "full"
		validate("E_INVALID_OPTIONS", "Invalid `{{type}: \"{value}\"}`.", ok, attrs("type", typ, "value", v))
	}
}

func validateOptionsSkeleton(pattern, skeleton any) {
	s, isStr := pattern.(string)
	validate("E_INVALID_OPTIONS", "Invalid `{skeleton: \"{value}\"}` based on provided CLDR.", isUndefined(skeleton) || (isStr && s != ""),
		attrs("type", "skeleton", "value", skeleton))
}

func validateRequiredIana(timeZone any) func(string, any) {
	return func(path string, value any) {
		if !strings.Contains(path, "globalize-iana") {
			return
		}
		validate("E_MISSING_IANA_TZ", "Missing required IANA timezone content for `{timeZone}`: `{path}`.", jsTruthy(value),
			attrs("path", strings.Replace(path, "globalize-iana/", "", 1), "timeZone", timeZone))
	}
}

// LoadTimeZone is Globalize.loadTimeZone(json): it loads IANA time zone data (the iana-tz-data
// package's JSON).
func LoadTimeZone(json any) {
	json = fromPublic(json)
	custom := newObject()
	custom.set("globalize-iana", json)
	validateParameterPresence(json, "json")
	validateParameterTypePlainObject(json, "json")
	cldrLoad([]any{custom})
}

func (g *Globalize) dateToPartsFormatter(options any) func(any) []Part {
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
	var p *dateFormatProps
	func() {
		c.on(keyValidateRequiredCldr, validateRequiredCldr)
		var ianaKey any
		if jsTruthy(timeZone) {
			ianaKey = new(int)
			c.on(ianaKey, validateRequiredIana(timeZone))
		}
		defer func() {
			c.off(keyValidateRequiredCldr)
			if ianaKey != nil {
				c.off(ianaKey)
			}
		}()
		pattern := dateExpandPattern(o, c)
		validateOptionsSkeleton(pattern, o.prop("skeleton"))
		p = dateFormatProperties(pattern, c, timeZone)
	}()
	pads := make([]int, 0, len(p.numberFormatters))
	for pad := range p.numberFormatters {
		pads = append(pads, pad)
	}
	sort.Ints(pads)
	formatters := map[int]func(any) string{}
	for _, pad := range pads {
		formatters[pad] = g.numberFormatter(attrs("raw", p.numberFormatters[pad]))
	}
	return func(value any) []Part {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		d, ok := value.(Date)
		validateParameterType(value, "value", isUndefined(value) || ok, "Date")
		return dateFormat(d, formatters, p)
	}
}

func (g *Globalize) dateFormatter(options any) func(any) string {
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	if !optionsHasStyle(o) {
		o.set("skeleton", "yMd")
	}
	toParts := g.dateToPartsFormatter(optionsArg(options, o))
	return func(value any) string { return partsJoin(toParts(value)) }
}

// DateFormatter is .dateFormatter([options]).
func (g *Globalize) DateFormatter(options ...any) func(value any) string {
	return g.dateFormatter(opt(options))
}

// DateToPartsFormatter is .dateToPartsFormatter([options]).
func (g *Globalize) DateToPartsFormatter(options ...any) func(value any) []Part {
	return g.dateToPartsFormatter(opt(options))
}

// FormatDate is .formatDate(value[, options]).
func (g *Globalize) FormatDate(value any, options ...any) string {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	_, ok := value.(Date)
	validateParameterType(value, "value", isUndefined(value) || ok, "Date")
	return g.dateFormatter(opt(options))(value)
}

// FormatDateToParts is .formatDateToParts(value[, options]).
func (g *Globalize) FormatDateToParts(value any, options ...any) []Part {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	_, ok := value.(Date)
	validateParameterType(value, "value", isUndefined(value) || ok, "Date")
	return g.dateToPartsFormatter(opt(options))(value)
}

// DateFormatter is Globalize.dateFormatter([options]) on the default locale.
func DateFormatter(options ...any) func(value any) string { return statics.DateFormatter(options...) }

// DateToPartsFormatter is Globalize.dateToPartsFormatter([options]).
func DateToPartsFormatter(options ...any) func(value any) []Part {
	return statics.DateToPartsFormatter(options...)
}

// FormatDate is Globalize.formatDate(value[, options]).
func FormatDate(value any, options ...any) string { return statics.FormatDate(value, options...) }

// FormatDateToParts is Globalize.formatDateToParts(value[, options]).
func FormatDateToParts(value any, options ...any) []Part {
	return statics.FormatDateToParts(value, options...)
}
