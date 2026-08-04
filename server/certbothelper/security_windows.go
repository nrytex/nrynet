package certbothelper

import (
	"fmt"
	"os"
	"path/filepath"
)

func lockRequest(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("certbot helper is already running: %w", err)
	}
	_ = file.Close()
	return func() { _ = os.Remove(path) }, nil
}

func openRequestFile(path string) (*os.File, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > 65536 {
		return nil, fmt.Errorf("request file must be a regular file no larger than 64 KiB")
	}
	return os.Open(path)
}
