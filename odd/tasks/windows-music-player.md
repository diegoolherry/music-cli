# Windows music player implementation

Objective: Implement the approved Windows-first local MP3 TUI incrementally, beginning with a small playback compatibility slice rather than claiming a finished v1.0.

## Problem and scope

The approved behavior lives in `desing/requirements.md`; the magenta OpenPencil mockup is visual guidance. Target Windows 10 22H2 on this host first; Windows 11 remains unverified. `D:\Music` is a fixed root; no installer, song deletion, configurable root, or recursive library. Use Go with standard Go tests; Bubble Tea v2 and Oto v3 are candidates to pin at implementation. `go-mp3` is unmaintained upstream, so record its risk and validate decoding on an actual MP3 before claiming compatibility. Keep source, tests, and user-facing instructions in each work unit. Do not push or open a PR without the user's decision.

## Method and delivery

- TDD: strict RED → GREEN → REFACTOR, explicitly selected by the user in this session. Runner: `go test ./...` (Go standard testing, no project runner yet); build: `go build ./...`. Record exact observed output for each task.
- Feature branch: `feat/windows-music-player`, branched from `main` at `f8d0e94`. Close each verified task with a Conventional Commit work unit; record its identity here.
- Delivery strategy: ask-on-risk; user selected stacked/linked PRs (each PR based on the previous one). Forecast: full feature exceeds ~400 authored changed lines across multiple work units. Track authored additions + deletions from commits, excluding generated assets; split coherent units, not tests from behavior, and do not shrink code to fit. No PR created.
- Native RDD is on (global); review candidate is a commit or PR slice, not a task checkbox. Follow native risk assessment and the offered route only.

## Tasks

- [ ] I1: Prove a bounded MP3 playback engine on Windows: open/decode, play/pause/stop-and-close, natural next in active folder, fixed alphabetical ordering; isolate device so tests do not need speakers. Check: observed RED before GREEN; `go test ./...`, `go build ./...`; manual audio check only with an available MP3 and output device. No claim of audible playback until actually heard. Suggested surfaces: `go.mod`, `go.sum`, `internal/playback/`, `cmd/music/`, `README.md`.
- [ ] I2: Discover immediate folders and MP3 files under `D:\Music`, sort deterministically, report empty/unreadable cases without false success; keep playback queue independent from browsing. Check: Go tests with temporary folders and `go test ./...`.
- [ ] I3: Implement two-pane terminal UI using approved keyboard map and magenta visual direction; wire browsing and playback status. Check: model/update tests, build, interactive CMD and PowerShell smoke check.
- [ ] I4: Add safe folder create/rename/recycle and MP3 base-name rename with in-use guard; never permanently delete on recycle failure. Check: filesystem tests (mock recycle boundary), Windows integration test with disposable files, build.
- [ ] I5: Verify v1 scenarios, Windows terminal/runtime behavior, and portable `music.exe` usage; update documentation and remaining OD4 evidence. Check: full Go tests/build plus manual playback and CMD/PowerShell acceptance on Windows; record untested platforms honestly.

## Progress

- I1 in progress: worker added pinned Go module, testable playback engine, explicit-path probe, README and tests; observed RED for stale error and asynchronous read failure, then `go test ./...`, `go build ./...`, `git diff --check` green. A user-supplied MP3 now exists directly inside `D:\Music`; a bounded explicit-path probe opened and stopped it with exit code 0, without modifying it. Audible playback is not yet human-confirmed.
- Independent verification: `go test ./...`, `go build ./...`, `go vet ./...` passed. `go test -race ./internal/playback` could not run with CGO disabled. Verifier found a pause/completion interleaving and asynchronous-error visibility defect; fix and reverify before I1 can close. Native read-only assess returned unassessable (empty native output), so independent verification was required.
- Correction: observed new RED before fixes to pause/completion synchronization and asynchronous error notifications; worker reports `go test ./...`, `go build ./...`, `go vet ./...` green, and a subsequent formatting-only pass reported `gofmt -l` empty. Parent independently reran `go test ./internal/playback -count=1` and `go build ./...` successfully. Human audible check remains pending.
- Work-unit commit `0d6dc7a` (`feat(playback): define ordered queue and lifecycle`) contains the 387-line pure playback state machine, its tests, and pinned module; rollback boundary is those four files. Native committed-range assess returned unassessable (empty output); inspect/start for that committed range was blocked at an empty workspace/base-ref selection before lineage creation, so no native review receipt is claimed. Independent verification and Go tests remain the fallback evidence; do not retry START blindly.
- User chose stacked PRs; the first coherent core unit has 387 authored lines. Next coherent slice is Oto adapter + runnable probe + documentation, expected <400 authored lines; no PR created or remote push.
- Next: commit the adapter/probe work unit after focused checks, then ask the human to confirm audible playback. No claim of complete Windows audio compatibility yet.
