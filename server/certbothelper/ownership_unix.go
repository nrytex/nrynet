//go:build !windows

package certbothelper

import (
	"os"
	"os/user"
	"path/filepath"
	"strconv"
)

func chownTargets(options Options) error {
	group, err := user.LookupGroup(options.ServiceGroup)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	for _, path := range []string{options.FullchainPath, options.PrivateKeyPath} {
		if err := os.Chown(path, 0, gid); err != nil {
			return err
		}
	}
	return nil
}

func chownStatus(options Options) error {
	group, err := user.LookupGroup(options.ServiceGroup)
	if err != nil {
		return err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return err
	}
	if err := os.Chown(filepath.Dir(options.StatusPath), 0, gid); err != nil {
		return err
	}
	return os.Chown(options.StatusPath, 0, gid)
}
