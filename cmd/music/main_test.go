package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

type controlFake struct {
	notifications chan error
	onStop        func()
	stopErr       error
}

func (f *controlFake) Errors() <-chan error { return f.notifications }
func (f *controlFake) Stop() error {
	if f.onStop != nil {
		f.onStop()
	}
	return f.stopErr
}
func (f *controlFake) Next() error     { return nil }
func (f *controlFake) Previous() error { return nil }
func (f *controlFake) Pause() error    { return nil }
func (f *controlFake) Current() string { return "" }

func TestControlFinalDrainAfterStop(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input chan string
	}{
		{"quit", func() chan string { c := make(chan string, 1); c <- "q"; return c }()},
		{"eof", func() chan string { c := make(chan string); close(c); return c }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &controlFake{notifications: make(chan error, 1)}
			f.onStop = func() { f.notifications <- errors.New("late failure") }
			var stderr, stdout bytes.Buffer
			runControls(f, tc.input, &stderr, &stdout)
			if !strings.Contains(stderr.String(), "playback: late failure") {
				t.Fatalf("lost stop-boundary error: %q", stderr.String())
			}
		})
	}
}

func TestControlReportsSynchronousStopErrorOnExit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input chan string
	}{
		{"quit", func() chan string { c := make(chan string, 1); c <- "q"; return c }()},
		{"eof", func() chan string { c := make(chan string); close(c); return c }()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &controlFake{notifications: make(chan error, 1), stopErr: errors.New("release failed")}
			var stderr, stdout bytes.Buffer
			runControls(f, tc.input, &stderr, &stdout)
			if !strings.Contains(stderr.String(), "release failed") {
				t.Fatalf("lost synchronous stop error: %q", stderr.String())
			}
		})
	}
}

func TestControlReportsErrorWhileIdle(t *testing.T) {
	f := &controlFake{notifications: make(chan error, 1)}
	inputs := make(chan string)
	f.notifications <- errors.New("idle failure")
	var stderr, stdout bytes.Buffer
	done := make(chan struct{})
	go func() { runControls(f, inputs, &stderr, &stdout); close(done) }()
	// A following quit synchronizes completion without relying on timing.
	inputs <- "q"
	<-done
	if !strings.Contains(stderr.String(), "playback: idle failure") {
		t.Fatalf("lost idle error: %q", stderr.String())
	}
}
