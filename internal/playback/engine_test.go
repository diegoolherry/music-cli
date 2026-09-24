package playback

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"testing"
)

type fakeStream struct{ closed bool }

func (s *fakeStream) Close() error { s.closed = true; return nil }

type fakeBackend struct {
	streams []*fakeStream
	fail    string
	paused  bool
	ended   func(error)
}

func (b *fakeBackend) Open(path string, ended func(error)) (Stream, error) {
	if path == b.fail {
		return nil, errors.New("open failed")
	}
	s := &fakeStream{}
	b.streams = append(b.streams, s)
	b.ended = ended
	return s, nil
}
func (b *fakeBackend) Pause(p bool) error { b.paused = p; return nil }
func (b *fakeBackend) finish()            { b.ended(nil) }
func (b *fakeBackend) failRead(err error) { b.ended(err) }

var _ io.Closer = (*fakeStream)(nil)

func TestPlaybackTransitions(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	files := []string{"10.mp3", "02.mp3", "01.mp3"}
	if err := e.Play("rock", files, "01.mp3"); err != nil {
		t.Fatal(err)
	}
	if got := e.Tracks(); !reflect.DeepEqual(got, []string{"01.mp3", "02.mp3", "10.mp3"}) {
		t.Fatalf("order: %v", got)
	}
	if err := e.Previous(); err != nil {
		t.Fatal(err)
	}
	if e.Current() != "01.mp3" {
		t.Fatal("previous wrapped")
	}
	if err := e.Pause(); err != nil || !b.paused || b.streams[0].closed {
		t.Fatal("pause must retain stream", err)
	}
	if err := e.Pause(); err != nil || b.paused {
		t.Fatal("resume", err)
	}
	b.finish()
	if e.Current() != "02.mp3" || !b.streams[0].closed {
		t.Fatal("natural next did not replace file")
	}
	if err := e.Next(); err != nil {
		t.Fatal(err)
	}
	b.finish()
	if e.Current() != "" || !b.streams[2].closed {
		t.Fatal("last track should stop")
	}
	if err := e.Play("rock", files, "01.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil || !b.streams[3].closed || e.Current() != "" {
		t.Fatal("stop must release file", err)
	}
}
func TestActivePathLifecycle(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	folder := filepath.Join(t.TempDir(), "Rock")
	check := func(want string) {
		t.Helper()
		if got := e.ActivePath(); got != want {
			t.Fatalf("ActivePath() = %q, want %q", got, want)
		}
	}
	check("")
	if err := e.Play(folder, []string{"a.mp3", "b.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	check(filepath.Join(folder, "a.mp3"))
	if err := e.Pause(); err != nil {
		t.Fatal(err)
	}
	check(filepath.Join(folder, "a.mp3"))
	if err := e.Next(); err != nil {
		t.Fatal(err)
	}
	check(filepath.Join(folder, "b.mp3"))
	b.finish()
	check("")
	if err := e.Play(folder, []string{"a.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	b.failRead(errors.New("read failed"))
	check("")
	if err := e.Play(folder, []string{"a.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	check("")
}

func TestCompletionQueuedBeforePauseCannotAdvance(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	if err := e.Play("rock", []string{"a.mp3", "b.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	callback := b.ended
	if err := e.Pause(); err != nil {
		t.Fatal(err)
	}
	callback(nil)
	if e.Current() != "a.mp3" || len(b.streams) != 1 {
		t.Fatal("paused completion advanced queue")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	callback(nil)
	if len(b.streams) != 1 {
		t.Fatal("completion after stop advanced queue")
	}
}
func TestStaleCompletionCannotAdvanceReplacement(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	if err := e.Play("first", []string{"a.mp3", "b.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	stale := b.ended
	if err := e.Play("second", []string{"c.mp3", "d.mp3"}, "c.mp3"); err != nil {
		t.Fatal(err)
	}
	stale(nil)
	if e.Current() != "c.mp3" || len(b.streams) != 2 || b.streams[1].closed {
		t.Fatal("old completion changed new playback")
	}
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	b.ended(nil)
	if e.Current() != "" || len(b.streams) != 2 {
		t.Fatal("completion after stop restarted playback")
	}
}
func TestFailedAutoNextClearsCurrentAndRecordsError(t *testing.T) {
	b := &fakeBackend{fail: filepath.Join("rock", "b.mp3")}
	e := New(b)
	if err := e.Play("rock", []string{"a.mp3", "b.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	b.finish()
	if e.Current() != "" || e.Err() == nil || !b.streams[0].closed {
		t.Fatal("failed next must close previous file and report failure")
	}
	select {
	case err := <-e.Errors():
		if err == nil {
			t.Fatal("nil auto-next failure")
		}
	default:
		t.Fatal("missing auto-next notification")
	}
}
func TestAsyncReadFailureStopsAndReports(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	if err := e.Play("rock", []string{"a.mp3", "b.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	b.failRead(errors.New("read failed"))
	if e.Current() != "" || e.Err() == nil || len(b.streams) != 1 || !b.streams[0].closed {
		t.Fatal("read failure must stop without auto-next")
	}
}
func TestAsyncFailureNotificationSurvivesStop(t *testing.T) {
	b := &fakeBackend{}
	e := New(b)
	if err := e.Play("rock", []string{"a.mp3"}, "a.mp3"); err != nil {
		t.Fatal(err)
	}
	b.failRead(errors.New("read failed"))
	if err := e.Stop(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-e.Errors():
		if err == nil || err.Error() != "read failed" {
			t.Fatalf("notification: %v", err)
		}
	default:
		t.Fatal("missing asynchronous failure notification")
	}
}
func TestFailureNotificationsAreBounded(t *testing.T) {
	e := New(&fakeBackend{})
	for i := 0; i < 20; i++ {
		e.notify(errors.New("failure"))
	}
	if len(e.Errors()) != 16 {
		t.Fatalf("buffered notifications: %d", len(e.Errors()))
	}
}
func TestStopClearsPreviousFailure(t *testing.T) {
	b := &fakeBackend{fail: filepath.Join("rock", "bad.mp3")}
	e := New(b)
	if err := e.Play("rock", []string{"bad.mp3"}, "bad.mp3"); err == nil {
		t.Fatal("expected failure")
	}
	if err := e.Stop(); err != nil || e.Err() != nil {
		t.Fatal("stop must clear stale playback error", err)
	}
}
func TestFailedPlayDoesNotClaimSuccess(t *testing.T) {
	b := &fakeBackend{fail: filepath.Join("rock", "02.mp3")}
	e := New(b)
	if err := e.Play("rock", []string{"02.mp3"}, "02.mp3"); err == nil || e.Current() != "" {
		t.Fatal("failed open reported playing")
	}
}
