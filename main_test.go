package main

import (
	"strings"
	"testing"

	"github.com/bytedance/sonic"
)

func newTestBuildstr() *buildstr {
	return &buildstr{
		builder:     &strings.Builder{},
		table:       map[string]sonic.NoCopyRawMessage{},
		printed:     map[string]struct{}{},
		order:       reservedOrder,
		orderCustom: []string{},
		orderSet:    makeSet(reservedOrder...),
	}
}

// TestFormatScalars pins the current scalar-decoding behavior of buildstr.format:
// scalars are decoded into interface{} and printed via fmt.Sprint, so strings
// lose their quotes, numbers print as float64 (42 -> "42"), null -> "<nil>".
// Objects and arrays are passed through as raw bytes, untouched.
func TestFormatScalars(t *testing.T) {
	b := newTestBuildstr()
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"string", `"abc"`, `abc`},
		{"string with space", `"hello world"`, `hello world`},
		{"int", `42`, `42`},
		{"float", `1.5`, `1.5`},
		{"float half", `0.5`, `0.5`},
		{"negative", `-3`, `-3`},
		{"bool true", `true`, `true`},
		{"bool false", `false`, `false`},
		{"null", `null`, `<nil>`},
		{"object raw", `{"a":1}`, `{"a":1}`},
		{"object raw spacing kept", `{"a": 1, "b": 2}`, `{"a": 1, "b": 2}`},
		{"array raw", `[1,2,3]`, `[1,2,3]`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := b.format([]byte(c.raw))
			if got != c.want {
				t.Errorf("format(%q) = %q, want %q", c.raw, got, c.want)
			}
		})
	}
}

// TestBuildstrFormat pins the full line output: reserved columns first
// (time, level, message present here), then other fields byte-sorted;
// every column is tab-terminated (including the last) and a newline ends the line.
func TestBuildstrFormat(t *testing.T) {
	b := newTestBuildstr()
	b.table = map[string]sonic.NoCopyRawMessage{
		"time":    []byte(`"2026-11-06T10:00:00Z"`),
		"level":   []byte(`"info"`),
		"message": []byte(`"hello"`),
		"count":   []byte(`42`),
		"obj":     []byte(`{"a":1}`),
	}
	got := b.Format()
	want := "2026-11-06T10:00:00Z\tinfo\thello\t42\t{\"a\":1}\t\n"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

// TestBuildstrFormatWithField pins the -field flag behavior: "<name>: " is
// prepended to each column.
func TestBuildstrFormatWithField(t *testing.T) {
	b := newTestBuildstr()
	b.withFiled = true
	b.table = map[string]sonic.NoCopyRawMessage{
		"level":   []byte(`"info"`),
		"message": []byte(`"hello"`),
		"nothing": []byte(`null`),
	}
	got := b.Format()
	want := "level: info\tmessage: hello\tnothing: <nil>\t\n"
	if got != want {
		t.Errorf("Format() = %q, want %q", got, want)
	}
}

// TestBuildstrFormatEmpty pins that an empty table yields an empty string (no newline).
func TestBuildstrFormatEmpty(t *testing.T) {
	b := newTestBuildstr()
	if got := b.Format(); got != "" {
		t.Errorf("Format() on empty table = %q, want \"\"", got)
	}
}

// TestBuildstrFormatNoOrderCustomDups pins that repeated Format() calls with the
// same custom field add it to orderCustom exactly once. If orderSet is not
// updated on append, every line re-appends the field and Format() degrades to O(n^2).
func TestBuildstrFormatNoOrderCustomDups(t *testing.T) {
	b := newTestBuildstr()
	for i := 0; i < 3; i++ {
		b.table = map[string]sonic.NoCopyRawMessage{
			"level":   []byte(`"info"`),
			"message": []byte(`"hello"`),
			"count":   []byte(`42`),
		}
		if got := b.Format(); got == "" {
			t.Fatalf("Format() on iteration %d = empty", i)
		}
		b.Reset()
	}
	if len(b.orderCustom) != 1 || b.orderCustom[0] != "count" {
		t.Errorf("after 3 Format() calls orderCustom = %v, want exactly [\"count\"]", b.orderCustom)
	}
}

// TestNoStaleFieldsAfterUnmarshalError mirrors the main loop across three lines:
// valid line introducing field "a" -> invalid line (unmarshal error) -> valid
// line without "a". The error branch must not leave partial keys in b.table,
// otherwise field "a" leaks into the third line's output.
func TestNoStaleFieldsAfterUnmarshalError(t *testing.T) {
	b := newTestBuildstr()

	// line 1: valid, introduces custom field "a"
	if err := unmarshalLine(b, []byte(`{"time":"t1","a":"1"}`)); err != nil {
		t.Fatal(err)
	}
	if got := b.Format(); got == "" {
		t.Fatal("Format() on line 1 = empty")
	}
	b.Reset()

	// line 2: invalid — unmarshal must fail
	if err := unmarshalLine(b, []byte(`{"a":"1","b":`)); err == nil {
		t.Fatal("line 2: unmarshalLine succeeded on invalid JSON")
	}

	// line 3: valid, without field "a"
	if err := unmarshalLine(b, []byte(`{"time":"t2"}`)); err != nil {
		t.Fatal(err)
	}
	got := b.Format()
	if !strings.Contains(got, "t2") {
		t.Fatalf("line 3 output missing time value: %q", got)
	}
	if strings.Contains(got, "1") {
		t.Errorf("stale field from invalid line leaked into line 3: %q", got)
	}
}
