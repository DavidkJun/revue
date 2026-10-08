//go:build windows

package ui

import (
	"os/exec"
	"syscall"
)

// createNoWindow keeps background helpers (wezterm cli) from attaching to
// the TUI's console, which would disturb its input mode.
const createNoWindow = 0x08000000

func hideWindow(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNoWindow, HideWindow: true}
}
