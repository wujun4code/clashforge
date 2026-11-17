package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func versionFixture(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "core")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body+"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBoundedCoreVersionPreservesExitAndArguments(t *testing.T) {
	for _, code := range []int{0, 1} {
		body := "[ \"$#\" = 1 ] && [ \"$1\" = -v ] || exit 125\nprintf 'Mihomo Meta v1.19.32\\n'\nexit "
		if code == 0 {
			body += "0"
		} else {
			body += "1"
		}
		var out bytes.Buffer
		if got := coreVersionCheck(versionFixture(t, body), time.Second, &out); got != code {
			t.Fatalf("version exit: got %d want %d; %s", got, code, &out)
		}
		if !strings.Contains(out.String(), "Mihomo Meta v1.19.32") {
			t.Fatal("version output was lost")
		}
	}
}

func TestBoundedCoreVersionDeadline(t *testing.T) {
	var out bytes.Buffer
	start := time.Now()
	// exec replaces the test shell: cancellation leaves no descendant sleep.
	got := coreVersionCheck(versionFixture(t, "exec /bin/sleep 10"), 40*time.Millisecond, &out)
	if got != 124 || time.Since(start) > 2*time.Second {
		t.Fatalf("version check did not stop within budget: code=%d elapsed=%s", got, time.Since(start))
	}
}

func TestBoundedCoreVersionMissingBinary(t *testing.T) {
	var out bytes.Buffer
	if got := coreVersionCheck(filepath.Join(t.TempDir(), "missing"), time.Second, &out); got != 126 {
		t.Fatalf("missing binary exit=%d", got)
	}
}

func TestBoundedCoreVersionNoisyChildOutput(t *testing.T) {
	const limit = 32 * 1024
	stdout := "printf '%s' '" + strings.Repeat("o", 1024) + "'\n"
	stderr := "printf '%s' '" + strings.Repeat("e", 1024) + "' >&2\n"
	for _, tc := range []struct {
		name       string
		iterations string
		write      string
	}{
		{"stdout", "1024", stdout},
		{"stderr", "1024", stderr},
		{"mixed", "512", stdout + stderr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Emit 1 MiB through a real child: os/exec's io.Copy must not
			// bypass the cap via a promoted bytes.Buffer.ReadFrom method.
			body := "i=0\nwhile [ \"$i\" -lt " + tc.iterations + " ]; do\n" +
				tc.write + "i=$((i + 1))\ndone"
			var out bytes.Buffer
			if got := coreVersionCheck(versionFixture(t, body), 5*time.Second, &out); got != 0 {
				t.Fatalf("noisy child exit=%d; captured=%d", got, out.Len())
			}
			if out.Len() != limit {
				t.Fatalf("captured=%d bytes, want %d", out.Len(), limit)
			}
		})
	}
}

func TestCoreVersionOutputIsBounded(t *testing.T) {
	b := &versionOutput{limit: 16}
	for i := 0; i < 20; i++ {
		n, err := b.Write([]byte("abcdefgh"))
		if n != 8 || err != nil {
			t.Fatalf("unexpected write result %d %v", n, err)
		}
	}
	if string(b.Bytes()) != "abcdefghabcdefgh" {
		t.Fatalf("unexpected output %q", b.Bytes())
	}
}
