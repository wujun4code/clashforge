// Package process is a build-time stand-in for
// github.com/shirou/gopsutil/v3/process, swapped in via a go.mod replace.
// The real package's darwin cgo files include macOS-only headers
// (libproc.h) absent from the iOS SDK, so gomobile bind fails; GOOS=ios
// satisfies the darwin build tag.
//
// mihomo v1.18.10 touches exactly one call here — tunnel/statistic's
// Manager.updateMemory reads MemoryInfo().RSS to serve the REST /memory
// endpoint — so the stub reports the Go runtime's resident footprint,
// which is a fair approximation for the single-runtime tunnel extension.
package process

import "runtime"

type Process struct {
	Pid int32 `json:"pid"`
}

func NewProcess(pid int32) (*Process, error) {
	return &Process{Pid: pid}, nil
}

// MemoryInfoStat mirrors the upstream field set (json tags included) so
// any consumer serialising the struct stays wire-compatible.
type MemoryInfoStat struct {
	RSS    uint64 `json:"rss"`
	VMS    uint64 `json:"vms"`
	HWM    uint64 `json:"hwm"`
	Data   uint64 `json:"data"`
	Stack  uint64 `json:"stack"`
	Locked uint64 `json:"locked"`
	Swap   uint64 `json:"swap"`
}

// MemoryInfo reports the Go runtime's OS-held memory minus pages already
// returned to the kernel — the closest cgo-free analogue of RSS, and the
// number jetsam effectively budgets for the Go side of the extension.
func (p *Process) MemoryInfo() (*MemoryInfoStat, error) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	rss := ms.Sys - ms.HeapReleased
	return &MemoryInfoStat{
		RSS: rss,
		VMS: ms.Sys,
	}, nil
}
