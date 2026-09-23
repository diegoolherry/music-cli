package playback

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// Backend opens a playing stream and invokes ended only for natural completion.
type Backend interface {
	Open(path string, ended func(error)) (Stream, error)
	Pause(bool) error
}
type Stream interface{ io.Closer }

type Engine struct {
	mu         sync.Mutex
	backend    Backend
	folder     string
	tracks     []string
	index      int
	stream     Stream
	paused     bool
	generation uint64
	lastError  error
	errors     chan error
}

func New(b Backend) *Engine { return &Engine{backend: b, errors: make(chan error, 16)} }

// Errors reports asynchronous failures without blocking the playback monitor.
// When full, the oldest unread failure is replaced.
func (e *Engine) Errors() <-chan error { return e.errors }
func (e *Engine) notify(err error) {
	if err == nil {
		return
	}
	select {
	case e.errors <- err:
	default:
		<-e.errors
		e.errors <- err
	}
}
func Order(files []string) []string {
	out := append([]string(nil), files...)
	slices.SortFunc(out, func(a, b string) int {
		x, y := strings.ToLower(a), strings.ToLower(b)
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
		return strings.Compare(a, b)
	})
	return out
}
func (e *Engine) Tracks() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.tracks...)
}
func (e *Engine) Current() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stream == nil {
		return ""
	}
	return e.tracks[e.index]
}

// ActivePath returns the path of the currently open stream, including while paused.
func (e *Engine) ActivePath() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stream == nil {
		return ""
	}
	return filepath.Join(e.folder, e.tracks[e.index])
}
func (e *Engine) Err() error { e.mu.Lock(); defer e.mu.Unlock(); return e.lastError }
func (e *Engine) stop() error {
	e.generation++
	if e.stream == nil {
		return nil
	}
	err := e.stream.Close()
	e.stream = nil
	e.paused = false
	return err
}
func (e *Engine) Stop() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	err := e.stop()
	e.lastError = err
	return err
}
func (e *Engine) start(i int) error {
	if err := e.stop(); err != nil {
		e.lastError = err
		return err
	}
	gen := e.generation
	s, err := e.backend.Open(filepath.Join(e.folder, e.tracks[i]), func(err error) { e.finished(gen, err) })
	if err != nil {
		e.lastError = err
		return err
	}
	e.stream = s
	e.index = i
	e.lastError = nil
	return nil
}
func (e *Engine) Play(folder string, files []string, selected string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	ordered := Order(files)
	i := slices.Index(ordered, selected)
	if i < 0 {
		return fmt.Errorf("track %q not in folder", selected)
	}
	e.folder = folder
	e.tracks = ordered
	return e.start(i)
}
func (e *Engine) move(delta int) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stream == nil || e.index+delta < 0 || e.index+delta >= len(e.tracks) {
		return nil
	}
	return e.start(e.index + delta)
}
func (e *Engine) Next() error     { return e.move(1) }
func (e *Engine) Previous() error { return e.move(-1) }
func (e *Engine) Pause() error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.stream == nil {
		return nil
	}
	if err := e.backend.Pause(!e.paused); err != nil {
		e.lastError = err
		return err
	}
	e.paused = !e.paused
	return nil
}
func (e *Engine) finished(gen uint64, playbackErr error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if gen != e.generation || e.stream == nil || (e.paused && playbackErr == nil) {
		return
	}
	if playbackErr != nil {
		closeErr := e.stop()
		e.lastError = playbackErr
		if closeErr != nil {
			e.lastError = fmt.Errorf("playback: %w; close: %v", playbackErr, closeErr)
		}
		e.notify(e.lastError)
		return
	}
	if e.index+1 == len(e.tracks) {
		e.lastError = e.stop()
		return
	}
	if err := e.start(e.index + 1); err != nil {
		e.notify(err)
	}
}
