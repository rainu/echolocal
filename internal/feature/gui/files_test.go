//go:build !board_biscuit

package gui

import (
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"testing"
)

func TestLocalPhotoUsesOneTransientSourceAndReadOnlySettings(t *testing.T) {
	source := &filePhoto{data: []byte("bounded fixture")}
	p, ok := source.Next()
	if !ok || string(p.Data) != "bounded fixture" {
		t.Fatal("missing selected photo")
	}
	if _, ok = source.Next(); ok {
		t.Fatal("source retained duplicate media")
	}
	settings := filePhotoSettings{}
	if p := settings.Read(); p.AppEvery != schema.DelayNever || p.AppFit != schema.FitContain {
		t.Fatal("local photo must not slideshow/crop", p.AppFit)
	}
	called := false
	settings.Update(func(*schema.Photos) { called = true })
	if called {
		t.Fatal("local app wrote photo settings")
	}
	n := 0
	closer := &mediaCloser{close: func() { n++ }}
	closer.Close()
	closer.Close()
	if n != 1 {
		t.Fatal("not idempotent")
	}
}
