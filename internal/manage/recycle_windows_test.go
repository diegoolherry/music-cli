//go:build windows

package manage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/x/cosh"
)

func TestRecycleGuardAndFullWidthFlags(t *testing.T) {
	if recycleAllowed(0) != co.HRESULT_E_ABORT {
		t.Fatal("unsafe delete permitted")
	}
	if recycleAllowed(cosh.TSF_DELETE_RECYCLE_IF_POSSIBLE) != co.HRESULT_S_OK {
		t.Fatal("recycle denied")
	}
	if recycleOnDelete <= 0xffff {
		t.Fatal("full-width bridge no longer needed")
	}
}

func TestRecycleBinDisposable(t *testing.T) {
	if os.Getenv("MUSIC_TEST_RECYCLE_BIN") != "1" {
		t.Skip("set MUSIC_TEST_RECYCLE_BIN=1 for disposable Recycle Bin integration")
	}
	root := t.TempDir()
	dir := filepath.Join(root, "disposable-playlist")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "marker.txt"), []byte("disposable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := recycleDirectory(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(dir); !os.IsNotExist(err) {
		t.Fatalf("source remains or stat failed: %v", err)
	}
}
