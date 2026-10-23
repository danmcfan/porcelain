package probe

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/danmcfan/porcelain/internal/ffmpeg"
)

// Media implements timeline.Media by probing files with ffprobe, caching
// results per path.
type Media struct {
	Ctx   context.Context
	mu    sync.Mutex
	cache map[string]Source
}

// NewMedia returns a Media bound to ctx.
func NewMedia(ctx context.Context) *Media {
	return &Media{Ctx: ctx, cache: map[string]Source{}}
}

// Source probes path (without hashing) and caches the result.
func (m *Media) Source(path string) (Source, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.cache[path]; ok {
		return s, nil
	}
	r, err := ffmpeg.Probe(m.Ctx, path)
	if err != nil {
		return Source{}, err
	}
	s := Source{Path: path, Kind: KindOther}
	fromProbe(&s, r)
	if s.Kind == KindOther {
		return Source{}, errors.New("no audio or video streams")
	}
	m.cache[path] = s
	return s, nil
}

// Duration implements timeline.Media.
func (m *Media) Duration(path string) (float64, bool, error) {
	s, err := m.Source(path)
	if err != nil {
		return 0, false, fmt.Errorf("probe %s: %w", path, err)
	}
	return s.Duration, s.Kind == KindStill, nil
}
