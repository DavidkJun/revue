//go:build windows

package git

import (
	"os/exec"
	"syscall"
)

// createNoWindow is CREATE_NO_WINDOW: git gets no console at all, so it can
// neither attach to the TUI's console (and disturb its input mode) nor flash
// a window of its own.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
