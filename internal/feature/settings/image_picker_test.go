//go:build !board_biscuit

package settings

import (
	"context"
	"errors"
	"github.com/ygelfand/echolocal/internal/avatar"
	"github.com/ygelfand/echolocal/internal/devicefs"
	featureshell "github.com/ygelfand/echolocal/internal/feature/shell"
	sharedshell "github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
	"strings"
	"testing"
	"time"
)

func waitImagePicker(t *testing.T, p *imagePickerView) imagePickerState {
	t.Helper()
	end := time.Now().Add(3 * time.Second)
	for time.Now().Before(end) {
		p.mu.Lock()
		s := p.state
		p.mu.Unlock()
		if !s.loading {
			return s
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("directory reader did not settle")
	return imagePickerState{}
}
func TestImagePickerFilteringNavigationSelectionAndCancel(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de", "it", "vi"} {
		say.Use(lang)
		chosen := ""
		reject := true
		p := &imagePickerView{redraw: func() {}, choose: func(name string) error {
			if reject {
				return errors.New(avatar.Text(lang, "background_invalid"))
			}
			chosen = name
			return nil
		}, list: func(_ context.Context, name string, offset int) (devicefs.Listing, error) {
			next := 256
			return devicefs.Listing{Path: name, Parent: "/", NextOffset: &next, Entries: []devicefs.Entry{
				{Name: "folder", Kind: "directory"}, {Name: "picture.PNG", Kind: "file", Size: 100},
				{Name: "bad.txt", Kind: "file"}, {Name: "pipe.png", Kind: "special"},
				{Name: "big.jpg", Kind: "file", Size: avatar.MaxBackgroundBytes + 1},
			}}, nil
		}}
		p.Page = &sharedshell.Page{Title: avatar.Text(lang, "picker_title"), Build: p.build}
		if sharedshell.SettingsPage(p) != p.Page {
			t.Fatal("native page wrapper not recognized")
		}
		featureshell.Get().Push(p)
		// Keep page construction independent of a real device or its directories.
		p.navigate("/fixture", 0)
		waitImagePicker(t, p)
		rows, acts := p.build()
		imageIndex := -1
		folderIndex := -1
		bigIndex := -1
		for i, row := range rows {
			if row.Label == "bad.txt" || row.Label == "pipe.png" {
				t.Fatal("unsupported item shown")
			}
			if row.Label == "picture.PNG" {
				imageIndex = i
			}
			if row.Label == "folder/" {
				folderIndex = i
			}
			if row.Label == "big.jpg" {
				bigIndex = i
			}
		}
		if imageIndex < 0 || folderIndex < 0 || bigIndex < 0 {
			t.Fatal(rows)
		}
		acts[bigIndex](0)
		if chosen != "" || !rows[bigIndex].Dim {
			t.Fatal("oversized image selected")
		}
		acts[imageIndex](0)
		p.mu.Lock()
		note := p.state.note
		p.mu.Unlock()
		if note != avatar.Text(lang, "background_invalid") || chosen != "" {
			t.Fatal("failed validation lost prior choice")
		}
		acts[folderIndex](0)
		s := waitImagePicker(t, p)
		if s.name != "/fixture/folder" {
			t.Fatal(s)
		}
		rows, acts = p.build()
		for i, row := range rows {
			if row.Label == avatar.Text(lang, "picker_cancel") {
				acts[i](0)
			}
		}
		if chosen != "" {
			t.Fatal("cancel saved")
		}
		p.Close()
	}
}
func TestImagePickerStopsStaleLoadsAndResumesAfterCover(t *testing.T) {
	began := make(chan struct{})
	canceled := make(chan struct{})
	p := &imagePickerView{redraw: func() {}, list: func(ctx context.Context, name string, _ int) (devicefs.Listing, error) {
		if name == "/slow" {
			close(began)
			<-ctx.Done()
			close(canceled)
			return devicefs.Listing{}, ctx.Err()
		}
		return devicefs.Listing{Path: name}, nil
	}}
	p.navigate("/slow", 0)
	<-began
	p.navigate("/current", 0)
	<-canceled
	s := waitImagePicker(t, p)
	if s.name != "/current" || s.note != "" {
		t.Fatal("stale response replaced new location", s)
	}
	p.Close()
	// Visibility Close must not permanently disable a page covered by Talk.
	_, _ = p.build()
	s = waitImagePicker(t, p)
	if s.name != "/current" {
		t.Fatal(s)
	}
	p.Close()
}
func TestImagePickerTranslationsAndPageProvider(t *testing.T) {
	for _, lang := range []string{"en", "de", "it", "vi"} {
		for _, key := range []string{"choose_image", "picker_title", "picker_cancel", "picker_folder", "picker_uploads", "picker_up", "picker_refresh", "picker_loading", "picker_empty", "picker_previous", "picker_next", "picker_limit", "picker_error", "picker_go"} {
			if strings.TrimSpace(avatar.Text(lang, key)) == "" {
				t.Fatal(lang, key)
			}
		}
	}
}
