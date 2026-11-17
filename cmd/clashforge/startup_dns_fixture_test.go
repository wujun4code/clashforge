package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type startupDNSValue struct {
	List   bool
	Values []string
}

func installStartupDNSFixture(t *testing.T, dir string) {
	t.Helper()
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("CF_STARTUP_TEST_BINARY", binary)
	t.Setenv("GORACE", "atexit_sleep_ms=0")
	t.Setenv("CLASHFORGE_DNS_STATE_DIR", filepath.Join(dir, "dns-owner"))
	initial := map[string]startupDNSValue{"port": {Values: []string{"5353"}}, "server": {List: true, Values: []string{"203.0.113.53"}}, "noresolv": {Values: []string{"0"}}}
	data, _ := json.Marshal(initial)
	if err := os.WriteFile(filepath.Join(dir, "dns-values.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nexec \"$CF_STARTUP_TEST_BINARY\" -test.run=^TestStartupDNSCommandHelper$ -- \"$@\"\n"
	if err := os.WriteFile(filepath.Join(dir, "uci"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
}

func startupDNSState(t *testing.T) map[string]startupDNSValue {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "dns-values.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]startupDNSValue
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	return fields
}

// Real DNS ownership code reads and mutates this state through a test-only UCI
// executable. Success with no state mutation is not accepted by the backend.
func TestStartupDNSCommandHelper(t *testing.T) {
	idx := -1
	for i, arg := range os.Args {
		if arg == "--" {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	args := os.Args[idx+1:]
	line := "uci " + strings.Join(args, " ")
	f, err := os.OpenFile(os.Getenv("CF_COMMAND_LOG"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		os.Exit(90)
	}
	fmt.Fprintln(f, line)
	f.Close()
	if fail := os.Getenv("CF_FAIL_COMMAND"); fail != "" && strings.Contains("uci:"+strings.Join(args, " "), fail) {
		os.Exit(42)
	}
	path := filepath.Join(os.Getenv("CF_MOCK_STATE_DIR"), "dns-values.json")
	data, _ := os.ReadFile(path)
	fields := map[string]startupDNSValue{}
	if err := json.Unmarshal(data, &fields); err != nil {
		os.Exit(91)
	}
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
		for key := range fields {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := fields[key]
			kind := "option"
			if value.List {
				kind = "list"
			}
			for _, item := range value.Values {
				fmt.Printf("\t%s %s %s\n", kind, quote(key), quote(item))
			}
		}
	case "show":
		fmt.Println("dhcp.cfgtest=dnsmasq")
	case "commit":
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
			fields[key] = startupDNSValue{Values: []string{pair[1]}}
		case "add_list":
			value := fields[key]
			value.List = true
			value.Values = append(value.Values, pair[1])
			fields[key] = value
		}
		data, _ = json.Marshal(fields)
		if err := os.WriteFile(path, data, 0600); err != nil {
			os.Exit(92)
		}
	default:
		os.Exit(2)
	}
	os.Exit(0)
}

func assertStartupDNSEqual(t *testing.T, want map[string]startupDNSValue) {
	t.Helper()
	if got := startupDNSState(t); !reflect.DeepEqual(got, want) {
		t.Fatalf("DNS state changed: got %+v want %+v", got, want)
	}
}
