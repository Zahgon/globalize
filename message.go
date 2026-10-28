// The message parser and compiler in this file are a port of messageformat.js 0.3.0-1 and the
// PEG.js 0.8.0 parser generated for it, which Globalize 1.7.1 inlines in its message module:
//
// messageformat.js - ICU PluralFormat + SelectFormat for JavaScript
// @author Alex Sexton - @SlexAxton
// @version 0.3.0-1
// @copyright 2012-2015 Alex Sexton, Eemeli Aro, and Contributors
// @license To use or fork, MIT. To contribute back, Dojo CLA

package globalize

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// The message module embeds messageformat 0.3.0 and the PEG.js 0.8.0 parser it generated.

type pegExpect struct{ typ, description string }

var (
	pegLBrace        = &pegExpect{"literal", `"{"`}
	pegComma         = &pegExpect{"literal", `","`}
	pegRBrace        = &pegExpect{"literal", `"}"`}
	pegPlural        = &pegExpect{"literal", `"plural"`}
	pegSelectOrdinal = &pegExpect{"literal", `"selectordinal"`}
	pegSelect        = &pegExpect{"literal", `"select"`}
	pegOffset        = &pegExpect{"literal", `"offset"`}
	pegColon         = &pegExpect{"literal", `":"`}
	pegEquals        = &pegExpect{"literal", `"="`}
	pegHash          = &pegExpect{"literal", `"#"`}
	pegIdentifier    = &pegExpect{"other", "identifier"}
	pegIDStart       = &pegExpect{"class", "[0-9a-zA-Z$_]"}
	pegIDRest        = &pegExpect{"class", `[^ \t\n\r,.+={}]`}
	pegCharClass     = &pegExpect{"class", `[^{}#\\\0-\x1F` + "\x7f" + ` \t\n\r]`}
	pegEscBackslash  = &pegExpect{"literal", `"\\\\"`}
	pegEscHash       = &pegExpect{"literal", `"\\#"`}
	pegEscLBrace     = &pegExpect{"literal", `"\\{"`}
	pegEscRBrace     = &pegExpect{"literal", `"\\}"`}
	pegEscU          = &pegExpect{"literal", `"\\u"`}
	pegDigit         = &pegExpect{"class", "[0-9]"}
	pegHexDigit      = &pegExpect{"class", "[0-9a-fA-F]"}
	pegWhitespace    = &pegExpect{"class", `[ \t\n\r]`}
)

type mfNode struct {
	typ           string
	statements    []*mfNode
	argumentIndex string
	elementFormat *mfNode
	output        bool
	key           string
	pattern       *mfNode
	styles        []string
	pluralForms   []mfForm
	offset        float64
	str           string
}

type mfForm struct {
	key any
	val *mfNode
}

type pegParser struct {
	in              []uint16
	pos             int
	maxFailPos      int
	maxFailExpected []*pegExpect
	silentFails     int
}

func (p *pegParser) fail(e *pegExpect) {
	if p.silentFails != 0 || p.pos < p.maxFailPos {
		return
	}
	if p.pos > p.maxFailPos {
		p.maxFailPos = p.pos
		p.maxFailExpected = nil
	}
	p.maxFailExpected = append(p.maxFailExpected, e)
}

func (p *pegParser) lit(s string, e *pegExpect) bool {
	n := len(s)
	if p.pos+n <= len(p.in) {
		ok := true
		for i := 0; i < n; i++ {
			if p.in[p.pos+i] != uint16(s[i]) {
				ok = false
				break
			}
		}
		if ok {
			p.pos += n
			return true
		}
	}
	p.fail(e)
	return false
}

func (p *pegParser) class(match func(uint16) bool, e *pegExpect) (uint16, bool) {
	if p.pos < len(p.in) && match(p.in[p.pos]) {
		p.pos++
		return p.in[p.pos-1], true
	}
	p.fail(e)
	return 0, false
}

func isIDStart(u uint16) bool {
	return u >= '0' && u <= '9' || u >= 'a' && u <= 'z' || u >= 'A' && u <= 'Z' || u == '$' || u == '_'
}

func isIDRest(u uint16) bool {
	switch u {
	case ' ', '\t', '\n', '\r', ',', '.', '+', '=', '{', '}':
		return false
	}
	return true
}

func isMessageChar(u uint16) bool {
	switch u {
	case '{', '}', '#', '\\', ' ', 0x7F:
		return false
	}
	return u > 0x1F
}

func isDigitUnit(u uint16) bool { return u >= '0' && u <= '9' }

func isHexUnit(u uint16) bool {
	return isDigitUnit(u) || u >= 'a' && u <= 'f' || u >= 'A' && u <= 'F'
}

func isWhitespaceUnit(u uint16) bool { return u == ' ' || u == '\t' || u == '\n' || u == '\r' }

func (p *pegParser) ws() {
	p.silentFails++
	for {
		if _, ok := p.class(isWhitespaceUnit, pegWhitespace); !ok {
			break
		}
	}
	p.silentFails--
}

func (p *pegParser) messageFormatPattern() *mfNode {
	n := &mfNode{typ: "messageFormatPattern"}
	for {
		s := p.messageFormatElement()
		if s == nil {
			s = p.string()
		}
		if s == nil {
			s = p.octothorpe()
		}
		if s == nil {
			break
		}
		n.statements = append(n.statements, s)
	}
	return n
}

func (p *pegParser) messageFormatElement() *mfNode {
	s0 := p.pos
	if !p.lit("{", pegLBrace) {
		return nil
	}
	p.ws()
	id, ok := p.id()
	if !ok {
		p.pos = s0
		return nil
	}
	s4 := p.pos
	var efmt *mfNode
	if p.lit(",", pegComma) {
		efmt = p.elementFormat()
		if efmt == nil {
			p.pos = s4
		}
	} else {
		p.pos = s4
	}
	p.ws()
	if !p.lit("}", pegRBrace) {
		p.pos = s0
		return nil
	}
	n := &mfNode{typ: "messageFormatElement", argumentIndex: id}
	if efmt != nil {
		n.elementFormat = efmt
	} else {
		n.output = true
	}
	return n
}

func (p *pegParser) elementFormat() *mfNode {
	keyed := func(word string, e *pegExpect, body func() *mfNode) *mfNode {
		s0 := p.pos
		p.ws()
		if !p.lit(word, e) {
			p.pos = s0
			return nil
		}
		p.ws()
		if !p.lit(",", pegComma) {
			p.pos = s0
			return nil
		}
		p.ws()
		pat := body()
		if pat == nil {
			p.pos = s0
			return nil
		}
		p.ws()
		return &mfNode{typ: "elementFormat", key: word, pattern: pat}
	}
	if n := keyed("plural", pegPlural, p.pluralFormatPattern); n != nil {
		return n
	}
	if n := keyed("selectordinal", pegSelectOrdinal, p.pluralFormatPattern); n != nil {
		return n
	}
	if n := keyed("select", pegSelect, p.selectFormatPattern); n != nil {
		return n
	}
	s0 := p.pos
	p.ws()
	id, ok := p.id()
	if !ok {
		p.pos = s0
		return nil
	}
	n := &mfNode{typ: "elementFormat", key: id, styles: []string{}}
	for {
		s, ok := p.argStylePattern()
		if !ok {
			break
		}
		n.styles = append(n.styles, s)
	}
	return n
}

func (p *pegParser) pluralFormatPattern() *mfNode {
	s0 := p.pos
	op, hasOffset := p.offsetPattern()
	var forms []mfForm
	for {
		f, ok := p.pluralForm()
		if !ok {
			break
		}
		forms = append(forms, f)
	}
	if len(forms) == 0 {
		p.pos = s0
		return nil
	}
	n := &mfNode{typ: "pluralFormatPattern", pluralForms: forms}
	if hasOffset {
		n.offset = op
	}
	return n
}

func (p *pegParser) offsetPattern() (float64, bool) {
	s0 := p.pos
	p.ws()
	if !p.lit("offset", pegOffset) {
		p.pos = s0
		return 0, false
	}
	p.ws()
	if !p.lit(":", pegColon) {
		p.pos = s0
		return 0, false
	}
	p.ws()
	d, ok := p.digits()
	if !ok {
		p.pos = s0
		return 0, false
	}
	p.ws()
	return d, true
}

func (p *pegParser) form(key func() (any, bool)) (mfForm, bool) {
	s0 := p.pos
	p.ws()
	k, ok := key()
	if !ok {
		p.pos = s0
		return mfForm{}, false
	}
	p.ws()
	if !p.lit("{", pegLBrace) {
		p.pos = s0
		return mfForm{}, false
	}
	p.ws()
	val := p.messageFormatPattern()
	p.ws()
	if !p.lit("}", pegRBrace) {
		p.pos = s0
		return mfForm{}, false
	}
	return mfForm{key: k, val: val}, true
}

func (p *pegParser) pluralForm() (mfForm, bool) { return p.form(p.pluralKey) }

func (p *pegParser) pluralKey() (any, bool) {
	if id, ok := p.id(); ok {
		return id, true
	}
	s0 := p.pos
	if !p.lit("=", pegEquals) {
		return nil, false
	}
	d, ok := p.digits()
	if !ok {
		p.pos = s0
		return nil, false
	}
	return d, true
}

func (p *pegParser) selectFormatPattern() *mfNode {
	var forms []mfForm
	for {
		f, ok := p.form(func() (any, bool) {
			id, ok := p.id()
			return id, ok
		})
		if !ok {
			break
		}
		forms = append(forms, f)
	}
	if len(forms) == 0 {
		return nil
	}
	return &mfNode{typ: "selectFormatPattern", pluralForms: forms}
}

func (p *pegParser) argStylePattern() (string, bool) {
	s0 := p.pos
	p.ws()
	if !p.lit(",", pegComma) {
		p.pos = s0
		return "", false
	}
	p.ws()
	id, ok := p.id()
	if !ok {
		p.pos = s0
		return "", false
	}
	p.ws()
	return id, true
}

func (p *pegParser) octothorpe() *mfNode {
	if !p.lit("#", pegHash) {
		return nil
	}
	return &mfNode{typ: "octothorpe"}
}

func (p *pegParser) string() *mfNode {
	var b []uint16
	matched := false
	for {
		if s, ok := p.chars(); ok {
			b = append(b, s...)
		} else if u, ok := p.class(isWhitespaceUnit, pegWhitespace); ok {
			b = append(b, u)
		} else {
			break
		}
		matched = true
	}
	if !matched {
		return nil
	}
	return &mfNode{typ: "string", str: fromUnits(b)}
}

func (p *pegParser) id() (string, bool) {
	p.silentFails++
	s0 := p.pos
	p.ws()
	s2 := p.pos
	ok := false
	if _, ok = p.class(isIDStart, pegIDStart); ok {
		for {
			if _, more := p.class(isIDRest, pegIDRest); !more {
				break
			}
		}
	} else {
		p.pos = s2
	}
	var text string
	if ok {
		text = fromUnits(p.in[s2:p.pos])
		p.ws()
	} else {
		p.pos = s0
	}
	p.silentFails--
	if !ok {
		p.fail(pegIdentifier)
	}
	return text, ok
}

func (p *pegParser) chars() ([]uint16, bool) {
	var out []uint16
	for {
		u, ok := p.char()
		if !ok {
			break
		}
		out = append(out, u)
	}
	return out, len(out) > 0
}

func (p *pegParser) char() (uint16, bool) {
	if u, ok := p.class(isMessageChar, pegCharClass); ok {
		return u, true
	}
	if p.lit(`\\`, pegEscBackslash) {
		return '\\', true
	}
	if p.lit(`\#`, pegEscHash) {
		return '#', true
	}
	if p.lit(`\{`, pegEscLBrace) {
		return '{', true
	}
	if p.lit(`\}`, pegEscRBrace) {
		return '}', true
	}
	s0 := p.pos
	if !p.lit(`\u`, pegEscU) {
		return 0, false
	}
	var v uint16
	for i := 0; i < 4; i++ {
		h, ok := p.class(isHexUnit, pegHexDigit)
		if !ok {
			p.pos = s0
			return 0, false
		}
		d, _ := strconv.ParseUint(string(rune(h)), 16, 8)
		v = v<<4 | uint16(d)
	}
	return v, true
}

func (p *pegParser) digits() (float64, bool) {
	start := p.pos
	for {
		if _, ok := p.class(isDigitUnit, pegDigit); !ok {
			break
		}
	}
	if p.pos == start {
		return 0, false
	}
	f, _ := strconv.ParseFloat(fromUnits(p.in[start:p.pos]), 64)
	return f, true
}

func pegStringEscape(u uint16) string {
	hex := strings.ToUpper(strconv.FormatUint(uint64(u), 16))
	switch {
	case u == '\\':
		return `\\`
	case u == '"':
		return `\"`
	case u == 0x08:
		return `\b`
	case u == '\t':
		return `\t`
	case u == '\n':
		return `\n`
	case u == 0x0C:
		return `\f`
	case u == '\r':
		return `\r`
	case u <= 0x07 || u == 0x0B || u == 0x0E || u == 0x0F:
		return `\x0` + hex
	case u >= 0x10 && u <= 0x1F || u >= 0x80 && u <= 0xFF:
		return `\x` + hex
	case u >= 0x0180 && u <= 0x0FFF:
		return `\u0` + hex
	case u >= 0x1080:
		return `\u` + hex
	}
	return fromUnits([]uint16{u})
}

func messageParse(input string) *mfNode {
	p := &pegParser{in: utf16Units(input)}
	result := p.messageFormatPattern()
	if p.pos == len(p.in) {
		return result
	}
	p.fail(&pegExpect{"end", "end of input"})
	expected := p.maxFailExpected
	sort.SliceStable(expected, func(i, j int) bool { return expected[i].description < expected[j].description })
	for i := 1; i < len(expected); {
		if expected[i-1] == expected[i] {
			expected = append(expected[:i], expected[i+1:]...)
		} else {
			i++
		}
	}
	descs := make([]string, len(expected))
	for i, e := range expected {
		descs[i] = e.description
	}
	var expectedDesc string
	switch {
	case len(descs) > 1:
		expectedDesc = strings.Join(descs[:len(descs)-1], ", ") + " or " + descs[len(descs)-1]
	case len(descs) == 1:
		expectedDesc = descs[0]
	default:
		expectedDesc = "undefined"
	}
	foundDesc := "end of input"
	if p.maxFailPos < len(p.in) {
		foundDesc = `"` + pegStringEscape(p.in[p.maxFailPos]) + `"`
	}
	panic(&Error{Name: "SyntaxError", Message: "Expected " + expectedDesc + " but " + foundDesc + " found."})
}

type mfExpr func(d any) any

type mfData struct {
	keys    *object
	offset  *object
	pfCount any
}

// copy is JSON.parse(JSON.stringify(data)).
func (d *mfData) copy() *mfData {
	return &mfData{keys: jsonCopyObject(d.keys), offset: jsonCopyObject(d.offset), pfCount: d.pfCount}
}

func jsonCopyObject(o *object) *object {
	cp := newObject()
	forIn(o, func(k string, v any) {
		if f, ok := v.(float64); ok && (math.IsInf(f, 0) || math.IsNaN(f)) {
			v = nil
		}
		cp.set(k, v)
	})
	return cp
}

func (d *mfData) level(delta float64) string { return numToString(toNumber(d.pfCount) + delta) }

type mfCompiler struct {
	plural func(any) string
}

// mfProp is the generated code's d.key / d["key"].
func mfProp(d any, key string) any { return propOf(d, key) }

type mfForms map[string]mfExpr

func mfFormKey(k any) string {
	if f, ok := k.(float64); ok {
		return numToString(f)
	}
	return k.(string)
}

func (m *mfCompiler) forms(ast *mfNode, data *mfData) mfForms {
	if !jsTruthy(data.pfCount) {
		data.pfCount = 0.0
	}
	if ast.typ == "selectFormatPattern" {
		data.offset.set(data.level(0), 0.0)
	}
	needOther := true
	forms := mfForms{}
	for _, f := range ast.pluralForms {
		if f.key == "other" {
			needOther = false
		}
		dc := data.copy()
		dc.pfCount = toNumber(dc.pfCount) + 1
		forms[mfFormKey(f.key)] = m.precompile(f.val, dc)
	}
	if needOther {
		throwError("Error", "No 'other' form found in "+ast.typ+" "+jsString(data.pfCount))
	}
	return forms
}

func mfSelect(d, value any, forms mfForms) any {
	if f, ok := forms[jsString(jsToPrimitiveDefault(value))]; ok {
		return f(d)
	}
	return forms["other"](d)
}

func mfPlural(d, value any, offset float64, lcfunc func(any) string, forms mfForms) any {
	if f, ok := forms[jsString(jsToPrimitiveDefault(value))]; ok {
		return f(d)
	}
	if offset != 0 {
		value = toNumber(value) - offset
	}
	if f, ok := forms[lcfunc(value)]; ok {
		return f(d)
	}
	return forms["other"](d)
}

func mfNumber(value any, offset float64) any {
	n := toNumber(value)
	if math.IsNaN(n) {
		throwError("Error", "'"+jsString(jsToPrimitiveDefault(value))+"' isn't a number.")
	}
	return n - offset
}

func (m *mfCompiler) precompile(ast *mfNode, data *mfData) mfExpr {
	switch ast.typ {
	case "messageFormatPattern":
		exprs := make([]mfExpr, 0, len(ast.statements))
		for _, st := range ast.statements {
			exprs = append(exprs, m.precompile(st, data))
		}
		if len(exprs) == 0 {
			return func(any) any { return "" }
		}
		return func(d any) any {
			v := exprs[0](d)
			for _, e := range exprs[1:] {
				v = jsAdd(v, e(d))
			}
			return v
		}
	case "messageFormatElement":
		if !jsTruthy(data.pfCount) {
			data.pfCount = 0.0
		}
		if ast.output {
			key := ast.argumentIndex
			return func(d any) any { return mfProp(d, key) }
		}
		data.keys.set(data.level(0), ast.argumentIndex)
		return m.precompile(ast.elementFormat, data)
	case "elementFormat":
		argKey := jsString(data.keys.prop(data.level(0)))
		if ast.pattern == nil && (ast.key == "select" || ast.key == "selectordinal" || ast.key == "plural") {
			throwError("Error", "Bad AST type: undefined")
		}
		switch ast.key {
		case "select":
			forms := m.forms(ast.pattern, data)
			return func(d any) any { return mfSelect(d, mfProp(d, argKey), forms) }
		case "selectordinal":
			forms := m.forms(ast.pattern, data)
			return func(d any) any { return mfPlural(d, mfProp(d, argKey), 0, m.plural, forms) }
		case "plural":
			lvl := "0"
			if jsTruthy(data.pfCount) {
				lvl = data.level(0)
			}
			data.offset.set(lvl, ast.pattern.offset)
			offset := toNumber(data.offset.prop(data.level(0)))
			if offset != offset {
				offset = 0
			}
			forms := m.forms(ast.pattern, data)
			return func(d any) any { return mfPlural(d, mfProp(d, argKey), offset, m.plural, forms) }
		}
		key := ast.key
		return func(d any) any {
			mfProp(d, argKey)
			throwTypeError("fmt." + key + " is not a function")
			return nil
		}
	case "string":
		s := ast.str
		return func(any) any { return s }
	case "octothorpe":
		if !jsTruthy(data.pfCount) {
			return func(any) any { return "#" }
		}
		key := jsString(data.keys.prop(data.level(-1)))
		offset := toNumber(data.offset.prop(data.level(-1)))
		if offset != offset {
			offset = 0
		}
		return func(d any) any { return mfNumber(mfProp(d, key), offset) }
	}
	throwError("Error", "Bad AST type: "+ast.typ)
	return nil
}

func compileMessage(message string, plural func(any) string) mfExpr {
	parsed := false
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}
			stage := "Parser"
			if parsed {
				stage = "Precompiler"
			}
			text := e.Name
			if e.Message != "" {
				text += ": " + e.Message
			}
			panic(&Error{Name: "Error", Message: stage + " error: " + text})
		}
	}()
	ast := messageParse(message)
	parsed = true
	m := &mfCompiler{plural: plural}
	return m.precompile(ast, &mfData{keys: newObject(), offset: newObject(), pfCount: Undefined})
}

// LoadMessages is Globalize.loadMessages(json).
func LoadMessages(json any) {
	json = fromPublic(json)
	custom := newObject()
	custom.set("globalize-messages", json)
	mainData := newObject()
	custom.set("main", mainData)
	validateParameterPresence(json, "json")
	validateParameterTypePlainObject(json, "json")
	forIn(json, func(locale string, _ any) { mainData.set(locale, newObject()) })
	cldrLoad([]any{custom})
}

func arrayJoinJS(a []any, sep string) string {
	parts := make([]string, len(a))
	for i, v := range a {
		if v == nil || isUndefined(v) {
			continue
		}
		parts[i] = jsString(v)
	}
	return strings.Join(parts, sep)
}

func (g *Globalize) messageFormatter(path any) func(args ...any) any {
	validateParameterPresence(path, "path")
	_, isStr := path.(string)
	validateParameterType(path, "path", isStr || isArray(path), "a String nor an Array")
	var p []any
	switch t := path.(type) {
	case []any:
		p = t
	default:
		if jsTruthy(path) {
			p = []any{path}
		}
	}
	c := g.cldr
	validateDefaultLocale(c)
	bundle := c.attributes.prop("bundle")
	validate("E_MISSING_MESSAGE_BUNDLE", "Missing message bundle for locale `{locale}`.",
		jsTruthy(bundle) && !isUndefined(c.get("globalize-messages/{bundle}")), attrs("locale", c.locale))
	message := c.get(append([]any{"globalize-messages/{bundle}"}, p...))
	pathStr := arrayJoinJS(p, "/")
	validate("E_MISSING_MESSAGE", "Missing required message content `{path}`.", !isUndefined(message), attrs("path", pathStr))
	if arr, ok := message.([]any); ok {
		message = arrayJoinJS(arr, " ")
	}
	msg, ok := message.(string)
	validate("E_INVALID_MESSAGE", "Invalid message content `{path}`. {expected} expected.", ok,
		attrs("expected", "a string", "path", pathStr))
	plural := g.pluralGenerator(Undefined)
	formatter := compileMessage(msg, plural)
	return func(args ...any) any {
		var variables any = Undefined
		if len(args) > 0 {
			variables = args[0]
		}
		switch variables.(type) {
		case float64, string:
			variables = append([]any(nil), args...)
		}
		validateParameterType(variables, "variables", isUndefined(variables) || isPlainObject(variables) || isArray(variables),
			"Array or Plain Object")
		return formatter(variables)
	}
}

// MessageFormatter is .messageFormatter(path). The formatter takes the variables Globalize's
// does: one object or array, or positional values when the first is a number or a string.
func (g *Globalize) MessageFormatter(path any) func(variables ...any) any {
	f := g.messageFormatter(fromPublic(path))
	return func(variables ...any) any {
		args := make([]any, len(variables))
		for i, v := range variables {
			args[i] = fromPublic(v)
		}
		return toPublic(f(args...))
	}
}

// FormatMessage is .formatMessage(path[, variables...]).
func (g *Globalize) FormatMessage(path any, variables ...any) any {
	return g.MessageFormatter(path)(variables...)
}

// MessageFormatter is Globalize.messageFormatter(path) on the default locale.
func MessageFormatter(path any) func(variables ...any) any { return statics.MessageFormatter(path) }

// FormatMessage is Globalize.formatMessage(path[, variables...]).
func FormatMessage(path any, variables ...any) any { return statics.FormatMessage(path, variables...) }
