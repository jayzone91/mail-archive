package platform

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

func PublishFile(temp, destination string) error {
	if err := os.Rename(temp, destination); err != nil {
		return err
	}
	return syncParent(destination)
}

func PublishNewDirectory(temp, destination string) error {
	if err := unix.Renameat2(unix.AT_FDCWD, temp, unix.AT_FDCWD, destination, unix.RENAME_NOREPLACE); err != nil {
		return err
	}
	return syncParent(destination)
}

func syncParent(destination string) error {
	directory, err := os.Open(filepath.Dir(destination))
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}
