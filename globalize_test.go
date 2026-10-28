package globalize_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	globalize "github.com/globalizejs/globalize-go"
)

// The tests read CLDR 36 JSON laid out the way the cldr-data package lays it out, from
// GLOBALIZE_CLDR (default /opt/cldr-data), and the iana-tz-data JSON from GLOBALIZE_TZ (default
// /opt/iana-tz-data/iana-tz-data.json). They are skipped when the data is not there.
func env(name, fallback string) string {
	if v := os.Getenv(name); v != "" {
		return v
	}
	return fallback
}

var (
	cldrRoot = env("GLOBALIZE_CLDR", "/opt/cldr-data")
	tzFile   = env("GLOBALIZE_TZ", "/opt/iana-tz-data/iana-tz-data.json")
	loadOnce sync.Once
	loadErr  error
)

// decode reads one JSON value, objects as globalize.Obj with their keys in file order.
func decode(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil
	}
	switch delim {
	case '{':
		o := globalize.Obj{}
		for dec.More() {
			key, err := dec.Token()
			if err != nil {
				return nil, err
			}
			v, err := decode(dec)
			if err != nil {
				return nil, err
			}
			o = append(o, key, v)
		}
		_, err = dec.Token()
		return o, err
	case '[':
		a := []any{}
		for dec.More() {
			v, err := decode(dec)
			if err != nil {
				return nil, err
			}
			a = append(a, v)
		}
		_, err = dec.Token()
		return a, err
	}
	return nil, fmt.Errorf("unexpected %v", delim)
}

func readJSON(path string) (any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return decode(json.NewDecoder(f))
}

func load() error {
	files := []string{"supplemental/likelySubtags", "supplemental/numberingSystems", "supplemental/plurals",
		"supplemental/ordinals", "supplemental/currencyData", "supplemental/timeData", "supplemental/weekData",
		"supplemental/metaZones"}
	for _, locale := range []string{"en", "de", "ar", "ru"} {
		for _, name := range []string{"numbers", "currencies", "units", "dateFields", "ca-gregorian", "timeZoneNames"} {
			files = append(files, "main/"+locale+"/"+name)
		}
	}
	data := make([]any, len(files))
	for i, name := range files {
		v, err := readJSON(filepath.Join(cldrRoot, name+".json"))
		if err != nil {
			return err
		}
		data[i] = v
	}
	globalize.Load(data...)
	tz, err := readJSON(tzFile)
	if err != nil {
		return err
	}
	globalize.LoadTimeZone(tz)
	globalize.LoadMessages(globalize.Obj{"en", globalize.Obj{
		"likes", "{count, plural, one{# like} other{# likes}}",
		"hi", "Hello, {0}!",
		"bad", "{a, plural, one{x}",
	}})
	return nil
}

func setup(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(cldrRoot, "supplemental", "likelySubtags.json")); err != nil {
		t.Skipf("no CLDR JSON under %s (set GLOBALIZE_CLDR): %v", cldrRoot, err)
	}
	loadOnce.Do(func() { loadErr = load() })
	if loadErr != nil {
		t.Fatal(loadErr)
	}
}

// call returns f's result, or the exception it raises as "E:<name>: <message>".
func call(f func() any) (v any) {
	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*globalize.Error)
			if !ok {
				panic(r)
			}
			v = "E:" + e.Name + ": " + e.Message
		}
	}()
	return f()
}

type check struct {
	name string
	f    func() any
	want any
}

func run(t *testing.T, checks []check) {
	t.Helper()
	for _, c := range checks {
		if got := call(c.f); got != c.want {
			t.Errorf("%s: got %#v, want %#v", c.name, got, c.want)
		}
	}
}

func TestNumbers(t *testing.T) {
	setup(t)
	en, de, ar := globalize.New("en"), globalize.New("de"), globalize.New("ar")
	run(t, []check{
		{"de grouping and decimals", func() any { return de.FormatNumber(1234567.891) }, "1.234.567,891"},
		{"ar digits and signs", func() any { return ar.FormatNumber(-1234.5) }, "\u061c-\u0661\u066c\u0662\u0663\u0664\u066b\u0665"},
		{"en percent", func() any {
			return en.FormatNumber(0.256, globalize.Obj{"style", "percent", "maximumFractionDigits", 1})
		}, "25.6%"},
		{"en compact", func() any { return en.FormatNumber(1234567, globalize.Obj{"compact", "short"}) }, "1M"},
		{"de parse", func() any { return de.ParseNumber("1.234,5") }, 1234.5},
		{"invalid value", func() any { return en.FormatNumber("x") }, "E:value: E_INVALID_PAR_TYPE: Invalid `value` parameter (x). Number expected."},
	})
}

func TestCurrencies(t *testing.T) {
	setup(t)
	en, de := globalize.New("en"), globalize.New("de")
	run(t, []check{
		{"de symbol", func() any { return de.FormatCurrency(3.5, "EUR") }, "3,50\u00a0\u20ac"},
		{"en name", func() any { return en.FormatCurrency(2, "USD", globalize.Obj{"style", "name"}) }, "2.00 US dollars"},
		{"en accounting", func() any { return en.FormatCurrency(-5, "USD", globalize.Obj{"style", "accounting"}) }, "($5.00)"},
	})
}

func TestPlurals(t *testing.T) {
	setup(t)
	en, ru := globalize.New("en"), globalize.New("ru")
	run(t, []check{
		{"en cardinal", func() any { return en.Plural(1) }, "one"},
		{"ru cardinal", func() any { return ru.Plural(5) }, "many"},
		{"en ordinal", func() any { return en.Plural(2, globalize.Obj{"type", "ordinal"}) }, "two"},
	})
}

func TestDates(t *testing.T) {
	setup(t)
	en, de := globalize.New("en"), globalize.New("de")
	run(t, []check{
		{"en full", func() any { return en.FormatDate(globalize.Date(0), globalize.Obj{"datetime", "full"}) }, "Thursday, January 1, 1970 at 12:00:00 AM GMT"},
		{"de skeleton", func() any { return de.FormatDate(globalize.Date(1709211900000), globalize.Obj{"skeleton", "yMMMdHm"}) }, "29. Feb. 2024, 13:05"},
		{"en time zone", func() any {
			return en.FormatDate(globalize.Date(1719835200000), globalize.Obj{"datetime", "long", "timeZone", "America/New_York"})
		}, "July 1, 2024 at 8:00:00 AM EDT"},
		{"en parse", func() any { return float64(*en.ParseDate("2/29/24", globalize.Obj{"date", "short"})) }, float64(1709164800000)},
	})
}

func TestRelativeTimeAndUnits(t *testing.T) {
	setup(t)
	en, de := globalize.New("en"), globalize.New("de")
	run(t, []check{
		{"en yesterday", func() any { return en.FormatRelativeTime(-1, "day") }, "yesterday"},
		{"de future", func() any { return de.FormatRelativeTime(3, "hour") }, "in 3 Stunden"},
		{"en long unit", func() any { return en.FormatUnit(3, "hour", globalize.Obj{"form", "long"}) }, "3 hours"},
		{"de compound unit", func() any { return de.FormatUnit(60, "mile-per-hour", globalize.Obj{"form", "short"}) }, "60 mi/h"},
	})
}

func TestMessages(t *testing.T) {
	setup(t)
	en := globalize.New("en")
	run(t, []check{
		{"plural message", func() any { return en.FormatMessage("likes", globalize.Obj{"count", 2}) }, "2 likes"},
		{"positional variable", func() any { return en.FormatMessage("hi", "World") }, "Hello, World!"},
		{"missing message", func() any { return en.FormatMessage("nope") }, "E:Error: E_MISSING_MESSAGE: Missing required message content `nope`."},
		{"syntax error", func() any { return en.FormatMessage("bad", globalize.Obj{"a", 1}) }, "E:Error: Parser error: SyntaxError: Expected \"=\", \"}\" or identifier but end of input found."},
	})
}
