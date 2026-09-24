package playback

import (
	"testing"
)

func TestCompletionDecisionSerializedWithPauseAndStop(t *testing.T) {
	s := &audioStream{done: make(chan struct{})}
	s.mu.Lock()
	finished := make(chan bool, 1)
	go func() { finished <- s.completed(func() bool { return false }) }()
	s.paused = true
	s.mu.Unlock()
	if <-finished {
		t.Fatal("pause must suppress natural completion")
	}
	close(s.done)
	if s.completed(func() bool { return false }) {
		t.Fatal("stop must suppress completion")
	}
}
