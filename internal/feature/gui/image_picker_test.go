//go:build !board_biscuit

package gui

import (
	gogui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/screens"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"
	"testing"
)

type testImagePickerView struct {
	page   *sharedshell.Page
	closed int
}

func (v *testImagePickerView) Covers() bool                       { return true }
func (v *testImagePickerView) ImagePickerPage() *sharedshell.Page { return v.page }
func (v *testImagePickerView) Close()                             { v.closed++ }

type pickerScreenSection struct{}

func (*pickerScreenSection) Read() schema.Screen               { return schema.DefaultScreen() }
func (*pickerScreenSection) Update(func(*schema.Screen)) error { return nil }
func TestImagePickerHasDrawableScreenAndRetainsLifecycleOwner(t *testing.T) {
	ui := widgets.NewUI(&pickerScreenSection{})
	ui.Reset()
	opts := &screens.Options{}
	extendImagePicker(ui, opts, nil)
	p := &testImagePickerView{page: &sharedshell.Page{Title: "Synthetic image picker", Build: func() ([]menu.Row, []func(int)) {
		return []menu.Row{{Label: "Synthetic image file.png", Kind: menu.Plain}}, []func(int){nil}
	}}}
	screen := opts.Views[0](p)
	if screen == nil || screen.View != p || screen.Title != p.page.Title || screen.Build == nil {
		t.Fatal("picker would fall through to clock", screen)
	}
	w := gogui.NewWindow(gogui.WindowCfg{State: new(int), Width: 1280, Height: 800})
	root := w.TestRender(screen.Build)
	texts := avatarCaptionTexts(root)
	found := false
	for _, text := range texts {
		if text == "Synthetic image file.png" {
			found = true
		}
	}
	if !found {
		t.Fatal("picker rows not drawn", texts)
	}
	if opts.Views[0](&testImagePickerView{}) != nil {
		t.Fatal("nil page accepted")
	}
}
