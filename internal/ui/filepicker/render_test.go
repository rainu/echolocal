//go:build !board_biscuit

package filepicker

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	gui "github.com/go-gui-org/go-gui/gui"
	"github.com/ygelfand/libcountertop/pkg/display/shell"
	"github.com/ygelfand/libcountertop/pkg/display/widgets"
	"github.com/ygelfand/libcountertop/pkg/say"
	"github.com/ygelfand/libcountertop/pkg/settings/schema"

	browser "github.com/ygelfand/echolocal/internal/filepicker"
)

type screenSettings struct{}

func (screenSettings) Read() schema.Screen               { return schema.Screen{} }
func (screenSettings) Update(func(*schema.Screen)) error { return nil }
func findShape(root *gui.Layout, id string) *gui.Shape {
	if root.Shape != nil && root.Shape.ID == id {
		return root.Shape
	}
	for i := range root.Children {
		if s := findShape(&root.Children[i], id); s != nil {
			return s
		}
	}
	return nil
}
func TestTouchLayoutLongNamesScrollAndPinnedActions(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		say.Use(lang)
		for _, size := range [][2]int{{1280, 800}, {800, 480}, {640, 480}} {
			t.Run(fmt.Sprintf("%s-%dx%d", lang, size[0], size[1]), func(t *testing.T) {
				root := t.TempDir()
				for i := 0; i < 90; i++ {
					requireNoError(
						t,
						os.WriteFile(
							filepath.Join(
								root,
								fmt.Sprintf("%03d-%s.vrm", i, strings.Repeat("long name ", 12)),
							),
							nil,
							0600,
						),
					)
				}
				stack := shell.New()
				defer stack.Close()
				p := New(
					Config{
						Title: "Files",
						Shell: stack,
						Browser: browser.Options{
							Locations:  []browser.Location{{Label: "files", Path: root}},
							Initial:    root,
							Extensions: []string{".vrm"},
						},
					},
				)
				stack.Push(p)
				u := widgets.NewUI(screenSettings{})
				// Exercise the actual shared widgets; bypass hold animation only.
				u.Toolkit.Pressable = func(layout func(gui.ContainerCfg) gui.View, cfg gui.ContainerCfg, click func(gui.EventCtx)) gui.View {
					if click != nil {
						cfg.OnClick = func(e gui.EventCtx) { click(e); e.Consume() }
					}
					return layout(cfg)
				}
				w := gui.NewWindow(gui.WindowCfg{State: new(int), Width: size[0], Height: size[1]})
				w.SetView(func(w *gui.Window) gui.View {
					return u.Toolkit.PageFrame(
						w,
						widgets.FrameOptions{
							Title: p.Title,
							Back:  stack.Pop,
							Build: p.Screen(u).Build,
							Foot:  p.Screen(u).Foot,
						},
					)
				})
				tree := w.TestRender(nil)
				get := func(id string) *gui.Shape {
					ids := w.ResolveID(id)
					if len(ids) != 1 {
						t.Fatalf("%s refs %v", id, ids)
					}
					s := findShape(tree, id)
					if s == nil {
						t.Fatal("missing", id)
					}
					return s
				}
				for _, id := range []string{"action-0", "action-1", "back"} {
					s := get(id)
					if s.X < 0 || s.X+s.Width > float32(size[0])+.5 || s.Y < 0 ||
						s.Y+s.Height > float32(size[1])+.5 ||
						s.Height < 48 {
						t.Fatal("inaccessible touch target", id, s)
					}
				}
				footer := get("action-0").Y
				status := get("picker-filter")
				statusY := status.Y
				if status.Height <= 0 || status.Y+status.Height > footer+.5 ||
					status.X < 0 || status.X+status.Width > float32(size[0])+.5 {
					t.Fatal("filter clipped or overlaps actions", status)
				}
				if p.Status() != text("File types", "Dateitypen")+": .vrm · 1–64 / 90" {
					t.Fatal("wrong pinned page status", p.Status())
				}
				scroller := get("page")
				if !scroller.Scrollable || scroller.Height < 100 ||
					scroller.Y+scroller.Height > statusY+.5 {
					t.Fatal("unusable scroller", scroller)
				}
				rows, _ := p.Build()
				target := ""
				for i, r := range rows {
					if strings.HasPrefix(r.Label, "000-") {
						target = fmt.Sprintf("row-%d", i)
					}
				}
				for tries := 0; tries < 12; tries++ {
					ids := w.ResolveID(target)
					if len(ids) == 1 && findShape(tree, target) != nil {
						break
					}
					if err := w.TestScroll(w.ResolveID("page")[0], 0, -3); err != nil {
						t.Fatal(err)
					}
					tree = w.TestRender(nil)
				}
				s := get(target)
				if s.Height < 48 || s.X < scroller.X || s.X+s.Width > scroller.X+scroller.Width+.5 {
					t.Fatal("long row clipped", s)
				}
				if err := w.TestScroll(w.ResolveID("page")[0], 0, -400); err != nil {
					t.Fatal(err)
				}
				tree = w.TestRender(nil)
				if get("action-0").Y != footer || get("picker-filter").Y != statusY {
					t.Fatal("confirmation or filter scrolled away")
				}
				if w.ScrollVerticalOffset(w.ResolveID("page")[0]) >= 0 {
					t.Fatal("list did not scroll")
				}
				action(t, p, text("Next entries", "Weitere Einträge"))
				tree = w.TestRender(nil)
				// Layout uses float32; tolerate subpixel rounding between differently sized pages.
				if get("picker-filter").Y < statusY-.5 || get("picker-filter").Y > statusY+.5 || !strings.HasSuffix(p.Status(), "65–90 / 90") {
					t.Fatalf("footer changed position or failed to update on pagination: before=%v after=%v status=%q", statusY, get("picker-filter").Y, p.Status())
				}
			})
		}
	}
}

func TestPinnedFooterLongFilterAndLocationChooser(t *testing.T) {
	old := say.Chosen()
	defer say.Use(old)
	for _, lang := range []string{"en", "de"} {
		t.Run(lang, func(t *testing.T) {
			say.Use(lang)
			stack := shell.New()
			defer stack.Close()
			p := New(Config{Title: "Files", Shell: stack, Browser: browser.Options{
				Extensions: []string{".first-long-extension", ".second-long-extension", ".third-long-extension", ".fourth-long-extension", ".fifth-long-extension"},
			}})
			stack.Push(p)
			u := widgets.NewUI(screenSettings{})
			screen := p.Screen(u)
			w := gui.NewWindow(gui.WindowCfg{State: new(int), Width: 640, Height: 480})
			w.SetView(func(w *gui.Window) gui.View {
				return u.Toolkit.PageFrame(w, widgets.FrameOptions{Title: p.Title, Build: screen.Build, Foot: screen.Foot})
			})
			tree := w.TestRender(nil)
			status := findShape(tree, "picker-filter")
			selectButton := findShape(tree, "action-0")
			cancelButton := findShape(tree, "action-1")
			list := findShape(tree, "page")
			if status == nil || selectButton == nil || cancelButton == nil || list == nil {
				t.Fatal("footer missing on location chooser")
			}
			if status.Height < 30 || status.Y+status.Height > selectButton.Y+.5 ||
				list.Height < 100 || list.Y+list.Height > status.Y+.5 {
				t.Fatal("long filter does not wrap clear of list/buttons", status, list)
			}
			for _, b := range []*gui.Shape{selectButton, cancelButton} {
				if b.X < 0 || b.X+b.Width > 640.5 || b.Y+b.Height > 480.5 || b.Height < 48 {
					t.Fatal("long filter displaced touch targets", b)
				}
			}
		})
	}
}
