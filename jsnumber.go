package globalize

import (
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"
)

func nowMillis() int64 { return time.Now().UnixMilli() }

// numToString is the ECMAScript Number::toString(x) in radix 10.
func numToString(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case x == 0:
		return "0"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	digits, n := shortestDigits(x)
	k := len(digits)
	switch {
	case k <= n && n <= 21:
		return sign + digits + strings.Repeat("0", n-k)
	case 0 < n && n <= 21:
		return sign + digits[:n] + "." + digits[n:]
	case -6 < n && n <= 0:
		return sign + "0." + strings.Repeat("0", -n) + digits
	}
	e := n - 1
	es := "+"
	if e < 0 {
		es = "-"
		e = -e
	}
	if k == 1 {
		return sign + digits + "e" + es + strconv.Itoa(e)
	}
	return sign + digits[:1] + "." + digits[1:] + "e" + es + strconv.Itoa(e)
}

// shortestDigits returns the shortest round-tripping decimal digits of x > 0 and the position n
// of the decimal point (x = 0.digits * 10^n).
func shortestDigits(x float64) (string, int) {
	s := strconv.FormatFloat(x, 'e', -1, 64)
	mant, exp, _ := strings.Cut(s, "e")
	digits := strings.Replace(mant, ".", "", 1)
	e, _ := strconv.Atoi(exp)
	digits = strings.TrimRight(digits, "0")
	if digits == "" {
		digits = "0"
	}
	return digits, e + 1
}

// toExponential is Number.prototype.toExponential() with fractionDigits undefined.
func toExponential(x float64) string {
	switch {
	case math.IsNaN(x):
		return "NaN"
	case math.IsInf(x, 1):
		return "Infinity"
	case math.IsInf(x, -1):
		return "-Infinity"
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	if x == 0 {
		return sign + "0e+0"
	}
	digits, n := shortestDigits(x)
	e := n - 1
	es := "+"
	if e < 0 {
		es = "-"
		e = -e
	}
	m := digits[:1]
	if len(digits) > 1 {
		m += "." + digits[1:]
	}
	return sign + m + "e" + es + strconv.Itoa(e)
}

// toFixed is Number.prototype.toFixed(f).
func toFixed(x float64, f int) string {
	if math.IsNaN(x) {
		return "NaN"
	}
	if math.Abs(x) >= 1e21 || math.IsInf(x, 0) {
		return numToString(x)
	}
	sign := ""
	if x < 0 {
		sign = "-"
		x = -x
	}
	var n *big.Int
	if x == 0 {
		n = new(big.Int)
	} else {
		mant, exp := math.Frexp(x)
		m := new(big.Int).SetInt64(int64(mant * (1 << 53)))
		exp -= 53
		num := new(big.Int).Mul(m, new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(f)), nil))
		if exp >= 0 {
			n = num.Lsh(num, uint(exp))
		} else {
			den := new(big.Int).Lsh(big.NewInt(1), uint(-exp))
			num.Lsh(num, 1)
			num.Add(num, den)
			den.Lsh(den, 1)
			n = num.Div(num, den)
		}
	}
	s := n.String()
	if f == 0 {
		return sign + s
	}
	if len(s) <= f {
		s = strings.Repeat("0", f+1-len(s)) + s
	}
	return sign + s[:len(s)-f] + "." + s[len(s)-f:]
}

func isStrWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\v', '\f', ' ', 0xA0, 0xFEFF, '\n', '\r', 0x2028, 0x2029, 0x1680, 0x202F, 0x205F, 0x3000:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}

// stringToNumber is the ECMAScript ToNumber applied to a string.
func stringToNumber(s string) float64 {
	s = strings.TrimFunc(s, isStrWhiteSpace)
	if s == "" {
		return 0
	}
	switch s {
	case "Infinity", "+Infinity":
		return math.Inf(1)
	case "-Infinity":
		return math.Inf(-1)
	}
	if len(s) > 2 && s[0] == '0' {
		base := 0
		switch s[1] {
		case 'x', 'X':
			base = 16
		case 'o', 'O':
			base = 8
		case 'b', 'B':
			base = 2
		}
		if base != 0 {
			v := new(big.Int)
			if _, ok := v.SetString(s[2:], base); !ok || strings.ContainsAny(s[2:], "+-_") {
				return math.NaN()
			}
			f, _ := new(big.Float).SetInt(v).Float64()
			return f
		}
	}
	i := 0
	if s[0] == '+' || s[0] == '-' {
		i++
	}
	digits := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
		digits++
	}
	if i < len(s) && s[i] == '.' {
		i++
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			digits++
		}
	}
	if digits == 0 {
		return math.NaN()
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		ed := 0
		for i < len(s) && s[i] >= '0' && s[i] <= '9' {
			i++
			ed++
		}
		if ed == 0 {
			return math.NaN()
		}
	}
	if i != len(s) {
		return math.NaN()
	}
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		if ne, ok := err.(*strconv.NumError); ok && ne.Err == strconv.ErrRange {
			return f
		}
		return math.NaN()
	}
	return f
}

// toNumber is the ECMAScript ToNumber (unary +).
func toNumber(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case undefinedType:
		return math.NaN()
	case nil:
		return 0
	case bool:
		if x {
			return 1
		}
		return 0
	case string:
		return stringToNumber(x)
	case Date:
		return float64(x)
	case []any:
		return stringToNumber(jsString(x))
	}
	return math.NaN()
}

// mathRound is Math.round.
func mathRound(x float64) float64 {
	if math.IsNaN(x) || math.IsInf(x, 0) || x == 0 {
		return x
	}
	if x > 0 && x < 0.5 {
		return 0
	}
	if x < 0 && x >= -0.5 {
		return math.Copysign(0, -1)
	}
	r := math.Floor(x)
	if x-r >= 0.5 {
		r++
	}
	return r
}

func mathTrunc(x float64) float64 {
	if math.IsNaN(x) {
		return math.NaN()
	}
	if x < 0 {
		return math.Ceil(x)
	}
	return math.Floor(x)
}

var exactPow10 = [...]float64{1e0, 1e1, 1e2, 1e3, 1e4, 1e5, 1e6, 1e7, 1e8, 1e9, 1e10, 1e11, 1e12,
	1e13, 1e14, 1e15, 1e16, 1e17, 1e18, 1e19, 1e20, 1e21, 1e22}

// pow10 is Math.pow(10, n) for an integer n.
func pow10(n int) float64 {
	if n >= 0 && n < len(exactPow10) {
		return exactPow10[n]
	}
	if n < 0 && -n < len(exactPow10) {
		return 1 / exactPow10[-n]
	}
	f, _ := strconv.ParseFloat("1e"+strconv.Itoa(n), 64)
	return f
}

// jsLog is Math.log, the fdlibm algorithm V8 uses.
func jsLog(x float64) float64 {
	const (
		ln2Hi = 6.93147180369123816490e-01
		ln2Lo = 1.90821492927058770002e-10
		two54 = 1.80143985094819840000e+16
		lg1   = 6.666666666666735130e-01
		lg2   = 3.999999999940941908e-01
		lg3   = 2.857142874366239149e-01
		lg4   = 2.222219843214978396e-01
		lg5   = 1.818357216161805012e-01
		lg6   = 1.531383769920937332e-01
		lg7   = 1.479819860511658591e-01
	)
	bits := math.Float64bits(x)
	hx := int32(bits >> 32)
	lx := uint32(bits)
	k := int32(0)
	if hx < 0x00100000 {
		if (hx&0x7fffffff)|int32(lx) == 0 {
			return math.Inf(-1)
		}
		if hx < 0 {
			return math.NaN()
		}
		k -= 54
		x *= two54
		bits = math.Float64bits(x)
		hx = int32(bits >> 32)
	}
	if hx >= 0x7ff00000 {
		return x + x
	}
	k += (hx >> 20) - 1023
	hx &= 0x000fffff
	i := (hx + 0x95f64) & 0x100000
	x = math.Float64frombits(uint64(uint32(hx|(i^0x3ff00000)))<<32 | (math.Float64bits(x) & 0xffffffff))
	k += i >> 20
	// Every product is rounded on its own (float64(...)) so no platform fuses it into an FMA.
	f := x - 1.0
	dk := float64(k)
	if (0x000fffff & (2 + hx)) < 3 {
		if f == 0 {
			if k == 0 {
				return 0
			}
			return float64(dk*ln2Hi) + float64(dk*ln2Lo)
		}
		R := float64(float64(f*f) * (0.5 - float64(0.33333333333333333*f)))
		if k == 0 {
			return f - R
		}
		return float64(dk*ln2Hi) - ((R - float64(dk*ln2Lo)) - f)
	}
	s := f / (2.0 + f)
	z := float64(s * s)
	i = hx - 0x6147a
	w := float64(z * z)
	j := 0x6b851 - hx
	t1 := float64(w * (lg2 + float64(w*(lg4+float64(w*lg6)))))
	t2 := float64(z * (lg1 + float64(w*(lg3+float64(w*(lg5+float64(w*lg7)))))))
	i |= j
	R := t2 + t1
	if i > 0 {
		hfsq := float64(float64(0.5*f) * f)
		if k == 0 {
			return f - (hfsq - float64(s*(hfsq+R)))
		}
		return float64(dk*ln2Hi) - ((hfsq - (float64(s*(hfsq+R)) + float64(dk*ln2Lo))) - f)
	}
	if k == 0 {
		return f - float64(s*(f-R))
	}
	return float64(dk*ln2Hi) - ((float64(s*(f-R)) - float64(dk*ln2Lo)) - f)
}

// parseIntJS is parseInt(s, 10).
func parseIntJS(v any) float64 {
	s := strings.TrimLeftFunc(jsString(v), isStrWhiteSpace)
	sign := 1.0
	if s != "" && (s[0] == '+' || s[0] == '-') {
		if s[0] == '-' {
			sign = -1
		}
		s = s[1:]
	}
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	if i == 0 {
		return math.NaN()
	}
	f, _ := strconv.ParseFloat(s[:i], 64)
	return sign * f
}
