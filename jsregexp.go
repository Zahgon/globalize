package globalize

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// jsRegexp is a JavaScript RegExp built from a source string Globalize assembles at run time.
type jsRegexp struct {
	re     *regexp.Regexp
	source string
}

const (
	jsSpaceClass = `\t\n\v\f\r \x{a0}\x{1680}\x{2000}-\x{200a}\x{2028}\x{2029}\x{202f}\x{205f}\x{3000}\x{feff}`
	jsDotClass   = `[^\n\r\x{2028}\x{2029}]`
)

// translateJSRegexp rewrites JavaScript regular-expression syntax into Go's where they differ.
func translateJSRegexp(src string) string {
	var b strings.Builder
	inClass := false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case c == '\\' && i+1 < len(src):
			n := src[i+1]
			switch {
			case n == 'u' && i+5 < len(src) && isHex(src[i+2:i+6]):
				b.WriteString(`\x{` + src[i+2:i+6] + `}`)
				i += 5
			case n == 's':
				if inClass {
					b.WriteString(jsSpaceClass)
				} else {
					b.WriteString("[" + jsSpaceClass + "]")
				}
				i++
			case n == 'S' && !inClass:
				b.WriteString("[^" + jsSpaceClass + "]")
				i++
			default:
				b.WriteByte(c)
				b.WriteByte(n)
				i++
			}
		case c == '[' && !inClass:
			inClass = true
			b.WriteByte(c)
			if i+1 < len(src) && src[i+1] == '^' {
				b.WriteByte('^')
				i++
			}
			if i+1 < len(src) && src[i+1] == ']' {
				b.WriteString(`\]`)
				i++
			}
		case c == ']' && inClass:
			inClass = false
			b.WriteByte(c)
		case c == '.' && !inClass:
			b.WriteString(jsDotClass)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

func isHex(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
			return false
		}
	}
	return true
}

func mustJSRegexp(src string) *jsRegexp {
	re, err := regexp.Compile(translateJSRegexp(src))
	if err != nil {
		throwError("SyntaxError", "Invalid regular expression: /"+src+"/: "+err.Error())
	}
	return &jsRegexp{re: re, source: src}
}

func (r *jsRegexp) find(s string) []int { return r.re.FindStringIndex(s) }

func (r *jsRegexp) test(s string) bool { return r.re.MatchString(s) }

// sourceLength is RegExp.prototype.source.length (UTF-16 code units).
func (r *jsRegexp) sourceLength() int { return jsLength(r.source) }

// quotedLiteralEnd returns where JavaScript's '([^']|”)+' matches at s[i] (exclusive end),
// or -1: the content is greedy and gives back iterations until a closing quote follows.
func quotedLiteralEnd(s string, i int) int {
	if i >= len(s) || s[i] != '\'' {
		return -1
	}
	bounds := []int{}
	j := i + 1
	for j < len(s) {
		if s[j] != '\'' {
			_, size := utf8.DecodeRuneInString(s[j:])
			j += size
		} else if j+1 < len(s) && s[j+1] == '\'' {
			j += 2
		} else {
			break
		}
		bounds = append(bounds, j)
	}
	for k := len(bounds) - 1; k >= 0; k-- {
		if b := bounds[k]; b < len(s) && s[b] == '\'' {
			return b + 1
		}
	}
	return -1
}

func decodeRune(s string) (rune, int) {
	r, size := utf8.DecodeRuneInString(s)
	if r == utf8.RuneError && size <= 1 {
		if _, n := decodeWTF8Surrogate(s); n > 0 {
			return 0xD800, n
		}
	}
	return r, size
}

// literalTokens visits s the way /('([^']|”)+'|”)|./g does: quoted literals whole, every
// other character on its own; line terminators, which "." does not match, are skipped.
func literalTokens(s string, fn func(tok string, literal bool)) {
	literalTokensIdx(s, func(start, end int, literal bool) { fn(s[start:end], literal) })
}

func literalTokensIdx(s string, fn func(start, end int, literal bool)) {
	for i := 0; i < len(s); {
		if end := quotedLiteralEnd(s, i); end > 0 {
			fn(i, end, true)
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "''") {
			fn(i, i+2, true)
			i += 2
			continue
		}
		r, size := decodeRune(s[i:])
		if !isJSLineTerminator(r) {
			fn(i, i+size, false)
		}
		i += size
	}
}

// compactAffixTokens visits s as /('(?:[^']|”)+'|”|\s+)|([^\s'0]+)/g does.
func compactAffixTokens(s string, fn func(tok string, literal bool)) {
	for i := 0; i < len(s); {
		if end := quotedLiteralEnd(s, i); end > 0 {
			fn(s[i:end], true)
			i = end
			continue
		}
		if strings.HasPrefix(s[i:], "''") {
			fn("''", true)
			i += 2
			continue
		}
		r, size := decodeRune(s[i:])
		if isJSWhiteSpace(r) {
			j := i + size
			for j < len(s) {
				r2, n := decodeRune(s[j:])
				if !isJSWhiteSpace(r2) {
					break
				}
				j += n
			}
			fn(s[i:j], true)
			i = j
			continue
		}
		if r == '\'' || r == '0' {
			i += size
			continue
		}
		j := i
		for j < len(s) {
			r2, n := decodeRune(s[j:])
			if isJSWhiteSpace(r2) || r2 == '\'' || r2 == '0' {
				break
			}
			j += n
		}
		fn(s[i:j], false)
		i = j
	}
}

// isJSLineTerminator reports whether r is one the JavaScript "." does not match.
func isJSLineTerminator(r rune) bool {
	return r == '\n' || r == '\r' || r == 0x2028 || r == 0x2029
}

// isJSWhiteSpace is the JavaScript \s.
func isJSWhiteSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xA0, 0x1680, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF:
		return true
	}
	return r >= 0x2000 && r <= 0x200A
}
