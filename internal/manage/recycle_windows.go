//go:build windows

package manage

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
	"github.com/rodrigocfd/windigo/x/cosh"
	"github.com/rodrigocfd/windigo/x/winsh"
)

const (
	// FOFX flags do not fit Windigo's uint16 cosh.FOF SetOperationFlags signature.
	recycleOnDelete uint32 = 0x00080000
	earlyFailure    uint32 = 0x00100000
)

type fileOperationPrefix struct {
	queryInterface, addRef, release     uintptr
	advise, unadvise, setOperationFlags uintptr
}

// setFullOperationFlags bridges only the six-entry COM vtable prefix; the
// upstream wrapper truncates DWORD flags to uint16, losing RECYCLEONDELETE.
func setFullOperationFlags(op *winsh.IFileOperation, flags uint32) error {
	p := op.Ppvt()
	// Reinterpret the COM interface pointer through a pointer-typed local so
	// vet can check the conversion without uintptr-to-pointer arithmetic.
	object := *(*unsafe.Pointer)(unsafe.Pointer(&p))
	vt := (*fileOperationPrefix)(*(*unsafe.Pointer)(object))
	ret, _, _ := syscall.SyscallN(vt.setOperationFlags, p, uintptr(flags))
	if ret != 0 {
		return co.HRESULT(ret)
	}
	return nil
}

func recycleAllowed(flags cosh.TSF) co.HRESULT {
	if flags&cosh.TSF_DELETE_RECYCLE_IF_POSSIBLE == 0 {
		return co.HRESULT_E_ABORT
	}
	return co.HRESULT_S_OK
}

func recycleDirectory(path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("recycle requires absolute path")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_, err := win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	if err != nil {
		return fmt.Errorf("initialize STA: %w", err)
	}
	defer win.CoUninitialize()
	rel := win.NewOleReleaser()
	defer rel.Release()
	var op *winsh.IFileOperation
	if err := win.CoCreateInstance(rel, &cosh.CLSID_FileOperation, nil, co.CLSCTX_ALL, &op); err != nil {
		return fmt.Errorf("create file operation: %w", err)
	}
	flags := recycleOnDelete | earlyFailure | uint32(cosh.FOF_SILENT|cosh.FOF_NOCONFIRMATION|cosh.FOF_NOERRORUI)
	if err := setFullOperationFlags(op, flags); err != nil {
		return fmt.Errorf("set recycle flags: %w", err)
	}
	var item *winsh.IShellItem
	if err := winsh.SHCreateItemFromParsingName(rel, path, &item); err != nil {
		return fmt.Errorf("create shell item: %w", err)
	}
	sink := winsh.NewIFileOperationProgressSinkImpl(rel)
	seen := false
	sink.PreDeleteItem(func(flags cosh.TSF, _ *winsh.IShellItem) co.HRESULT {
		if recycleAllowed(flags) == co.HRESULT_S_OK {
			seen = true
		}
		return recycleAllowed(flags)
	})
	if err := op.DeleteItem(item, sink); err != nil {
		return fmt.Errorf("queue recycle: %w", err)
	}
	if err := op.PerformOperations(); err != nil {
		return fmt.Errorf("perform recycle: %w", err)
	}
	aborted, err := op.GetAnyOperationsAborted()
	if err != nil {
		return fmt.Errorf("check recycle completion: %w", err)
	}
	if aborted || !seen {
		return fmt.Errorf("recycle not confirmed (aborted=%t, pre=%t)", aborted, seen)
	}
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return fmt.Errorf("recycle source not confirmed absent: %v", err)
	}
	return nil
}
