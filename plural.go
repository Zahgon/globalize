// The plural rule compiler in this file is a port of make-plural.js 3.0.0, which Globalize 1.7.1
// inlines in its plural module:
//
// make-plural.js -- https://github.com/eemeli/make-plural.js/
// Copyright (c) 2014-2015 by Eemeli Aro <eemeli@gmail.com>
//
// Permission to use, copy, modify, and/or distribute this software for any
// purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies.
//
// The software is provided "as is" and the author disclaims all warranties
// with regard to this software including all implied warranties of
// merchantability and fitness. In no event shall the author be liable for
// any special, direct, indirect, or consequential damages or any damages
// whatsoever resulting from loss of use, data or profits, whether in an
// action of contract, negligence or other tortious action, arising out of
// or in connection with the use or performance of this software.

package globalize

import (
	"regexp"
	"strconv"
	"strings"
)

// This file ports the plural module and the make-plural 3.0.0 compiler it inlines. make-plural
// rewrites each CLDR rule into a JavaScript expression and evaluates it with JavaScript
// semantics; the rewrite is reproduced as is and the expression evaluated by a small interpreter.

type pluralParser struct {
	flags []string
	set   map[string]bool
}

func (p *pluralParser) flag(name string) {
	if !p.set[name] {
		p.set[name] = true
		p.flags = append(p.flags, name)
	}
}

func replaceSubmatches(re *regexp.Regexp, s string, fn func(m []string) string) string {
	return re.ReplaceAllStringFunc(s, func(match string) string {
		return fn(re.FindStringSubmatch(match))
	})
}

var (
	plRe1 = regexp.MustCompile(`([tv]) (!?)= 0`)
	plRe2 = regexp.MustCompile(`\b[fintv]\b`)
	plRe3 = regexp.MustCompile(`([fin]) % (10+)`)
	plRe4 = regexp.MustCompile(`n10+ = 0`)
	plRe5 = regexp.MustCompile(`(\w+ (!?)= )([0-9.]+,[0-9.,]+)`)
	plRe6 = regexp.MustCompile(`(\w+) (!?)= ([0-9]+)\.\.([0-9]+)`)
)

func (p *pluralParser) parse(cond string) string {
	switch cond {
	case "i = 0 or n = 1":
		return "n >= 0 && n <= 1"
	case "i = 0,1":
		return "n >= 0 && n < 2"
	case "i = 1 and v = 0":
		p.flag("v0")
		return "n == 1 && v0"
	}
	s := replaceSubmatches(plRe1, cond, func(m []string) string {
		sn := m[1] + "0"
		p.flag(sn)
		if m[2] != "" {
			return "!" + sn
		}
		return sn
	})
	s = plRe2.ReplaceAllStringFunc(s, func(m string) string {
		p.flag(m)
		return m
	})
	s = replaceSubmatches(plRe3, s, func(m []string) string {
		sn := m[1] + m[2]
		p.flag(sn)
		return sn
	})
	s = plRe4.ReplaceAllStringFunc(s, func(m string) string { return "t0 && " + m })
	s = replaceSubmatches(plRe5, s, func(m []string) string {
		if m[0] == "n = 0,1" {
			return "(n == 0 || n == 1)"
		}
		items := strings.Split(m[3], ",")
		if m[2] != "" {
			return m[1] + strings.Join(items, " && "+m[1])
		}
		return "(" + m[1] + strings.Join(items, " || "+m[1]) + ")"
	})
	s = replaceSubmatches(plRe6, s, func(m []string) string {
		sym, noteq, x0, x1 := m[1], m[2], m[3], m[4]
		if stringToNumber(x0)+1 == stringToNumber(x1) {
			if noteq != "" {
				return sym + " != " + x0 + " && " + sym + " != " + x1
			}
			return "(" + sym + " == " + x0 + " || " + sym + " == " + x1 + ")"
		}
		if noteq != "" {
			return "(" + sym + " < " + x0 + " || " + sym + " > " + x1 + ")"
		}
		if sym == "n" {
			p.flag("t0")
			return "(t0 && n >= " + x0 + " && n <= " + x1 + ")"
		}
		return "(" + sym + " >= " + x0 + " && " + sym + " <= " + x1 + ")"
	})
	s = strings.ReplaceAll(s, " and ", " && ")
	s = strings.ReplaceAll(s, " or ", " || ")
	return strings.ReplaceAll(s, " = ", " == ")
}

var modVarRe = regexp.MustCompile(`^.10+$`)

// pluralVar is one declarator of the generated `var s = String(n).split('.'), ...`.
type pluralVar struct {
	name string
	eval func(env map[string]any, s []string) any
}

func (p *pluralParser) vars() []pluralVar {
	var vars []pluralVar
	s1 := func(s []string) any {
		if len(s) > 1 {
			return s[1]
		}
		return Undefined
	}
	if p.set["i"] {
		vars = append(vars, pluralVar{"i", func(_ map[string]any, s []string) any { return s[0] }})
	}
	if p.set["f"] || p.set["v"] {
		vars = append(vars, pluralVar{"f", func(_ map[string]any, s []string) any {
			if v := s1(s); jsTruthy(v) {
				return v
			}
			return ""
		}})
	}
	if p.set["t"] {
		vars = append(vars, pluralVar{"t", func(_ map[string]any, s []string) any {
			f := ""
			if v := s1(s); jsTruthy(v) {
				f = v.(string)
			}
			return strings.TrimRight(f, "0")
		}})
	}
	if p.set["v"] {
		vars = append(vars, pluralVar{"v", func(env map[string]any, _ []string) any {
			return float64(jsLength(jsString(pluralLookup(env, "f"))))
		}})
	}
	if p.set["v0"] {
		vars = append(vars, pluralVar{"v0", func(_ map[string]any, s []string) any { return !jsTruthy(s1(s)) }})
	}
	if p.set["t0"] || p.set["n10"] || p.set["n100"] {
		vars = append(vars, pluralVar{"t0", func(env map[string]any, s []string) any {
			return looseEquals(stringToNumber(s[0]), pluralLookup(env, "n"))
		}})
	}
	for _, k := range p.flags {
		if !modVarRe.MatchString(k) {
			continue
		}
		k := k
		digits := len(k) - 2
		vars = append(vars, pluralVar{k, func(env map[string]any, s []string) any {
			var base any
			if k[0] == 'n' {
				t0 := pluralLookup(env, "t0")
				if !jsTruthy(t0) {
					return t0
				}
				base = s[0]
			} else {
				base = pluralLookup(env, k[:1])
			}
			str, ok := base.(string)
			if !ok {
				throwTypeError(k[:1] + ".slice is not a function")
			}
			if len(str) > digits {
				return str[len(str)-digits:]
			}
			return str
		}})
	}
	return vars
}

func pluralLookup(env map[string]any, name string) any {
	v, ok := env[name]
	if !ok {
		throwError("ReferenceError", name+" is not defined")
	}
	return v
}

// looseEquals is the JavaScript == on the primitive values the generated code compares.
func looseEquals(a, b any) bool {
	switch x := a.(type) {
	case float64:
		switch y := b.(type) {
		case float64:
			return x == y
		case string:
			return x == stringToNumber(y)
		case bool:
			return x == toNumber(y)
		}
	case string:
		switch y := b.(type) {
		case string:
			return x == y
		case float64, bool:
			return stringToNumber(x) == toNumber(y)
		}
	case bool:
		switch y := b.(type) {
		case bool:
			return x == y
		case float64, string:
			return toNumber(x) == toNumber(y)
		}
	}
	return false
}

// plExpr is a node of a generated plural condition.
type plExpr struct {
	op          string
	left, right *plExpr
	name        string
	num         float64
}

type plTokens struct {
	toks []string
	pos  int
}

func (t *plTokens) peek() string {
	if t.pos < len(t.toks) {
		return t.toks[t.pos]
	}
	return ""
}

func (t *plTokens) next() string {
	tok := t.peek()
	t.pos++
	return tok
}

var plTokenRe = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*|[0-9]+(\.[0-9]+)?|==|!=|<=|>=|&&|\|\||[<>!()]`)

func parsePluralExpr(src string) *plExpr {
	t := &plTokens{toks: plTokenRe.FindAllString(src, -1)}
	e := t.or()
	if t.pos != len(t.toks) {
		throwError("SyntaxError", "Unexpected token")
	}
	return e
}

func (t *plTokens) or() *plExpr {
	e := t.and()
	for t.peek() == "||" {
		t.next()
		e = &plExpr{op: "||", left: e, right: t.and()}
	}
	return e
}

func (t *plTokens) and() *plExpr {
	e := t.eq()
	for t.peek() == "&&" {
		t.next()
		e = &plExpr{op: "&&", left: e, right: t.eq()}
	}
	return e
}

func (t *plTokens) eq() *plExpr {
	e := t.rel()
	for t.peek() == "==" || t.peek() == "!=" {
		op := t.next()
		e = &plExpr{op: op, left: e, right: t.rel()}
	}
	return e
}

func (t *plTokens) rel() *plExpr {
	e := t.unary()
	for {
		switch t.peek() {
		case "<", ">", "<=", ">=":
			op := t.next()
			e = &plExpr{op: op, left: e, right: t.unary()}
			continue
		}
		return e
	}
}

func (t *plTokens) unary() *plExpr {
	if t.peek() == "!" {
		t.next()
		return &plExpr{op: "!", left: t.unary()}
	}
	tok := t.next()
	switch {
	case tok == "(":
		e := t.or()
		if t.next() != ")" {
			throwError("SyntaxError", "Unexpected token")
		}
		return e
	case tok != "" && tok[0] >= '0' && tok[0] <= '9':
		f, _ := strconv.ParseFloat(tok, 64)
		return &plExpr{op: "num", num: f}
	case tok != "":
		return &plExpr{op: "var", name: tok}
	}
	throwError("SyntaxError", "Unexpected end of input")
	return nil
}

func (e *plExpr) eval(env map[string]any) any {
	switch e.op {
	case "num":
		return e.num
	case "var":
		return pluralLookup(env, e.name)
	case "!":
		return !jsTruthy(e.left.eval(env))
	case "&&":
		l := e.left.eval(env)
		if !jsTruthy(l) {
			return l
		}
		return e.right.eval(env)
	case "||":
		l := e.left.eval(env)
		if jsTruthy(l) {
			return l
		}
		return e.right.eval(env)
	case "==":
		return looseEquals(e.left.eval(env), e.right.eval(env))
	case "!=":
		return !looseEquals(e.left.eval(env), e.right.eval(env))
	}
	return jsCompare(e.left.eval(env), e.right.eval(env), e.op)
}

type pluralCase struct {
	cond *plExpr
	cat  string
}

var ruleSplitRe = regexp.MustCompile(`\s*@\w*`)

// makePlural is new MakePlural(lc, {cardinals, ordinals}) for one plural type.
func makePlural(lc any, typ string, rules any) func(float64) string {
	p := &pluralParser{set: map[string]bool{}}
	var cases []pluralCase
	langRules := getProp(rules, jsString(lc))
	if !jsTruthy(langRules) {
		throwError("Error", `Locale "`+jsString(lc)+`" `+typ+" rules not found")
	}
	forIn(langRules, func(r string, v any) {
		cond := ruleSplitRe.Split(strings.TrimFunc(jsString(v), isStrWhiteSpace), -1)[0]
		cat := strings.Replace(r, "pluralRule-count-", "", 1)
		if cond != "" {
			cases = append(cases, pluralCase{cond: parsePluralExpr(p.parse(cond)), cat: cat})
		}
	})
	vars := p.vars()
	return func(n float64) string {
		s := strings.Split(numToString(n), ".")
		env := map[string]any{"n": n}
		for _, v := range vars {
			env[v.name] = v.eval(env, s)
		}
		for _, c := range cases {
			if jsTruthy(c.cond.eval(env)) {
				return c.cat
			}
		}
		return "other"
	}
}

func (g *Globalize) pluralGenerator(options any) func(any) string {
	validateParameterTypePlainObject(options, "options")
	o := optionsObject(options)
	c := g.cldr
	typ := o.prop("type")
	if !jsTruthy(typ) {
		typ = "cardinal"
	}
	t := o.prop("type")
	validateParameterType(t, "options.type", isUndefined(t) || t == "cardinal" || t == "ordinal", `String "cardinal" or "ordinal"`)
	validateDefaultLocale(c)
	func() {
		c.on(keyValidateCldr, validateCldrListener)
		defer c.off(keyValidateCldr)
		c.supplemental([]any{"plurals-type-" + jsString(typ), "{language}"})
	}()
	rules := c.supplemental("plurals-type-" + jsString(typ))
	plural := makePlural(c.attributes.prop("language"), jsString(typ), rules)
	return func(value any) string {
		value = fromPublic(value)
		validateParameterPresence(value, "value")
		validateParameterTypeNumber(value, "value")
		return plural(value.(float64))
	}
}

// PluralGenerator is .pluralGenerator([options]).
func (g *Globalize) PluralGenerator(options ...any) func(value any) string {
	return g.pluralGenerator(opt(options))
}

// Plural is .plural(value[, options]).
func (g *Globalize) Plural(value any, options ...any) string {
	value = fromPublic(value)
	validateParameterPresence(value, "value")
	validateParameterTypeNumber(value, "value")
	return g.pluralGenerator(opt(options))(value)
}

// PluralGenerator is Globalize.pluralGenerator([options]) on the default locale.
func PluralGenerator(options ...any) func(value any) string {
	return statics.PluralGenerator(options...)
}

// Plural is Globalize.plural(value[, options]).
func Plural(value any, options ...any) string { return statics.Plural(value, options...) }
