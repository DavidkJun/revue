//go:build !windows

package claude

import "time"

// killsTree reports whether cancellation is expected to kill grandchildren;
// elsewhere only the claude process itself is killed.
const killsTree = false

func processGone(int, time.Duration) bool { return true }
