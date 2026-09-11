//go:build windows

package filemgr

import "golang.org/x/sys/windows"

// freeBytesAt returns the number of bytes available to the current user
// in the filesystem containing path.
func freeBytesAt(path string) (uint64, error) {
	ptr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	if err := windows.GetDiskFreeSpaceEx(ptr, &free, &total, &totalFree); err != nil {
		return 0, err
	}
	return free, nil
}
