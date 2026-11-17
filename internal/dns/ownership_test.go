package dns

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// All external networking commands are test-local PATH shims to this helper.
// No host UCI, service, nft, kill, or port-occupier process is executed.
type fakeDNSValue struct {
	List   bool     `json:"list"`
	Values []string `json:"values"`
}
type dnsFixture struct {
	t   *testing.T
	dir string
}

func newDNSFixture(t *testing.T, initial map[string]fakeDNSValue) *dnsFixture {
	t.Helper()
	d := t.TempDir()
	f := &dnsFixture{t, d}
	bin, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("DNS_TEST_BINARY", bin)
	// Race-instrumented helper processes should not sleep one second each.
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("DNS_TEST_DATA", d)
	t.Setenv("CLASHFORGE_DNS_STATE_DIR", filepath.Join(d, "ownership"))
	for _, name := range []string{"uci", "service", "nft", "kill"} {
		script := fmt.Sprintf("#!/bin/sh\nexec \"$DNS_TEST_BINARY\" -test.run=^TestDNSCommandHelper$ -- %s \"$@\"\n", name)
		if err := os.WriteFile(filepath.Join(d, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", d)
	f.write(initial)
	return f
}
func (f *dnsFixture) write(v map[string]fakeDNSValue) {
	f.t.Helper()
	b, _ := json.Marshal(v)
	if err := os.WriteFile(filepath.Join(f.dir, "uci.json"), b, 0600); err != nil {
		f.t.Fatal(err)
	}
}
func (f *dnsFixture) read() map[string]fakeDNSValue {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, "uci.json"))
	if err != nil {
		f.t.Fatal(err)
	}
	var v map[string]fakeDNSValue
	if err = json.Unmarshal(b, &v); err != nil {
		f.t.Fatal(err)
	}
	return v
}
func (f *dnsFixture) commands() string {
	b, _ := os.ReadFile(filepath.Join(f.dir, "commands"))
	return string(b)
}
func (f *dnsFixture) equal(want map[string]fakeDNSValue) {
	f.t.Helper()
	if got := f.read(); !reflect.DeepEqual(got, want) {
		f.t.Fatalf("UCI state differs (values intentionally not logged)")
	}
}
func TestDNSCommandHelper(t *testing.T) {
	idx := -1
	for i, a := range os.Args {
		if a == "--" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	args := os.Args[idx+1:]
	d := os.Getenv("DNS_TEST_DATA")
	line := strings.Join(args, " ")
	log, err := os.OpenFile(filepath.Join(d, "commands"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(90)
	}
	fmt.Fprintln(log, line)
	log.Close()
	if fail := os.Getenv("DNS_TEST_FAIL"); fail != "" && strings.Contains(line, fail) {
		os.Exit(42)
	}
	if noop := os.Getenv("DNS_TEST_NOOP"); noop != "" && strings.Contains(line, noop) {
		os.Exit(0)
	}
	if args[0] != "uci" {
		os.Exit(0)
	}
	b, _ := os.ReadFile(filepath.Join(d, "uci.json"))
	var fields map[string]fakeDNSValue
	json.Unmarshal(b, &fields)
	args = args[1:]
	for len(args) > 0 && strings.HasPrefix(args[0], "-") {
		args = args[1:]
	}
	if len(args) == 0 {
		os.Exit(2)
	}
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	switch args[0] {
	case "export":
		fmt.Println("package dhcp\n\nconfig dnsmasq 'cfgtest'")
		keys := make([]string, 0, len(fields))
		for k := range fields {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := fields[k]
			kind := "option"
			if v.List {
				kind = "list"
			}
			for _, s := range v.Values {
				fmt.Printf("\t%s %s %s\n", kind, quote(k), quote(s))
			}
		}
		os.Exit(0)
	case "show":
		fmt.Println("dhcp.cfgtest=dnsmasq")
		os.Exit(0)
	case "commit":
		os.Exit(0)
	case "set", "add_list", "delete":
		pair := strings.SplitN(args[1], "=", 2)
		key := pair[0][strings.LastIndex(pair[0], ".")+1:]
		switch args[0] {
		case "delete":
			if _, ok := fields[key]; !ok {
				os.Exit(1)
			}
			delete(fields, key)
		case "set":
			fields[key] = fakeDNSValue{Values: []string{pair[1]}}
		case "add_list":
			v := fields[key]
			v.List = true
			v.Values = append(v.Values, pair[1])
			fields[key] = v
		}
	default:
		os.Exit(2)
	}
	b, _ = json.Marshal(fields)
	if os.WriteFile(filepath.Join(d, "uci.json"), b, 0600) != nil {
		os.Exit(91)
	}
	if fail := os.Getenv("DNS_TEST_FAIL_AFTER"); fail != "" && strings.Contains(line, fail) {
		os.Exit(42)
	}
	os.Exit(0)
}
func userDNS() map[string]fakeDNSValue {
	return map[string]fakeDNSValue{
		"port": {Values: []string{"5353"}}, "server": {List: true, Values: []string{"/private domain/192.0.2.1#54", "2001:db8::1", "quote'and\\slash"}}, "noresolv": {Values: []string{"0"}}, "unrelated": {Values: []string{"untouched"}},
	}
}
func TestOwnershipRestoreWithoutSetupIsNoOp(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	for _, m := range []DnsmasqMode{ModeReplace, ModeUpstream, ModeNone} {
		if err := Restore(m); err != nil {
			t.Fatal(err)
		}
	}
	f.equal(initial)
	if f.commands() != "" {
		t.Fatal("unowned restore executed external commands")
	}
}
func TestOwnershipRestoresOriginalAcrossModes(t *testing.T) {
	for _, sequence := range [][]DnsmasqMode{{ModeUpstream, ModeUpstream}, {ModeUpstream, ModeReplace, ModeUpstream}, {ModeReplace, ModeReplace, ModeUpstream}} {
		t.Run(fmt.Sprint(sequence), func(t *testing.T) {
			initial := userDNS()
			f := newDNSFixture(t, initial)
			for i, m := range sequence {
				if err := Setup(m, 17874+i); err != nil {
					t.Fatal(err)
				}
			}
			// Restore deliberately uses a mode unrelated to the last Setup.
			if err := Restore(ModeNone); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
			before := f.commands()
			if err := Restore(ModeUpstream); err != nil {
				t.Fatal(err)
			}
			if f.commands() != before {
				t.Fatal("second restore is not a no-op")
			}
		})
	}
}
func TestOwnershipPreservesLaterUserEdits(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	if err := Setup(ModeUpstream, 17874); err != nil {
		t.Fatal(err)
	}
	modified := f.read()
	modified["server"] = fakeDNSValue{List: true, Values: []string{"192.0.2.99"}}
	modified["port"] = fakeDNSValue{Values: []string{"1053"}}
	f.write(modified)
	if err := Restore(ModeReplace); err != nil {
		t.Fatal(err)
	}
	initial["server"] = modified["server"]
	initial["port"] = modified["port"]
	f.equal(initial)
}
func TestOwnershipFailureRetry(t *testing.T) {
	for _, failure := range []string{"delete", "uci commit", "service dnsmasq restart"} {
		t.Run(failure, func(t *testing.T) {
			initial := map[string]fakeDNSValue{}
			f := newDNSFixture(t, initial)
			if err := Setup(ModeUpstream, 17874); err != nil {
				t.Fatal(err)
			}
			t.Setenv("DNS_TEST_FAIL", failure)
			if err := Restore(ModeReplace); err == nil {
				t.Fatal("restore failure swallowed")
			}
			t.Setenv("DNS_TEST_FAIL", "")
			if err := Restore(ModeUpstream); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
		})
	}
}
func TestOwnershipSetupFailureIsRecoverable(t *testing.T) {
	for _, failure := range []string{"uci delete", "uci add_list", "uci set", "uci commit", "service dnsmasq restart"} {
		t.Run(failure, func(t *testing.T) {
			initial := userDNS()
			f := newDNSFixture(t, initial)
			t.Setenv("DNS_TEST_FAIL", failure)
			if err := Setup(ModeUpstream, 17874); err == nil {
				t.Fatal("setup failure swallowed")
			}
			t.Setenv("DNS_TEST_FAIL", "")
			if err := Restore(ModeUpstream); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
		})
	}
}
func TestOwnershipStateCollision(t *testing.T) {
	f := newDNSFixture(t, userDNS())
	path := os.Getenv("CLASHFORGE_DNS_STATE_DIR")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "journal.json"), []byte("user state"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Setup(ModeUpstream, 17874); err == nil {
		t.Fatal("accepted unowned journal directory")
	}
	if f.commands() != "" {
		t.Fatal("collision mutated UCI")
	}
}
func TestOwnershipSurvivesProcessRestart(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	for _, op := range []string{"setup", "restore"} {
		cmd := exec.Command(os.Getenv("DNS_TEST_BINARY"), "-test.run=^TestOwnershipRestartChild$")
		cmd.Env = append(os.Environ(), "DNS_RESTART_CHILD="+op)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("child failed: %v: %s", err, out)
		}
	}
	f.equal(initial)
}
func TestOwnershipRestartChild(t *testing.T) {
	switch os.Getenv("DNS_RESTART_CHILD") {
	case "setup":
		if err := Setup(ModeUpstream, 17874); err != nil {
			t.Fatal(err)
		}
	case "restore":
		if err := Restore(ModeReplace); err != nil {
			t.Fatal(err)
		}
	}
}
