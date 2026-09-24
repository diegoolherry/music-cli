package playback

import (
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/ebitengine/oto/v3"
	mp3 "github.com/hajimehoshi/go-mp3"
)

// Audio owns a single Oto context. A different MP3 sample rate is rejected,
// rather than played at the wrong speed; Oto permits only one context per process.
type Audio struct {
	mu      sync.Mutex
	context *oto.Context
	rate    int
	active  *audioStream
}
type audioStream struct {
	file   *os.File
	player *oto.Player
	done   chan struct{}
	once   sync.Once
	mu     sync.Mutex
	paused bool
}

func (s *audioStream) Close() error {
	var err error
	s.once.Do(func() {
		s.mu.Lock()
		close(s.done)
		s.player.PauseAndStopReading()
		s.mu.Unlock()
		err = s.file.Close()
	})
	return err
}

// completed observes the player and pause state under the same lock as Pause.
func (s *audioStream) completed(isPlaying func() bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return false
	default:
	}
	return !s.paused && !isPlaying()
}
func (a *Audio) Open(path string, ended func(error)) (Stream, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	d, err := mp3.NewDecoder(f)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	a.mu.Lock()
	if a.context != nil && a.rate != d.SampleRate() {
		a.mu.Unlock()
		f.Close()
		return nil, fmt.Errorf("MP3 sample rate %d differs from device context %d; restart player for this track", d.SampleRate(), a.rate)
	}
	if a.context == nil {
		ctx, ready, err := oto.NewContext(&oto.NewContextOptions{SampleRate: d.SampleRate(), ChannelCount: 2, Format: oto.FormatSignedInt16LE})
		if err != nil {
			a.mu.Unlock()
			f.Close()
			return nil, err
		}
		<-ready
		if err = ctx.Err(); err != nil {
			a.mu.Unlock()
			f.Close()
			return nil, err
		}
		a.context = ctx
		a.rate = d.SampleRate()
	}
	s := &audioStream{file: f, player: a.context.NewPlayer(d), done: make(chan struct{})}
	a.active = s
	a.mu.Unlock()
	s.player.Play()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-s.done:
				return
			case <-ticker.C:
				if s.completed(s.player.IsPlaying) {
					ended(s.player.Err())
					return
				}
			}
		}
	}()
	return s, nil
}
func (a *Audio) Pause(paused bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.active == nil {
		return nil
	}
	s := a.active
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.done:
		return nil
	default:
	}
	s.paused = paused
	if paused {
		s.player.Pause()
	} else {
		s.player.Play()
	}
	return s.player.Err()
}
