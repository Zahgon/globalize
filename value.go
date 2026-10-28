// Package globalize is a Go port of Globalize, the CLDR-driven internationalization library
// (number, currency, date, plural, relative time, unit and message formatting and parsing).
//
// The API mirrors Globalize's JavaScript API. JavaScript values are modelled as follows:
// numbers are float64, strings are string, booleans are bool, null is nil, undefined is
// Undefined, object literals are Obj (keys and values alternating, in insertion order), arrays
// are []any and a JavaScript Date is Date. Everything Globalize throws is raised as a panic
// carrying an *Error.
package globalize

import (
	"math"
	"strings"
)

// Obj is a JavaScript object literal: its keys and values alternate, in insertion order.
type Obj []any

// Get returns the value stored under key, looking the pairs up the way a JavaScript object
// with those properties would (a later duplicate key wins).
func (o Obj) Get(key string) (any, bool) {
	var v any
	found := false
	for i := 0; i+1 < len(o); i += 2 {
		if k, ok := o[i].(string); ok && k == key {
			v, found = o[i+1], true
		}
	}
	return v, found
}

// Keys returns the keys of the object o stands for, in JavaScript property order.
func (o Obj) Keys() []string {
	return toObject(o).keys
}

type undefinedType struct{}

func (undefinedType) String() string { return "undefined" }

// Undefined is the JavaScript undefined.
var Undefined any = undefinedType{}

// Error is what every exception the original throws becomes: Name is the JavaScript constructor
// name ("Error", "SyntaxError", "TypeError", ...) and Message the exception message.
type Error struct {
	Name    string
	Message string
}

func (e *Error) Error() string {
	if e.Message == "" {
		return e.Name
	}
	return e.Name + ": " + e.Message
}

// Date is a JavaScript Date: its time value in milliseconds since the epoch, NaN when invalid.
type Date float64

// Part is one element of a formatToParts result.
type Part struct {
	Type  string
	Value string
}

// Now returns the current time. Globalize reads the clock only when it parses a date that does
// not fix every field; replace Now to pin it.
var Now = func() Date {
	return Date(float64(nowMillis()))
}

func throwError(name, message string) {
	panic(&Error{Name: name, Message: message})
}

func throwTypeError(message string) {
	throwError("TypeError", message)
}

func isUndefined(v any) bool {
	_, ok := v.(undefinedType)
	return ok
}

func jsTruthy(v any) bool {
	switch x := v.(type) {
	case nil, undefinedType:
		return false
	case bool:
		return x
	case float64:
		return !(x == 0 || math.IsNaN(x))
	case string:
		return x != ""
	}
	return true
}

func jsTypeof(v any) string {
	switch v.(type) {
	case undefinedType:
		return "undefined"
	case nil:
		return "object"
	case bool:
		return "boolean"
	case float64:
		return "number"
	case string:
		return "string"
	case func(args ...any) any:
		return "function"
	}
	return "object"
}

func isArray(v any) bool {
	_, ok := v.([]any)
	return ok
}

// jsString is the JavaScript String(v) / "" + v conversion.
func jsString(v any) string {
	switch x := v.(type) {
	case undefinedType:
		return "undefined"
	case nil:
		return "null"
	case bool:
		if x {
			return "true"
		}
		return "false"
	case float64:
		return numToString(x)
	case string:
		return x
	case []any:
		parts := make([]string, len(x))
		for i, e := range x {
			if e == nil || isUndefined(e) {
				continue
			}
			parts[i] = jsString(e)
		}
		return strings.Join(parts, ",")
	case Date:
		return dateToString(float64(x))
	case *object:
		return "[object Object]"
	case *Cldr:
		return "[object Object]"
	case func(args ...any) any:
		return "function () { [native code] }"
	}
	return "[object Object]"
}

// isPlainObject is Globalize's `obj !== null && "" + obj === "[object Object]"`.
func isPlainObject(v any) bool {
	if v == nil || isUndefined(v) {
		return false
	}
	switch v.(type) {
	case *object, *Cldr:
		return true
	case []any:
		return jsString(v) == "[object Object]"
	}
	return false
}
