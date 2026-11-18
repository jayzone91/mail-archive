package platform

import (
	"encoding/binary"
	"errors"
	"strings"

	"golang.org/x/sys/windows"
)

func SharesStorage(first, second string) (bool, error) {
	source, err := volumeName(first)
	if err != nil {
		return false, err
	}
	target, err := volumeName(second)
	if err != nil {
		return false, err
	}
	if strings.EqualFold(source, target) {
		return true, nil
	}
	left, err := windowsDisks(source)
	if err != nil {
		return false, err
	}
	right, err := windowsDisks(target)
	if err != nil {
		return false, err
	}
	for disk := range left {
		if right[disk] {
			return true, nil
		}
	}
	return false, nil
}

func volumeName(path string) (string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	mount := make([]uint16, 32768)
	if err := windows.GetVolumePathName(name, &mount[0], uint32(len(mount))); err != nil {
		return "", err
	}
	volume := make([]uint16, 128)
	if err := windows.GetVolumeNameForVolumeMountPoint(&mount[0], &volume[0], uint32(len(volume))); err != nil {
		return "", err
	}
	return windows.UTF16ToString(volume), nil
}

func windowsDisks(volume string) (map[uint32]bool, error) {
	name, err := windows.UTF16PtrFromString(strings.TrimSuffix(volume, `\`))
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(name, 0, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(handle)
	const queryExtents = 0x00560000 // IOCTL_VOLUME_GET_VOLUME_DISK_EXTENTS
	const header, extent = 8, 24
	var size uint32
	result := make([]byte, header+extent*256)
	if err := windows.DeviceIoControl(handle, queryExtents, nil, 0, &result[0], uint32(len(result)), &size, nil); err != nil {
		return nil, err
	}
	if size < header {
		return nil, errors.New("Unvollständige Windows-Datenträgerzuordnung.")
	}
	count := binary.LittleEndian.Uint32(result)
	if count == 0 || count > (size-header)/extent {
		return nil, errors.New("Ungültige Windows-Datenträgerzuordnung.")
	}
	disks := make(map[uint32]bool, count)
	for i := uint32(0); i < count; i++ {
		disks[binary.LittleEndian.Uint32(result[header+i*extent:])] = true
	}
	return disks, nil
}
