package kv

import (
	"errors"
	"reflect"
	"testing"
)

func TestMemStoreRoundTrip(t *testing.T) {
	var s Store = NewMemStore()
	if _, err := s.Get("a"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get on empty = %v, want ErrNotFound", err)
	}
	if err := s.Set("a", "1"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("b", "2"); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("a", "one"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Get("a"); err != nil || v != "one" {
		t.Errorf("Get(a) = %q, %v; want one (Set must replace)", v, err)
	}
	if v, err := s.Get("b"); err != nil || v != "2" {
		t.Errorf("Get(b) = %q, %v", v, err)
	}
}

func TestMemStoreDeleteAndKeys(t *testing.T) {
	s := NewMemStore()
	for _, k := range []string{"zeta", "alpha", "mid"} {
		_ = s.Set(k, k)
	}
	if got := s.Keys(); !reflect.DeepEqual(got, []string{"alpha", "mid", "zeta"}) {
		t.Errorf("Keys = %v, want sorted", got)
	}
	if err := s.Delete("mid"); err != nil {
		t.Errorf("Delete existing = %v", err)
	}
	if err := s.Delete("mid"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Delete missing = %v, want ErrNotFound", err)
	}
	if _, err := s.Get("mid"); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get after delete = %v, want ErrNotFound", err)
	}
	if got := s.Keys(); !reflect.DeepEqual(got, []string{"alpha", "zeta"}) {
		t.Errorf("Keys after delete = %v", got)
	}
	if got := NewMemStore().Keys(); len(got) != 0 {
		t.Errorf("Keys on empty = %v", got)
	}
}
