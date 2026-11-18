package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// SharesStorage recognizes common Linux block devices and device-mapper stacks.
// Unrecognized mount topologies report an error instead of asserting independence.
func SharesStorage(first, second string) (bool, error) {
	var left, right unix.Stat_t
	if err := unix.Stat(first, &left); err != nil {
		return false, err
	}
	if err := unix.Stat(second, &right); err != nil {
		return false, err
	}
	if left.Dev == right.Dev {
		return true, nil
	}
	firstDisks, err := linuxDisks(left.Dev)
	if err != nil {
		return false, err
	}
	secondDisks, err := linuxDisks(right.Dev)
	if err != nil {
		return false, err
	}
	for disk := range firstDisks {
		if secondDisks[disk] {
			return true, nil
		}
	}
	return false, nil
}

func linuxDisks(device uint64) (map[string]bool, error) {
	path, err := filepath.EvalSymlinks(fmt.Sprintf("/sys/dev/block/%d:%d", unix.Major(device), unix.Minor(device)))
	if err != nil {
		return nil, err
	}
	disks := map[string]bool{}
	var resolve func(string) error
	resolve = func(node string) error {
		if _, err := os.Stat(filepath.Join(node, "partition")); err == nil {
			node = filepath.Dir(node)
		} else if !os.IsNotExist(err) {
			return err
		}
		slaves, err := os.ReadDir(filepath.Join(node, "slaves"))
		if err != nil {
			return err
		}
		if len(slaves) == 0 {
			if strings.Contains(node, "/virtual/") {
				return errors.New("Virtuelles Speichermedium konnte nicht zugeordnet werden.")
			}
			disks[node] = true
			return nil
		}
		for _, slave := range slaves {
			child, err := filepath.EvalSymlinks(filepath.Join(node, "slaves", slave.Name()))
			if err != nil {
				return err
			}
			if err := resolve(child); err != nil {
				return err
			}
		}
		return nil
	}
	return disks, resolve(path)
}
