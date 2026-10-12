package jx

import "testing"

func TestAccessors(t *testing.T) {
	v, err := Parse([]byte(`{"a":{"b":[1,"x",null]},"n":"12.5","big":12345678901234567,"t":true,"list":[{"k":1}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if v.Get("a", "b").Index(0).Int() != 1 || v.Get("a", "b").Index(1).Str() != "x" || !v.Get("a", "b").Index(2).IsNull() {
		t.Error("nested access")
	}
	if v.Get("n").Num() != 12.5 {
		t.Error("numeric string")
	}
	if v.Get("big").Int() != 12345678901234567 {
		t.Error("big int precision")
	}
	if !v.Get("t").Bool() || v.Get("missing", "deep").Exists() {
		t.Error("bool / missing")
	}
	if v.First("nope", "n").Str() != "12.5" {
		t.Error("First")
	}
	if len(v.List("nope", "list")) != 1 || v.Len() != 5 {
		t.Error("List/Len")
	}
	if _, ok := v.Get("a").Float(); ok {
		t.Error("object is not numeric")
	}
}
