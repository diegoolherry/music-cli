// Command music browses the fixed Windows music library or runs an explicit audio probe.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/diegoolherry/music-cli/internal/library"
	"github.com/diegoolherry/music-cli/internal/playback"
	"github.com/diegoolherry/music-cli/internal/ui"
)

func main() {
	if err := run(os.Args[1:], launchUI, probe); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, launch func(string) error, probePaths func([]string) error) error {
	if len(args) == 0 {
		return launch(`D:\Music`)
	}
	if args[0] != "--probe" || len(args) < 2 {
		return fmt.Errorf("usage: music [--probe <mp3 path> [mp3 path ...]]")
	}
	return probePaths(args[1:])
}

func launchUI(root string) error {
	snapshot, scanErr := library.Scan(root)
	engine := playback.New(&playback.Audio{})
	result, runErr := tea.NewProgram(ui.New(root, snapshot, scanErr, engine)).Run()
	if stopErr := engine.Stop(); stopErr != nil {
		return fmt.Errorf("release playback: %w", stopErr)
	}
	if runErr != nil {
		return runErr
	}
	if model, ok := result.(ui.Model); ok {
		return model.QuitError
	}
	return nil
}

func probe(paths []string) error {
	folder := filepath.Dir(paths[0])
	names := make([]string, 0, len(paths))
	for _, path := range paths {
		if filepath.Dir(path) != folder {
			return fmt.Errorf("all paths must be in one folder")
		}
		names = append(names, filepath.Base(path))
	}
	engine := playback.New(&playback.Audio{})
	if err := engine.Play(folder, names, names[0]); err != nil {
		return err
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
	return nil
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
