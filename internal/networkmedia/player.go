package networkmedia

import (
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/ygelfand/libcountertop/pkg/media/cast"
	"github.com/ygelfand/libcountertop/pkg/media/localvideo"
	"github.com/ygelfand/libcountertop/pkg/media/smburl"
	"github.com/ygelfand/libcountertop/pkg/media/urlplayer"
)

type Player struct {
	localvideo.Player
	store     *Store
	transport *Transport
	mu        sync.Mutex
	network   bool
}

func Wrap(p localvideo.Player, s *Store, t *Transport) *Player {
	return &Player{Player: p, store: s, transport: t}
}
func (p *Player) Load(m cast.Media, at time.Duration, play bool) error {
	raw := m.URL()
	u, _ := url.Parse(raw)
	network := strings.HasPrefix(raw, "smb:")
	if network {
		internal, e := smburl.Internal(raw)
		if e != nil {
			return e
		}
		c, e := p.store.snapshot()
		if e != nil {
			return e
		}
		if !c.Enabled {
			return publicError(playbackDiagnostic("disabled", "config", "SOURCE_DISABLED"))
		}
		m.ContentID = internal
		m.ContentURL = ""
		m.Metadata = cast.Metadata{Title: "Network video"}
	} else if u != nil && u.Hostname() == smburl.Host {
		return smburl.ErrURL
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.transport.ClearError()
	e := p.Player.Load(m, at, play)
	if e == nil {
		p.network = network
	}
	return e
}
func (p *Player) Snapshot() urlplayer.Snapshot {
	p.mu.Lock()
	network := p.network
	p.mu.Unlock()
	s := p.Player.Snapshot()
	if network && s.State == "error" {
		if e := p.transport.Error(); e != "" {
			s.Error = e
		} else {
			s.Error = playbackDiagnostic("decode", "decode", "VIDEO_DECODE_FAILED")
		}
	}
	return s
}

type publicError string

func (e publicError) Error() string         { return string(e) }
func (e publicError) PlaybackError() string { return string(e) }

func (p *Player) Active() bool {
	p.mu.Lock()
	network := p.network
	p.mu.Unlock()
	if !network {
		return false
	}
	s := p.Player.Snapshot()
	return s.State != "idle" && s.State != "error"
}
