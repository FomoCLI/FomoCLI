package cache

import (
	"testing"
)

func TestPutGetListClear(t *testing.T) {
	s := &Store{Dir: t.TempDir()}
	if e, err := s.Get("/v2/x"); e != nil || err != nil {
		t.Fatalf("empty store: %v %v", e, err)
	}
	if err := s.Put("/v2/x", []byte(`{"a":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Put("/v2/bad", []byte(`not json`)); err != nil {
		t.Fatal(err)
	}
	e, err := s.Get("/v2/x")
	if err != nil || e == nil || string(e.Body) != `{"a":1}` || e.Path != "/v2/x" {
		t.Fatalf("get: %+v %v", e, err)
	}
	if e, _ := s.Get("/v2/bad"); e != nil {
		t.Error("invalid JSON should not be cached")
	}
	items, _ := s.List()
	if len(items) != 1 || items[0].Path != "/v2/x" {
		t.Fatalf("list: %+v", items)
	}
	if n, _ := s.Clear(0); n != 1 {
		t.Fatalf("clear removed %d", n)
	}
	if items, _ := s.List(); len(items) != 0 {
		t.Fatal("not cleared")
	}
}
