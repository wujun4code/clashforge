// Package m1cpu is a build-time stand-in for github.com/shoenig/go-m1cpu,
// swapped in via a go.mod replace.  The real package's cgo path
// (darwin && arm64 && cgo) calls IOKit's kIOMasterPortDefault, which the
// iOS SDK marks unavailable, so gomobile bind fails.  GOOS=ios satisfies
// the darwin build tag, hence this stub: gopsutil (mihomo's stats dep)
// only uses these values for CPU-frequency reporting, which has no
// meaning inside a packet-tunnel extension anyway.
package m1cpu

// IsAppleSilicon reports false so callers (gopsutil cpu_darwin) take
// their generic sysctl fallback path instead of the per-core queries.
func IsAppleSilicon() bool { return false }

func PCoreHz() uint64 { return 0 }

func ECoreHz() uint64 { return 0 }

func PCoreGHz() float64 { return 0 }

func ECoreGHz() float64 { return 0 }

func PCoreCount() int { return 0 }

func ECoreCount() int { return 0 }

func PCoreCache() (int, int, int) { return 0, 0, 0 }

func ECoreCache() (int, int, int) { return 0, 0, 0 }

func ModelName() string { return "" }
