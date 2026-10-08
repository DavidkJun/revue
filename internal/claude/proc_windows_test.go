//go:build windows

package claude

import (
	"time"

	"golang.org/x/sys/windows"
)

// killsTree reports whether cancellation is expected to kill grandchildren.
const killsTree = true

func processGone(pid int, timeout time.Duration) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return true
	}
	defer windows.CloseHandle(h)
	ev, err := windows.WaitForSingleObject(h, uint32(timeout/time.Millisecond))
	return err == nil && ev == windows.WAIT_OBJECT_0
}
