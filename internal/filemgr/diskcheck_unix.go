//go:build !windows

package filemgr

import "syscall"

// freeBytesAt returns the number of bytes available to the current user
// in the filesystem containing path.
func freeBytesAt(path string) (uint64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return 0, err
	}
	return st.Bavail * uint64(st.Bsize), nil
}
