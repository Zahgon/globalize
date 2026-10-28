// Locale resolution, CLDR traversal and the data events in this file are a port of cldrjs 0.5.5,
// the CLDR JavaScript Library Globalize 1.7.1 depends on:
//
// Copyright (c) Rafael Xavier de Souza http://rafael.xavier.blog.br
//
// Permission is hereby granted, free of charge, to any person
// obtaining a copy of this software and associated documentation
// files (the "Software"), to deal in the Software without
// restriction, including without limitation the rights to use,
// copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following
// conditions:
//
// The above copyright notice and this permission notice shall be
// included in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND,
// EXPRESS OR IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES
// OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE AND
// NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR COPYRIGHT
// HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY,
// WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

package globalize

import (
	"regexp"
	"strings"
)

// This file ports cldrjs 0.5.5 (core, event, supplemental and unresolved modules), the CLDR data
// layer Globalize runs on.

var (
	cldrRaw                 = newObject()
	cldrResolved            = newObject()
	availableBundleMap      = newObject()
	availableBundleMapQueue []string
)

const localeSep = "-"

// Cldr is a cldrjs Cldr instance: a locale resolved against the loaded CLDR data.
type Cldr struct {
	locale     string
	attributes *object
	listeners  []*listener
}

type listener struct {
	key  any
	fn   func(path string, value any)
	once bool
}

func cldrCreateError(code string, attributes *object) {
	msg := code
	if s, ok := jsonStringify(attributes); ok {
		msg += ": " + s
	}
	throwError(errorName(attributes), msg)
}

func cldrAttrs(kv ...any) *object {
	o := newObject()
	for i := 0; i+1 < len(kv); i += 2 {
		o.set(kv[i].(string), kv[i+1])
	}
	return o
}

func cldrValidatePresence(value any, name string) {
	if isUndefined(value) {
		cldrCreateError("E_MISSING_PARAMETER", cldrAttrs("name", name))
	}
}

func cldrValidateType(value any, name string, check bool, expected string) {
	if !check {
		cldrCreateError("E_INVALID_PAR_TYPE", cldrAttrs("expected", expected, "name", name, "value", value))
	}
}

func cldrValidateTypePath(value any, name string) {
	_, isStr := value.(string)
	cldrValidateType(value, name, isStr || isArray(value), "String or Array")
}

func cldrValidateTypePlainObject(value any, name string) {
	cldrValidateType(value, name, isUndefined(value) || isPlainObject(value), "Plain Object")
}

func cldrValidateTypeString(value any, name string) {
	_, isStr := value.(string)
	cldrValidateType(value, name, isStr, "a string")
}

// arrayJoin is Array.prototype.join.
func arrayJoin(a []any, sep string) string {
	parts := make([]string, len(a))
	for i, e := range a {
		if e == nil || isUndefined(e) {
			continue
		}
		parts[i] = jsString(e)
	}
	return strings.Join(parts, sep)
}

var placeholderRe = regexp.MustCompile(`\{[a-zA-Z]+\}`)

func pathNormalize(path any, attributes *object) []string {
	var p string
	switch x := path.(type) {
	case []any:
		p = arrayJoin(x, "/")
	case string:
		p = x
	default:
		throwError("Error", `invalid path "`+jsString(path)+`"`)
	}
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "cldr/")
	p = placeholderRe.ReplaceAllStringFunc(p, func(name string) string {
		name = name[1 : len(name)-1]
		if attributes == nil {
			throwTypeError("Cannot read properties of undefined (reading '" + name + "')")
		}
		return jsString(attributes.prop(name))
	})
	return strings.Split(p, "/")
}

func resourceGet(data any, path []string) any {
	node := data
	i := 0
	for ; i < len(path)-1; i++ {
		node = getProp(node, path[i])
		if !jsTruthy(node) {
			return Undefined
		}
	}
	return getProp(node, path[i])
}

func resourceSet(data *object, path []string, value any) {
	node := any(data)
	i := 0
	for ; i < len(path)-1; i++ {
		if !jsTruthy(getProp(node, path[i])) {
			if o, ok := node.(*object); ok {
				o.set(path[i], newObject())
			}
		}
		node = getProp(node, path[i])
	}
	if o, ok := node.(*object); ok {
		o.set(path[i], value)
	}
}

func isObjectNonArray(v any) bool {
	if v == nil {
		return true
	}
	switch v.(type) {
	case *object, Date, *Cldr:
		return true
	}
	return false
}

// forIn calls fn for every key a JavaScript for-in loop visits on v.
func forIn(v any, fn func(k string, val any)) {
	switch x := v.(type) {
	case *object:
		for _, k := range append([]string(nil), x.keys...) {
			if val, ok := x.vals[k]; ok {
				fn(k, val)
			}
		}
	case []any:
		for i, e := range x {
			fn(numToString(float64(i)), e)
		}
	case string:
		u := utf16Units(x)
		for i := range u {
			fn(numToString(float64(i)), fromUnits(u[i:i+1]))
		}
	}
}

func jsonMerge(sources ...any) *object {
	dest := newObject()
	for _, source := range sources {
		forIn(source, func(prop string, val any) {
			if cur, ok := dest.get(prop); ok && isObjectNonArray(cur) {
				dest.set(prop, jsonMerge(cur, val))
			} else {
				dest.set(prop, val)
			}
		})
	}
	return dest
}

func coreSetAvailableBundles(json any) {
	if json == nil || isUndefined(json) {
		throwTypeError("Cannot read properties of " + jsString(json) + " (reading 'main')")
	}
	main := getProp(json, "main")
	forIn(main, func(bundle string, _ any) {
		if bundle == "root" {
			return
		}
		for _, b := range availableBundleMapQueue {
			if b == bundle {
				return
			}
		}
		availableBundleMapQueue = append(availableBundleMapQueue, bundle)
	})
}

func cldrAlwaysArray(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	return []any{v}
}

func coreLoad(source *object, jsons []any) *object {
	first := any(Undefined)
	if len(jsons) > 0 {
		first = jsons[0]
	}
	cldrValidatePresence(first, "json")
	for _, j := range jsons {
		for _, item := range cldrAlwaysArray(j) {
			cldrValidateTypePlainObject(item, "json")
			source = jsonMerge(source, item)
			coreSetAvailableBundles(item)
		}
	}
	return source
}

// cldrLoad is Cldr.load (the unresolved module's version).
func cldrLoad(jsons []any) {
	cldrRaw = coreLoad(cldrRaw, jsons)
}

var subtagsRe = regexp.MustCompile(`^(([a-z]{2,3})(-([A-Z][a-z]{3}))?(-([A-Z]{2}|[0-9]{3}))?)((-([a-zA-Z0-9]{5,8}|[0-9][a-zA-Z0-9]{3}))*)$|^(root)$`)

// coreSubtags returns the subtags of locale; index 4 holds the -u- extension when present
// (indices 3 and 4 may be absent, which nil marks for index 3).
func coreSubtags(locale string) []any {
	locale = strings.Replace(locale, "_", "-", 1)
	subtags := []any{}
	aux := strings.Split(locale, "-u-")
	if len(aux) > 1 && aux[1] != "" {
		ext := strings.Split(aux[1], "-t-")
		locale = aux[0]
		if len(ext) > 1 && ext[1] != "" {
			locale += "-t-" + ext[1]
		}
		subtags = []any{Undefined, Undefined, Undefined, Undefined, ext[0]}
	}
	unicodeLanguageID := strings.Split(locale, "-t-")[0]
	m := subtagsRe.FindStringSubmatch(unicodeLanguageID)
	if m == nil {
		return []any{"und", "Zzzz", "ZZ"}
	}
	if len(subtags) < 3 {
		subtags = []any{Undefined, Undefined, Undefined}
	}
	switch {
	case m[10] != "":
		subtags[0] = m[10]
	case m[2] != "":
		subtags[0] = m[2]
	default:
		subtags[0] = "und"
	}
	subtags[1] = orStr(m[4], "Zzzz")
	subtags[2] = orStr(m[6], "ZZ")
	if m[7] != "" {
		if len(subtags) < 4 {
			subtags = append(subtags, m[7][1:])
		} else {
			subtags[3] = m[7][1:]
		}
	}
	return subtags
}

func orStr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

var zzzzRe = regexp.MustCompile(`\b(Zzzz|ZZ)\b`)

func coreLikelySubtags(c *Cldr, subtags []any, force bool) []any {
	language, script, territory := subtags[0], subtags[1], subtags[2]
	var variants []any
	if len(subtags) > 3 {
		variants = subtags[3:4]
	}
	if language != "und" && script != "Zzzz" && territory != "ZZ" {
		return append([]any{language, script, territory}, variants...)
	}
	if isUndefined(c.get("supplemental/likelySubtags")) {
		return nil
	}
	tests := [][]any{{language, script, territory}, {language, territory}, {language, script}, {language}, {"und", script}}
	var match any
	found := false
	for _, test := range tests {
		joined := arrayJoin(test, localeSep)
		if zzzzRe.MatchString(joined) {
			match = false
			continue
		}
		match = c.get([]any{"supplemental/likelySubtags", joined})
		if jsTruthy(match) {
			found = true
			break
		}
	}
	if found {
		parts := strings.Split(jsString(match), localeSep)
		at := func(i int) any {
			if i < len(parts) {
				return parts[i]
			}
			return Undefined
		}
		r := []any{language, script, territory}
		if language == "und" {
			r[0] = at(0)
		}
		if script == "Zzzz" {
			r[1] = at(1)
		}
		if territory == "ZZ" {
			r[2] = at(2)
		}
		return append(r, variants...)
	}
	if force {
		und := c.get("supplemental/likelySubtags/und")
		s, ok := und.(string)
		if !ok {
			throwTypeError("Cannot read properties of " + jsString(und) + " (reading 'split')")
		}
		r := []any{}
		for _, p := range strings.Split(s, localeSep) {
			r = append(r, p)
		}
		return r
	}
	return nil
}

func coreRemoveLikelySubtags(c *Cldr, max []any) []any {
	language, script, territory := max[0], max[1], max[2]
	var variants any = Undefined
	if len(max) > 3 {
		variants = max[3]
	}
	tests := [][2][]any{
		{{language, "Zzzz", "ZZ"}, {language}},
		{{language, "Zzzz", territory}, {language, territory}},
		{{language, script, "ZZ"}, {language, script}},
	}
	for _, test := range tests {
		result := coreLikelySubtags(c, test[0], false)
		match := test[1]
		if result != nil && result[0] == max[0] && result[1] == max[1] && result[2] == max[2] {
			if jsTruthy(variants) {
				match = append(match, variants)
			}
			return match
		}
	}
	return max
}

func bundleLookup(c *Cldr, minLanguageID string) any {
	if len(availableBundleMapQueue) > 0 {
		queue := availableBundleMapQueue
		for i := 0; i < len(queue); i++ {
			bundle := queue[i]
			maxBundle := coreLikelySubtags(c, coreSubtags(bundle), false)
			if maxBundle == nil {
				availableBundleMapQueue = append(append([]string(nil), queue[:i]...), queue[i+1:]...)
				throwError("Error", "Could not find likelySubtags for "+bundle)
			}
			minBundle := arrayJoin(coreRemoveLikelySubtags(c, maxBundle), localeSep)
			if existing, ok := availableBundleMap.get(minBundle); ok && jsTruthy(existing) && jsLength(jsString(existing)) < jsLength(bundle) {
				continue
			}
			availableBundleMap.set(minBundle, bundle)
		}
		availableBundleMapQueue = nil
	}
	if v, ok := availableBundleMap.get(minLanguageID); ok && jsTruthy(v) {
		return v
	}
	return nil
}

func bundleParentLookup(locale string) (string, bool) {
	if locale == "root" {
		return "", false
	}
	path := pathNormalize([]any{"supplemental/parentLocales/parentLocale", locale}, nil)
	parent := resourceGet(cldrResolved, path)
	if !jsTruthy(parent) {
		parent = resourceGet(cldrRaw, path)
	}
	if jsTruthy(parent) {
		return jsString(parent), true
	}
	p := ""
	if i := strings.LastIndex(locale, localeSep); i > 0 {
		p = locale[:i]
	}
	if p == "" {
		return "root", true
	}
	return p, true
}

func itemLookup(locale string, defined bool, path any, attributes *object, childLocale *string) any {
	if !defined || (childLocale != nil && locale == *childLocale) {
		return Undefined
	}
	normalized := pathNormalize(path, attributes)
	value := resourceGet(cldrResolved, normalized)
	if !isUndefined(value) && jsTypeof(value) != "object" {
		return value
	}
	value = resourceGet(cldrRaw, normalized)
	if isUndefined(value) {
		parent, ok := bundleParentLookup(locale)
		var parentValue any = Undefined
		if ok {
			parentValue = parent
		}
		var src any = Undefined
		if attributes != nil {
			src = attributes
		}
		child := locale
		value = itemLookup(parent, ok, path, jsonMerge(src, cldrAttrs("bundle", parentValue)), &child)
	}
	if !isUndefined(value) {
		resourceSet(cldrResolved, normalized, value)
	}
	return value
}

// newCldr is new Cldr(locale).
func newCldr(locale any) *Cldr {
	c := &Cldr{}
	c.init(locale)
	return c
}

func (c *Cldr) init(localeArg any) {
	cldrValidatePresence(localeArg, "locale")
	cldrValidateTypeString(localeArg, "locale")
	locale := localeArg.(string)
	subtags := coreSubtags(locale)
	unicodeLocaleExtensions := ""
	unicodeLocaleExtensionsRaw := ""
	if len(subtags) == 5 {
		unicodeLocaleExtensions = subtags[4].(string)
		unicodeLocaleExtensionsRaw = localeSep + "u" + localeSep + unicodeLocaleExtensions
		subtags = subtags[:4]
		if !jsTruthy(subtags[3]) {
			subtags = subtags[:3]
		}
	}
	var variant any = Undefined
	if len(subtags) > 3 {
		variant = subtags[3]
	}
	maxLanguageID := coreLikelySubtags(c, subtags, true)
	if maxLanguageID == nil {
		maxLanguageID = subtags
	}
	minLanguageID := arrayJoin(coreRemoveLikelySubtags(c, maxLanguageID), localeSep)
	attrs := newObject()
	attrs.set("bundle", bundleLookup(c, minLanguageID))
	attrs.set("minLanguageId", minLanguageID+unicodeLocaleExtensionsRaw)
	attrs.set("maxLanguageId", arrayJoin(maxLanguageID, localeSep)+unicodeLocaleExtensionsRaw)
	attrs.set("language", maxLanguageID[0])
	attrs.set("script", maxLanguageID[1])
	attrs.set("territory", maxLanguageID[2])
	attrs.set("region", maxLanguageID[2])
	attrs.set("variant", variant)
	c.attributes = attrs
	if unicodeLocaleExtensions != "" {
		uExtRe.ReplaceAllStringFunc("-"+unicodeLocaleExtensions, func(m string) string {
			sub := uExtRe.FindStringSubmatch(m)
			if sub[1] != "" {
				attrs.set("u"+sub[1], sub[2])
			} else {
				attrs.set("u"+m, true)
			}
			return m
		})
	}
	c.locale = locale
}

var uExtRe = regexp.MustCompile(`-[a-z]{3,8}|(-[a-z]{2})-([a-z]{3,8})`)

func (c *Cldr) on(key any, fn func(string, any)) {
	for _, l := range c.listeners {
		if l.key == key {
			return
		}
	}
	c.listeners = append(c.listeners, &listener{key: key, fn: fn})
}

func (c *Cldr) once(key any, fn func(string, any)) {
	c.listeners = append(c.listeners, &listener{key: key, fn: fn, once: true})
}

func (c *Cldr) off(key any) {
	for i := len(c.listeners) - 1; i >= 0; i-- {
		if c.listeners[i].key == key {
			c.listeners = append(c.listeners[:i:i], c.listeners[i+1:]...)
			return
		}
	}
}

func (c *Cldr) emit(path string, value any) {
	i := len(c.listeners)
	for i > 0 {
		i--
		if i >= len(c.listeners) {
			continue
		}
		l := c.listeners[i]
		if l.once {
			c.off(l.key)
		}
		l.fn(path, value)
	}
}

// get is Cldr.prototype.get as the event module wraps the unresolved module's lookup.
func (c *Cldr) get(path any) any {
	cldrValidatePresence(path, "path")
	cldrValidateTypePath(path, "path")
	bundle := ""
	if c.attributes != nil {
		if b := c.attributes.prop("bundle"); jsTruthy(b) {
			bundle = jsString(b)
		}
	}
	value := itemLookup(bundle, true, path, c.attributes, nil)
	normalized := strings.Join(pathNormalize(path, c.attributes), "/")
	c.emit(normalized, value)
	return value
}

func (c *Cldr) main(path any) any {
	cldrValidatePresence(path, "path")
	cldrValidateTypePath(path, "path")
	if c.attributes.prop("bundle") == nil {
		cldrCreateError("E_MISSING_BUNDLE", cldrAttrs("locale", c.locale))
	}
	return c.get(append([]any{"main/{bundle}"}, cldrAlwaysArray(path)...))
}

func (c *Cldr) supplemental(path any) any {
	return c.get(append([]any{"supplemental"}, cldrAlwaysArray(path)...))
}

func (c *Cldr) weekDataFirstDay() any {
	if v := c.get("supplemental/weekData/firstDay/{territory}"); jsTruthy(v) {
		return v
	}
	return c.get("supplemental/weekData/firstDay/001")
}

func (c *Cldr) weekDataMinDays() float64 {
	v := c.get("supplemental/weekData/minDays/{territory}")
	if !jsTruthy(v) {
		v = c.get("supplemental/weekData/minDays/001")
	}
	return parseIntJS(v)
}

func (c *Cldr) timeDataPreferred() any {
	if v := c.get("supplemental/timeData/{territory}/_preferred"); jsTruthy(v) {
		return v
	}
	return c.get("supplemental/timeData/001/_preferred")
}

func (c *Cldr) jsonView() *object {
	o := newObject()
	o.set("attributes", c.attributes)
	o.set("locale", c.locale)
	return o
}

// Attributes returns the instance's attributes object (bundle, minLanguageId, maxLanguageId,
// language, script, territory, region, variant and any -u- extension keys).
func (c *Cldr) Attributes() Obj {
	return toPublic(c.attributes).(Obj)
}

// Locale returns the locale the instance was created for.
func (c *Cldr) Locale() string { return c.locale }

// Get is cldr.get(path): the CLDR item at path, with parent-locale inheritance.
func (c *Cldr) Get(path any) any { return toPublic(c.get(fromPublic(path))) }

// Main is cldr.main(path): the item at path under main/{bundle}.
func (c *Cldr) Main(path any) any { return toPublic(c.main(fromPublic(path))) }

// Supplemental is cldr.supplemental(path).
func (c *Cldr) Supplemental(path any) any { return toPublic(c.supplemental(fromPublic(path))) }
