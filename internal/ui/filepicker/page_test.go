//go:build !board_biscuit

package filepicker

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	browser "github.com/ygelfand/echolocal/internal/filepicker"
	"github.com/ygelfand/libcountertop/pkg/display/menu"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/say"
)

func action(t *testing.T, p *shell.Page, label string) {
	t.Helper()
	rows, acts := p.Build()
	for i, r := range rows {
		if r.Label == label {
			if acts[i] == nil {
				t.Fatal("no action", label)
			}
			acts[i](0)
			return
		}
	}
	t.Fatal("missing row", label)
}
func TestNavigationExplicitConfirmCancelAndFailure(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		root := t.TempDir()
		os.Mkdir(filepath.Join(root, "folder"), 0700)
		file := filepath.Join(root, "folder", "model.vrm")
		os.WriteFile(file, []byte("fixture"), 0600)
		stack := shell.New()
		origin := &shell.Page{Title: "origin"}
		stack.Push(origin)
		value := "previous"
		fail := false
		makePage := func() *shell.Page {
			return New(Config{Title: "model", Shell: stack, Browser: browser.Options{Locations: []browser.Location{{Label: "files", Path: root}}, Extensions: []string{".vrm"}, Initial: root}, Choose: func(s string) error {
				if fail {
					return errors.New("validation failed")
				}
				value = s
				return nil
			}})
		}
		p := makePage()
		if rows, _ := p.Build(); len(rows) != 0 {
			t.Fatal("search triggered IO")
		}
		stack.Push(p)
		action(t, p, "folder")
		action(t, p, "model.vrm")
		if value != "previous" {
			t.Fatal("selection saved without confirmation")
		}
		p.Actions[1].Do()
		if stack.Top() != origin || value != "previous" {
			t.Fatal("cancel modified origin")
		}
		p = makePage()
		stack.Push(p)
		action(t, p, "folder")
		action(t, p, text("Parent folder", "Übergeordneter Ordner"))
		action(t, p, "folder")
		action(t, p, "model.vrm")
		fail = true
		p.Actions[0].Do()
		if stack.Top() != p || value != "previous" {
			t.Fatal("validation failure saved/closed")
		}
		fail = false
		p.Actions[0].Do()
		if value != file || stack.Top() != origin {
			t.Fatal("confirmation did not save/return")
		}
		// Shell back also cancels rather than committing a draft.
		p = makePage()
		stack.Push(p)
		action(t, p, "folder")
		action(t, p, "model.vrm")
		stack.Pop()
		if stack.Top() != origin || value != file {
			t.Fatal("back changed selection")
		}
		stack.Close()
	}
}
func TestManyEntriesPagingAndWrappingPaths(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	say.Use("en")
	root := t.TempDir()
	for i := 0; i < 150; i++ {
		os.WriteFile(filepath.Join(root, fmt.Sprintf("file-%03d.vrm", i)), nil, 0600)
	}
	stack := shell.New()
	p := New(Config{Shell: stack, Browser: browser.Options{Locations: []browser.Location{{Label: "files", Path: root}}, Initial: root}})
	stack.Push(p)
	action(t, p, "Next entries")
	action(t, p, "file-064.vrm")
	rows, _ := p.Build()
	count := 0
	for _, r := range rows {
		if strings.HasPrefix(r.Label, "file-") {
			count++
		}
		if r.Label == "Selection" && r.Hint != filepath.Join(root, "file-064.vrm") {
			t.Fatal(r)
		}
	}
	if count != browser.PageSize {
		t.Fatal("unbounded page", count)
	}
	action(t, p, "Previous entries")
	action(t, p, "file-000.vrm")
	row := Row("File", strings.Repeat("long path ", 40), Config{Shell: stack})
	if row.Kind != menu.Chevron || row.Hint == "" || row.Value != "" || row.Open == nil {
		t.Fatal("path must wrap")
	}
	stack.Close()
}
func TestDirectoryConfirmationAndUnavailableLocation(t *testing.T) {
	root := t.TempDir()
	stack := shell.New()
	selected := ""
	p := New(Config{Shell: stack, Browser: browser.Options{Locations: []browser.Location{{Label: "files", Path: root}, {Label: "missing", Path: filepath.Join(root, "missing")}}, Directory: true}, Choose: func(v string) error { selected = v; return nil }})
	stack.Push(p)
	action(t, p, "missing")
	if rows, _ := p.Build(); len(rows) < 3 {
		t.Fatal("no safe unavailable state")
	}
	action(t, p, "files")
	p.Actions[0].Do()
	if selected != root {
		t.Fatal("directory not selected", selected)
	}
	stack.Close()
}
