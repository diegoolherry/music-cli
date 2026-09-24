//go:build windows

package manage

import "syscall"

// MoveFile does not set MOVEFILE_REPLACE_EXISTING, so an occupied target fails.
func renameNoReplace(source, destination string) error {
	from, err := syscall.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := syscall.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return syscall.MoveFile(from, to)
}
