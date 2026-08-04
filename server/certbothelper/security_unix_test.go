//go:build !windows

package certbothelper

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLockIgnoresStaleFileButRejectsConcurrentHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), "request.lock")
	if err := os.WriteFile(path, []byte("stale"), 0o640); err != nil {
		t.Fatal(err)
	}
	unlock, err := lockRequest(path)
	if err != nil {
		t.Fatalf("stale lock blocked helper: %v", err)
	}
	if _, err := lockRequest(path); err == nil {
		t.Fatal("concurrent helper acquired the same lock")
	}
	unlock()
	unlockAgain, err := lockRequest(path)
	if err != nil {
		t.Fatalf("released lock stayed blocked: %v", err)
	}
	unlockAgain()
}

func TestOpenRequestRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.json")
	link := filepath.Join(dir, "request.json")
	if err := os.WriteFile(target, []byte(`{"action":"issue"}`), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := openRequestFile(link); err == nil {
		t.Fatal("symlink request was accepted")
	}
}
