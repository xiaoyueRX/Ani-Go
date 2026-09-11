//go:build windows

package downloader

import (
	"golang.org/x/sys/windows"
)

func getFreeDiskSpace(path string) (uint64, error) {
	pathPtr, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	var freeBytesAvailable, totalBytes, totalFreeBytes uint64
	err = windows.GetDiskFreeSpaceEx(pathPtr, &freeBytesAvailable, &totalBytes, &totalFreeBytes)
	if err != nil {
		return 0, err
	}
	return freeBytesAvailable, nil
}
