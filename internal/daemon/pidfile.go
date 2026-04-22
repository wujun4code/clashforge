package daemon

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

type PIDFile struct {
	path string
}

func AcquirePIDFile(path string) (*PIDFile, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		// Check if the recorded process is still alive; if not, remove stale pidfile
		pidStr := string(data)
		if pid, perr := strconv.Atoi(strings.TrimSpace(pidStr)); perr == nil {
			if proc, perr2 := os.FindProcess(pid); perr2 == nil {
				if err2 := proc.Signal(os.Signal(syscall.Signal(0))); err2 == nil {
					// Process is alive — real conflict
					return nil, fmt.Errorf("pidfile already exists at %s (pid=%s\n)", path, pidStr)
				}
			}
		}
		// Stale pidfile — process is gone, remove and continue
		_ = os.Remove(path)
	}
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o644); err != nil {
		return nil, err
	}
	return &PIDFile{path: path}, nil
}

func (p *PIDFile) Close() error {
	if p == nil || p.path == "" {
		return nil
	}
	return os.Remove(p.path)
}
