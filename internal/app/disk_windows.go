//go:build windows

package app

import "golang.org/x/sys/windows"

// diskUsage returns total/available/used bytes for the filesystem holding dir.
func diskUsage(dir string) (total, avail, used uint64, err error) {
	p, err := windows.UTF16PtrFromString(dir)
	if err != nil {
		return 0, 0, 0, err
	}
	var free, tot, totalFree uint64
	if err = windows.GetDiskFreeSpaceEx(p, &free, &tot, &totalFree); err != nil {
		return 0, 0, 0, err
	}
	total = tot
	avail = free
	if total > avail {
		used = total - avail
	}
	return total, avail, used, nil
}
