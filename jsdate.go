package globalize

import (
	"fmt"
	"math"
)

// The ECMAScript Date algorithms, with the local time zone being UTC.

const msPerDay = 86400000.0

func posMod(a, b float64) float64 {
	r := math.Mod(a, b)
	if r < 0 {
		r += b
	}
	if r == 0 {
		return 0
	}
	return r
}

func dayOf(t float64) float64 { return math.Floor(t / msPerDay) }

func timeWithinDay(t float64) float64 { return posMod(t, msPerDay) }

func daysInYear(y float64) float64 {
	if math.Mod(y, 4) != 0 {
		return 365
	}
	if math.Mod(y, 100) != 0 {
		return 366
	}
	if math.Mod(y, 400) != 0 {
		return 365
	}
	return 366
}

func dayFromYear(y float64) float64 {
	return float64(365*(y-1970)) + math.Floor((y-1969)/4) - math.Floor((y-1901)/100) + math.Floor((y-1601)/400)
}

func yearFromTime(t float64) float64 {
	d := dayOf(t)
	y := math.Floor(d/365.2425) + 1970
	for dayFromYear(y) > d {
		y--
	}
	for dayFromYear(y+1) <= d {
		y++
	}
	return y
}

var cumulativeDays = [...]float64{0, 31, 59, 90, 120, 151, 181, 212, 243, 273, 304, 334, 365}

func monthDay(t float64) (month, date float64) {
	y := yearFromTime(t)
	dwy := dayOf(t) - dayFromYear(y)
	leap := 0.0
	if daysInYear(y) == 366 {
		leap = 1
	}
	for m := 0; m < 12; m++ {
		end := cumulativeDays[m+1]
		if m >= 1 {
			end += leap
		}
		if dwy < end {
			start := cumulativeDays[m]
			if m >= 2 {
				start += leap
			}
			return float64(m), dwy - start + 1
		}
	}
	return 11, dwy - cumulativeDays[11] - leap + 1
}

func weekDay(t float64) float64 { return posMod(dayOf(t)+4, 7) }

func hourFromTime(t float64) float64 { return posMod(math.Floor(t/3600000), 24) }
func minFromTime(t float64) float64  { return posMod(math.Floor(t/60000), 60) }
func secFromTime(t float64) float64  { return posMod(math.Floor(t/1000), 60) }
func msFromTime(t float64) float64   { return posMod(t, 1000) }

func toIntegerOrInfinity(x float64) float64 {
	if math.IsNaN(x) {
		return 0
	}
	if math.IsInf(x, 0) {
		return x
	}
	r := mathTrunc(x)
	if r == 0 {
		return 0
	}
	return r
}

func isFinite(x float64) bool { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func makeTime(h, m, s, ms float64) float64 {
	if !isFinite(h) || !isFinite(m) || !isFinite(s) || !isFinite(ms) {
		return math.NaN()
	}
	h, m, s, ms = toIntegerOrInfinity(h), toIntegerOrInfinity(m), toIntegerOrInfinity(s), toIntegerOrInfinity(ms)
	return float64(float64(float64(h*3600000)+float64(m*60000))+float64(s*1000)) + ms
}

func makeDay(year, month, date float64) float64 {
	if !isFinite(year) || !isFinite(month) || !isFinite(date) {
		return math.NaN()
	}
	y, m, dt := toIntegerOrInfinity(year), toIntegerOrInfinity(month), toIntegerOrInfinity(date)
	ym := y + math.Floor(m/12)
	if !isFinite(ym) || math.Abs(ym) > 1000000 || math.Abs(m) > 10000000 {
		return math.NaN()
	}
	mn := posMod(m, 12)
	days := dayFromYear(ym) + cumulativeDays[int(mn)]
	if mn >= 2 && daysInYear(ym) == 366 {
		days++
	}
	return days + dt - 1
}

func makeDate(day, t float64) float64 {
	if !isFinite(day) || !isFinite(t) {
		return math.NaN()
	}
	tv := float64(day*msPerDay) + t
	if !isFinite(tv) {
		return math.NaN()
	}
	return tv
}

func timeClip(t float64) float64 {
	if !isFinite(t) || math.Abs(t) > 8.64e15 {
		return math.NaN()
	}
	return toIntegerOrInfinity(t)
}

// newLocalDate is new Date(year, month[, date[, hours[, minutes[, seconds[, ms]]]]]).
func newLocalDate(args ...float64) float64 {
	get := func(i int, def float64) float64 {
		if i < len(args) {
			return args[i]
		}
		return def
	}
	y := get(0, math.NaN())
	if !math.IsNaN(y) {
		if yi := toIntegerOrInfinity(y); yi >= 0 && yi <= 99 {
			y = 1900 + yi
		}
	}
	return timeClip(makeDate(makeDay(y, get(1, 0), get(2, 1)), makeTime(get(3, 0), get(4, 0), get(5, 0), get(6, 0))))
}

// jsDate is a mutable JavaScript Date.
type jsDate struct{ t float64 }

func newJSDate(t float64) *jsDate { return &jsDate{t: timeClip(t)} }

func (d *jsDate) getTime() float64 { return d.t }

func (d *jsDate) field(f func(float64) float64) float64 {
	if math.IsNaN(d.t) {
		return math.NaN()
	}
	return f(d.t)
}

func (d *jsDate) getFullYear() float64 { return d.field(yearFromTime) }
func (d *jsDate) getMonth() float64 {
	return d.field(func(t float64) float64 { m, _ := monthDay(t); return m })
}
func (d *jsDate) getDate() float64 {
	return d.field(func(t float64) float64 { _, dd := monthDay(t); return dd })
}
func (d *jsDate) getDay() float64            { return d.field(weekDay) }
func (d *jsDate) getHours() float64          { return d.field(hourFromTime) }
func (d *jsDate) getMinutes() float64        { return d.field(minFromTime) }
func (d *jsDate) getSeconds() float64        { return d.field(secFromTime) }
func (d *jsDate) getMilliseconds() float64   { return d.field(msFromTime) }
func (d *jsDate) getTimezoneOffset() float64 { return d.field(func(float64) float64 { return 0 }) }

func (d *jsDate) setTime(t float64) float64 {
	d.t = timeClip(t)
	return d.t
}

func (d *jsDate) setTimeFields(h, m, s, ms *float64) float64 {
	t := d.t
	if math.IsNaN(t) {
		return math.NaN()
	}
	hh, mm, ss, mss := hourFromTime(t), minFromTime(t), secFromTime(t), msFromTime(t)
	if h != nil {
		hh = *h
	}
	if m != nil {
		mm = *m
	}
	if s != nil {
		ss = *s
	}
	if ms != nil {
		mss = *ms
	}
	d.t = timeClip(makeDate(dayOf(t), makeTime(hh, mm, ss, mss)))
	return d.t
}

func (d *jsDate) setHours(v float64) float64        { return d.setTimeFields(&v, nil, nil, nil) }
func (d *jsDate) setMinutes(v float64) float64      { return d.setTimeFields(nil, &v, nil, nil) }
func (d *jsDate) setSeconds(v float64) float64      { return d.setTimeFields(nil, nil, &v, nil) }
func (d *jsDate) setMilliseconds(v float64) float64 { return d.setTimeFields(nil, nil, nil, &v) }

func (d *jsDate) setDate(v float64) float64 {
	t := d.t
	if math.IsNaN(t) {
		return math.NaN()
	}
	m, _ := monthDay(t)
	d.t = timeClip(makeDate(makeDay(yearFromTime(t), m, v), timeWithinDay(t)))
	return d.t
}

func (d *jsDate) setMonth(v float64) float64 {
	t := d.t
	if math.IsNaN(t) {
		return math.NaN()
	}
	_, dd := monthDay(t)
	d.t = timeClip(makeDate(makeDay(yearFromTime(t), v, dd), timeWithinDay(t)))
	return d.t
}

func (d *jsDate) setFullYear(v float64) float64 {
	t := d.t
	if math.IsNaN(t) {
		t = 0
	}
	m, dd := monthDay(t)
	d.t = timeClip(makeDate(makeDay(v, m, dd), timeWithinDay(t)))
	return d.t
}

func pad(n float64, w int) string {
	s := fmt.Sprintf("%d", int64(n))
	for len(s) < w {
		s = "0" + s
	}
	return s
}

func dateToISOString(t float64) string {
	if math.IsNaN(t) {
		throwError("RangeError", "Invalid time value")
	}
	y := yearFromTime(t)
	m, d := monthDay(t)
	var ys string
	if y >= 0 && y <= 9999 {
		ys = pad(y, 4)
	} else if y < 0 {
		ys = "-" + pad(-y, 6)
	} else {
		ys = "+" + pad(y, 6)
	}
	return ys + "-" + pad(m+1, 2) + "-" + pad(d, 2) + "T" + pad(hourFromTime(t), 2) + ":" + pad(minFromTime(t), 2) + ":" +
		pad(secFromTime(t), 2) + "." + pad(msFromTime(t), 3) + "Z"
}

var weekdayNames = [...]string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
var monthNames = [...]string{"Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

func dateToString(t float64) string {
	if math.IsNaN(t) {
		return "Invalid Date"
	}
	y := yearFromTime(t)
	m, d := monthDay(t)
	ys := pad(math.Abs(y), 4)
	if y < 0 {
		ys = "-" + ys
	}
	return weekdayNames[int(weekDay(t))] + " " + monthNames[int(m)] + " " + pad(d, 2) + " " + ys + " " +
		pad(hourFromTime(t), 2) + ":" + pad(minFromTime(t), 2) + ":" + pad(secFromTime(t), 2) +
		" GMT+0000 (Coordinated Universal Time)"
}

// dateLike is what Globalize's date code calls on a Date or a ZonedDateTime.
type dateLike interface {
	getTime() float64
	getFullYear() float64
	getMonth() float64
	getDate() float64
	getDay() float64
	getHours() float64
	getMinutes() float64
	getSeconds() float64
	getMilliseconds() float64
	getTimezoneOffset() float64
	setFullYear(float64) float64
	setMonth(float64) float64
	setDate(float64) float64
	setHours(float64) float64
	setMinutes(float64) float64
	setSeconds(float64) float64
	setMilliseconds(float64) float64
}

type tzData struct {
	offsets, untils, isdsts any
}

// zonedDateTime is the ZonedDateTime Globalize inlines (zoned-date-time 1.1.0).
type zonedDateTime struct {
	original *jsDate
	local    *jsDate
	data     tzData
}

func newZonedDateTime(t float64, data tzData) *zonedDateTime {
	z := &zonedDateTime{original: newJSDate(t), local: newJSDate(t), data: data}
	if !jsTruthy(data.untils) || !jsTruthy(data.offsets) || !jsTruthy(data.isdsts) {
		throwError("Error", "Invalid IANA data")
	}
	z.local.setTime(z.local.getTime() - float64(float64(z.getTimezoneOffset()*60)*1e3))
	return z
}

func (z *zonedDateTime) untilsIndex() int {
	untils := z.data.untils
	n := int(toNumber(getProp(untils, "length")))
	t := z.original.getTime()
	i := 0
	for i < n-1 && t >= toNumber(getProp(untils, numToString(float64(i)))) {
		i++
	}
	return i
}

func (z *zonedDateTime) getTimezoneOffset() float64 {
	return toNumber(getProp(z.data.offsets, numToString(float64(z.untilsIndex()))))
}

func (z *zonedDateTime) isDST() bool {
	return jsTruthy(getProp(z.data.isdsts, numToString(float64(z.untilsIndex()))))
}

func (z *zonedDateTime) getTime() float64 {
	return z.local.getTime() + float64(float64(z.getTimezoneOffset()*60)*1e3)
}

func (z *zonedDateTime) clone() *zonedDateTime {
	return newZonedDateTime(z.original.getTime(), z.data)
}

func (z *zonedDateTime) toDate() float64 { return timeClip(z.getTime()) }

func (z *zonedDateTime) getFullYear() float64     { return z.local.getFullYear() }
func (z *zonedDateTime) getMonth() float64        { return z.local.getMonth() }
func (z *zonedDateTime) getDate() float64         { return z.local.getDate() }
func (z *zonedDateTime) getDay() float64          { return z.local.getDay() }
func (z *zonedDateTime) getHours() float64        { return z.local.getHours() }
func (z *zonedDateTime) getMinutes() float64      { return z.local.getMinutes() }
func (z *zonedDateTime) getSeconds() float64      { return z.local.getSeconds() }
func (z *zonedDateTime) getMilliseconds() float64 { return z.local.getMilliseconds() }

func (z *zonedDateTime) setWrap(fn func() float64) float64 {
	offset1 := z.getTimezoneOffset()
	ret := fn()
	z.original.setTime(timeClip(z.getTime()))
	offset2 := z.getTimezoneOffset()
	if offset2-offset1 != 0 && !math.IsNaN(offset2-offset1) {
		z.original.setMinutes(z.original.getMinutes() + offset2 - offset1)
	}
	return ret
}

func (z *zonedDateTime) setFullYear(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setFullYear(v) })
}
func (z *zonedDateTime) setMonth(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setMonth(v) })
}
func (z *zonedDateTime) setDate(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setDate(v) })
}
func (z *zonedDateTime) setHours(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setHours(v) })
}
func (z *zonedDateTime) setMinutes(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setMinutes(v) })
}
func (z *zonedDateTime) setSeconds(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setSeconds(v) })
}
func (z *zonedDateTime) setMilliseconds(v float64) float64 {
	return z.setWrap(func() float64 { return z.local.setMilliseconds(v) })
}
