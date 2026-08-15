//go:build linux || darwin

package app

import "syscall"

// diskUsage returns total/available/used bytes for the filesystem holding dir.
func diskUsage(dir string) (total, avail, used uint64, err error) {
	var st syscall.Statfs_t
	if err = syscall.Statfs(dir, &st); err != nil {
		return 0, 0, 0, err
	}
	total = st.Blocks * uint64(st.Bsize)
	avail = st.Bavail * uint64(st.Bsize)
	if total > avail {
		used = total - avail
	}
	return total, avail, used, nil
}
