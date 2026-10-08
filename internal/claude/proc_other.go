//go:build !windows

package claude

import (
	"os"
	"os/exec"
)

func hideConsole(*exec.Cmd) {}

func checkBatchArgs(string, []string) error { return nil }

// procTree only knows the claude process itself; killing it is enough to
// end the stream, and WaitDelay covers a grandchild that keeps the pipes.
type procTree struct {
	proc *os.Process
}

func attachTree(p *os.Process) *procTree { return &procTree{proc: p} }

func (t *procTree) kill() { t.proc.Kill() }

func (t *procTree) release() {}
