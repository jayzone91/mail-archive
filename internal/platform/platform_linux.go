package platform

import (
	"os"
	"path/filepath"
	"syscall"
)

func DataDir() (string, error) {
	if base := os.Getenv("XDG_DATA_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "mailarchive"), nil
	}
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "mailarchive"), err
}

func PrivateDir(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return err
	}
	return os.Chmod(path, 0700)
}

var StopSignals = []os.Signal{os.Interrupt, syscall.SIGTERM}
