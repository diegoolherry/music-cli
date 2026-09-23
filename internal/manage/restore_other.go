//go:build !windows && !linux

package manage

import "errors"

func renameNoReplace(source, destination string) error {
	return errors.New("atomic no-replace restoration unavailable on this platform")
}
