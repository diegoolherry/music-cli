// Command music is a bounded audio probe; it does not browse or modify music files.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diegoolherry/music-cli/internal/playback"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: music <mp3 path> [mp3 path ...]")
		os.Exit(2)
	}
	paths := os.Args[1:]
	folder := filepath.Dir(paths[0])
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		if filepath.Dir(path) != folder {
			fmt.Fprintln(os.Stderr, "all paths must be in one folder")
			os.Exit(2)
		}
		names = append(names, filepath.Base(path))
	}
	engine := playback.New(&playback.Audio{})
	if err := engine.Play(folder, names, names[0]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("Audio probe: n next, p previous, space/Enter pause or resume, s stop, q quit. Natural end advances in folder.")
	inputs := make(chan string)
	go func() {
		defer close(inputs)
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			inputs <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintln(os.Stderr, "input:", err)
		}
	}()
	runControls(engine, inputs, os.Stderr, os.Stdout)
}

type controlEngine interface {
	Errors() <-chan error
	Stop() error
	Next() error
	Previous() error
	Pause() error
	Current() string
}

func runControls(engine controlEngine, inputs <-chan string, stderr, stdout io.Writer) {
	showPending := func() {
		for {
			select {
			case err := <-engine.Errors():
				fmt.Fprintln(stderr, "playback:", err)
			default:
				return
			}
		}
	}
	for {
		var line string
		select {
		case err := <-engine.Errors():
			fmt.Fprintln(stderr, "playback:", err)
			continue
		case value, ok := <-inputs:
			if !ok {
				showPending()
				if err := engine.Stop(); err != nil {
					fmt.Fprintln(stderr, err)
				}
				showPending()
				return
			}
			line = value
		}
		showPending()
		key := strings.TrimSpace(line)
		var err error
		switch key {
		case "n":
			err = engine.Next()
		case "p":
			err = engine.Previous()
		case "":
			err = engine.Pause()
		case "s":
			showPending()
			err = engine.Stop()
		case "q":
			showPending()
			if err := engine.Stop(); err != nil {
				fmt.Fprintln(stderr, err)
			}
			showPending()
			return
		default:
			fmt.Fprintln(stdout, "unknown command")
			continue
		}
		if err != nil {
			fmt.Fprintln(stderr, err)
		}
		showPending()
		fmt.Fprintln(stdout, "current:", engine.Current())
	}
}
