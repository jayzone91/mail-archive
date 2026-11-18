package platform

import "golang.org/x/sys/windows"

func PublishFile(temp, destination string) error {
	return moveFile(temp, destination, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func PublishNewDirectory(temp, destination string) error {
	return moveFile(temp, destination, windows.MOVEFILE_WRITE_THROUGH)
}

func moveFile(temp, destination string, flags uint32) error {
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, flags)
}
