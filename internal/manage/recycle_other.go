//go:build !windows

package manage

import "fmt"

func recycleDirectory(string) error { return fmt.Errorf("Recycle Bin is only supported on Windows") }
