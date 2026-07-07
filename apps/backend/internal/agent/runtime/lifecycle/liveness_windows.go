//go:build windows

package lifecycle

import "os"

// isLocalPIDAlive reports whether a process with the given pid currently exists
// on this host. On Windows os.FindProcess opens the process handle (via
// OpenProcess) and returns an error when the pid does not refer to a live
// process, so a successful open means the process is alive. Only ever called for
// local/standalone rows — never for a remote SSH pid (see RowProcessLiveness).
func isLocalPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	return true
}
