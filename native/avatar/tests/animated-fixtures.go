//go:build ignore

// Authored deterministic visual-only loops for device alpha/timing checks.
package main

import (
	"fmt"
	"github.com/gen2brain/webp"
	"github.com/ygelfand/echolocal/internal/avatar"
	"image"
	"image/color"
	"io"
	"os"
	"path/filepath"
)

func main() {
	out := os.Args[1]
	if e := os.MkdirAll(out, 0700); e != nil {
		panic(e)
	}
	for _, name := range []string{"background", "front", "clear", "still", "finite", "large"} {
		var frames []image.Image
		var delays []int
		w, h := 320, 200
		n := 12
		if name == "large" {
			w, h, n = 640, 400, 24
		}
		if name == "still" {
			n = 1
		}
		for i := 0; i < n; i++ {
			img := image.NewNRGBA(image.Rect(0, 0, w, h))
			for y := 0; y < h; y++ {
				for x := 0; x < w; x++ {
					p := color.NRGBA{uint8(24 + i*4), 120, uint8(180 - i*4), 255}
					if name == "front" || name == "clear" {
						p = color.NRGBA{}
						if name == "front" {
							if y >= 100 && y < 150 {
								p = color.NRGBA{uint8(240 - i*8), 24, uint8(48 + i*8), 128}
							}
							if y >= 150 {
								p = color.NRGBA{96, uint8(48 + i*4), 24, 255}
							}
						}
					}
					img.SetNRGBA(x, y, p)
				}
			}
			frames = append(frames, img)
			delays = append(delays, 80+i%3*40)
		}
		path := filepath.Join(out, name+".webp")
		f, e := os.Create(path)
		if e != nil {
			panic(e)
		}
		e = webp.EncodeAll(f, &webp.WEBP{Image: frames, Delay: delays, LoopCount: func() int {
			if name == "finite" {
				return 1
			}
			return 0
		}()}, webp.Options{Lossless: true, Exact: true})
		f.Close()
		if e != nil {
			panic(e)
		}
		p, close, e := avatar.ResolveBackground("/", out, path)
		if e != nil {
			panic(e)
		}
		data, e := os.Open(p)
		if e != nil {
			panic(e)
		}
		dest, e := os.Create(filepath.Join(out, name+".rgba"))
		if e != nil {
			panic(e)
		}
		_, e = io.Copy(dest, data)
		data.Close()
		dest.Close()
		close()
		if e != nil {
			panic(e)
		}
		fmt.Println("FIXTURE", name, n)
	}
}
