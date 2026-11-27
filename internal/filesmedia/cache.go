package filesmedia

import (
	"context"
	"fmt"
	"image"
	"os"
	"sync"
)

// Cache owns one decoder, a bounded queue, and at most 64 small/negative results.
// It is discarded on folder/visibility/refresh changes, so stale jobs never
// replace the next folder's previews. Nothing is written to media storage.
type Cache struct {
	mu      sync.Mutex
	items   map[string]*image.NRGBA
	pending map[string]bool
	queue   chan job
	ctx     context.Context
	cancel  context.CancelFunc
	changed func()
}
type job struct {
	key, name string
	open      func() (*os.File, error)
	helper    string
}

func NewCache(changed func()) *Cache {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Cache{items: map[string]*image.NRGBA{}, pending: map[string]bool{}, queue: make(chan job, 64), ctx: ctx, cancel: cancel, changed: changed}
	go c.run()
	return c
}
func (c *Cache) Close() { c.cancel() }
func (c *Cache) Get(key, name, helper string, open func() (*os.File, error)) *image.NRGBA {
	c.mu.Lock()
	defer c.mu.Unlock()
	if img, ok := c.items[key]; ok {
		return img
	}
	if c.pending[key] || len(c.items)+len(c.pending) >= 64 || c.ctx.Err() != nil {
		return nil
	}
	select {
	case c.queue <- job{key, name, open, helper}:
		c.pending[key] = true
	default:
	}
	return nil
}

func (c *Cache) run() {
	for {
		select {
		case <-c.ctx.Done():
			return
		case j := <-c.queue:
			if c.ctx.Err() != nil {
				return
			}
			var img *image.NRGBA
			if f, e := j.open(); e == nil {
				switch Kind(j.name) {
				case "image":
					img, _ = ImageContext(c.ctx, f, 192, 192)
				case "video":
					img, _ = Video(c.ctx, f, j.helper)
				}
				f.Close()
			}
			c.mu.Lock()
			delete(c.pending, j.key)
			if c.ctx.Err() == nil {
				c.items[j.key] = img
			}
			c.mu.Unlock()
			if c.ctx.Err() == nil && c.changed != nil {
				c.changed()
			}
		}
	}
}
func Key(name string, size, modified int64) string {
	return fmt.Sprintf("%s:%d:%d", name, size, modified)
}
