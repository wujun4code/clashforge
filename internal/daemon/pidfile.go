package daemon

import (
	"fmt"
	"os"
	"strconv"
)

type PIDFile struct {
	path string
}

func AcquirePIDFile(path string) (*PIDFile, error) {
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return nil, fmt.Errorf("pidfile already exists at %s (pid=%s)", path, string(data))
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
