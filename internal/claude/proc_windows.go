//go:build windows

package claude

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// batchUnsafe is what cmd.exe expands, splits on or executes in a command
// line quoted the C-runtime way, as exec quotes it. A quote is included
// because exec escapes it as \", which ends cmd.exe's quoted region.
const batchUnsafe = "%\"&|<>^\r\n"

// checkBatchArgs refuses arguments that would not reach claude intact when
// it is a .cmd or .bat shim (npm installs it as one): Windows runs those
// through cmd.exe, which would rewrite them or run part of them as commands.
func checkBatchArgs(path string, args []string) error {
	if ext := strings.ToLower(filepath.Ext(path)); ext != ".cmd" && ext != ".bat" {
		return nil
	}
	for _, a := range args {
		if strings.ContainsAny(a, batchUnsafe) {
			return fmt.Errorf("claude: %s runs through cmd.exe, which cannot pass the argument %q unchanged", filepath.Base(path), a)
		}
	}
	return nil
}

// hideConsole keeps claude (and everything it spawns, which inherits the
// windowless console) off the TUI's console, so it can never change its
// modes or flash a window.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// procTree is claude plus everything it spawns (Grep runs rg). They share a
// job object, so killing the job kills the whole tree, and closing the
// last handle to it (even if revue itself dies) does too.
type procTree struct {
	proc *os.Process
	mu   sync.Mutex
	job  windows.Handle // 0 when the job could not be set up
}

// attachTree puts the started process into a kill-on-close job. The child
// runs briefly before it is assigned, but claude starts no subprocess that
// early. Without a job, kill falls back to killing just the process.
func attachTree(p *os.Process) *procTree {
	t := &procTree{proc: p}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return t
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info)))
	if err == nil {
		err = assign(job, p.Pid)
	}
	if err != nil {
		windows.CloseHandle(job)
		return t
	}
	t.job = job
	return t
}

func assign(job windows.Handle, pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(job, h)
}

func (t *procTree) kill() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		windows.TerminateJobObject(t.job, 1)
	}
	t.proc.Kill()
}

// release closes the job once claude has exited, which also kills any
// grandchild it left behind.
func (t *procTree) release() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.job != 0 {
		windows.CloseHandle(t.job)
		t.job = 0
	}
}
