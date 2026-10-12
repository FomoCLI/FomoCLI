// Package jx is a tiny, nil-safe accessor for decoded JSON.
//
// FOMO API is an independent, evolving API. Rather than binding every payload
// to a rigid struct (and breaking whenever a field is added, renamed or turns
// null), renderers read fields through jx: a missing or mistyped field simply
// yields a zero value and the CLI keeps working.
package jx

import (
	"encoding/json"
	"strconv"
	"strings"
)

// V wraps any decoded JSON value.
type V struct{ raw any }

// Parse decodes JSON bytes into a V. Numbers are kept as json.Number so large
// integers survive intact.
func Parse(b []byte) (V, error) {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var x any
	if err := dec.Decode(&x); err != nil {
		return V{}, err
	}
	return V{x}, nil
}

// Wrap turns an already-decoded value into a V.
func Wrap(x any) V { return V{x} }

// Raw returns the underlying value.
func (v V) Raw() any { return v.raw }

// Get walks a path of object keys and returns the value found (or a null V).
func (v V) Get(path ...string) V {
	cur := v.raw
	for _, k := range path {
		m, ok := cur.(map[string]any)
		if !ok {
			return V{}
		}
		cur = m[k]
	}
	return V{cur}
}

// First returns the first non-null value among several candidate keys.
func (v V) First(keys ...string) V {
	for _, k := range keys {
		if x := v.Get(k); !x.IsNull() {
			return x
		}
	}
	return V{}
}

// Index returns element i of an array.
func (v V) Index(i int) V {
	a, ok := v.raw.([]any)
	if !ok || i < 0 || i >= len(a) {
		return V{}
	}
	return V{a[i]}
}

// Array returns the elements of an array (nil if not an array).
func (v V) Array() []V {
	a, ok := v.raw.([]any)
	if !ok {
		return nil
	}
	out := make([]V, len(a))
	for i, x := range a {
		out[i] = V{x}
	}
	return out
}

// Keys returns the keys of an object in unspecified order.
func (v V) Keys() []string {
	m, ok := v.raw.(map[string]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func (v V) IsNull() bool   { return v.raw == nil }
func (v V) IsObject() bool { _, ok := v.raw.(map[string]any); return ok }
func (v V) IsArray() bool  { _, ok := v.raw.([]any); return ok }

// Exists reports whether the value is present and not null.
func (v V) Exists() bool { return v.raw != nil }

// Str returns the value as a string. Numbers and bools are formatted.
func (v V) Str() string {
	switch x := v.raw.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	}
	return ""
}

// Float returns the value as a float64 and whether it was numeric.
func (v V) Float() (float64, bool) {
	switch x := v.raw.(type) {
	case json.Number:
		f, err := x.Float64()
		return f, err == nil
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(x, 64)
		return f, err == nil
	}
	return 0, false
}

// Num returns the value as a float64 (0 if not numeric).
func (v V) Num() float64 { f, _ := v.Float(); return f }

// Int returns the value as an int64 (0 if not numeric).
func (v V) Int() int64 {
	if n, ok := v.raw.(json.Number); ok {
		if i, err := n.Int64(); err == nil {
			return i
		}
	}
	return int64(v.Num())
}

// Bool returns the value as a bool (false if not a bool).
func (v V) Bool() bool { b, _ := v.raw.(bool); return b }

// Len returns the length of an array or object.
func (v V) Len() int {
	switch x := v.raw.(type) {
	case []any:
		return len(x)
	case map[string]any:
		return len(x)
	}
	return 0
}

// List finds the main array in a payload: the value itself if it is an array,
// otherwise the first of the given keys holding an array.
func (v V) List(keys ...string) []V {
	if v.IsArray() {
		return v.Array()
	}
	for _, k := range keys {
		if a := v.Get(k); a.IsArray() {
			return a.Array()
		}
	}
	return nil
}

// JSON re-encodes the value as indented JSON.
func (v V) JSON() string {
	b, err := json.MarshalIndent(v.raw, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}
