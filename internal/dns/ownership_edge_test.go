package dns

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOwnershipExactFieldTypes(t *testing.T) {
	for _, initial := range []map[string]fakeDNSValue{
		{},
		{"server": {Values: []string{"one scalar with spaces\nand newline"}}, "noresolv": {Values: []string{""}}},
		{"port": {List: true, Values: []string{"5353"}}, "server": {List: true, Values: []string{""}}, "noresolv": {List: true, Values: []string{"0", "1"}}},
	} {
		t.Run("roundtrip", func(t *testing.T) {
			f := newDNSFixture(t, initial)
			if err := Setup(ModeUpstream, 17874); err != nil {
				t.Fatal(err)
			}
			if err := Restore(ModeUpstream); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
		})
	}
}
func TestOwnershipRepeatedSetupRefusesUserEdits(t *testing.T) {
	f := newDNSFixture(t, userDNS())
	if err := Setup(ModeUpstream, 17874); err != nil {
		t.Fatal(err)
	}
	edited := f.read()
	edited["noresolv"] = fakeDNSValue{Values: []string{"user-edit"}}
	f.write(edited)
	before := f.commands()
	if err := Setup(ModeUpstream, 17875); err == nil {
		t.Fatal("repeated setup claimed user edit")
	}
	f.equal(edited)
	after := strings.TrimPrefix(f.commands(), before)
	if strings.Contains(after, "uci set") || strings.Contains(after, "uci delete") || strings.Contains(after, "uci add_list") {
		t.Fatal("preflight failure still mutated fields")
	}
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
	expected := userDNS()
	expected["noresolv"] = edited["noresolv"]
	f.equal(expected)
}
func TestOwnershipPartialListRestoreRetry(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	if err := Setup(ModeUpstream, 17874); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DNS_TEST_FAIL", "uci add_list dhcp.cfgtest.server=2001:db8::1")
	if err := Restore(ModeReplace); err == nil {
		t.Fatal("partial list restore failure swallowed")
	}
	t.Setenv("DNS_TEST_FAIL", "")
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
	f.equal(initial)
}
func TestOwnershipSetupRetryKeepsEarliestOriginal(t *testing.T) {
	for _, failure := range []string{"uci add_list", "uci commit", "service dnsmasq restart"} {
		t.Run(failure, func(t *testing.T) {
			initial := userDNS()
			f := newDNSFixture(t, initial)
			t.Setenv("DNS_TEST_FAIL", failure)
			if err := Setup(ModeUpstream, 17874); err == nil {
				t.Fatal("expected failure")
			}
			t.Setenv("DNS_TEST_FAIL", "")
			if err := Setup(ModeUpstream, 17875); err != nil {
				t.Fatal(err)
			}
			if err := Restore(ModeNone); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
		})
	}
}
func TestOwnershipSecureState(t *testing.T) {
	for _, which := range []string{"directory permissions", "journal permissions", "journal symlink", "journal corrupt"} {
		t.Run(which, func(t *testing.T) {
			f := newDNSFixture(t, userDNS())
			if err := Setup(ModeUpstream, 17874); err != nil {
				t.Fatal(err)
			}
			dir := os.Getenv("CLASHFORGE_DNS_STATE_DIR")
			path := filepath.Join(dir, "journal.json")
			switch which {
			case "directory permissions":
				if err := os.Chmod(dir, 0755); err != nil {
					t.Fatal(err)
				}
			case "journal permissions":
				if err := os.Chmod(path, 0644); err != nil {
					t.Fatal(err)
				}
			case "journal symlink":
				if err := os.Rename(path, path+".original"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(path+".original", path); err != nil {
					t.Fatal(err)
				}
			case "journal corrupt":
				if err := os.WriteFile(path, []byte("not a journal"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := f.commands()
			if err := Restore(ModeUpstream); err == nil {
				t.Fatal("unsafe state accepted")
			}
			if f.commands() != before {
				t.Fatal("unsafe ledger triggered external command")
			}
		})
	}
}
func setupTestDNSFiles(t *testing.T, mode DnsmasqMode, port int, paths ...string) error {
	t.Helper()
	l, err := openDNSLedger(true)
	if err != nil {
		return err
	}
	defer l.close()
	return l.setupFiles(mode, port, paths)
}
func TestOwnershipConfFileCollision(t *testing.T) {
	for _, content := range []string{"# user's DNS file\nserver=192.0.2.1\n", dnsManagedHeader + "port=0\n"} {
		t.Run("unowned", func(t *testing.T) {
			f := newDNSFixture(t, map[string]fakeDNSValue{})
			path := filepath.Join(f.dir, clashforgeConfName)
			if err := os.WriteFile(path, []byte(content), 0644); err != nil {
				t.Fatal(err)
			}
			if err := setupTestDNSFiles(t, ModeUpstream, 17874, path); err == nil {
				t.Fatal("overwrote same-name user file")
			}
			b, _ := os.ReadFile(path)
			if string(b) != content {
				t.Fatal("collision file was modified")
			}
			if err := Restore(ModeUpstream); err != nil {
				t.Fatal(err)
			}
			b, _ = os.ReadFile(path)
			if string(b) != content {
				t.Fatal("unowned file was removed")
			}
			if f.commands() != "" {
				t.Fatal("collision executed service/network commands")
			}
		})
	}
}
func TestOwnershipConfFileLifecycle(t *testing.T) {
	f := newDNSFixture(t, map[string]fakeDNSValue{})
	path := filepath.Join(f.dir, clashforgeConfName)
	for _, mode := range []DnsmasqMode{ModeReplace, ModeUpstream, ModeReplace} {
		if err := setupTestDNSFiles(t, mode, 17874, path); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DNS_TEST_FAIL", "service dnsmasq restart")
	if err := Restore(ModeNone); err == nil {
		t.Fatal("file restore restart failure swallowed")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("owned config was not removed")
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("CLASHFORGE_DNS_STATE_DIR"), "journal.json")); err != nil {
		t.Fatal("restore failure dropped file owner")
	}
	t.Setenv("DNS_TEST_FAIL", "")
	if err := Restore(ModeReplace); err != nil {
		t.Fatal(err)
	}
	before := f.commands()
	if err := Restore(ModeUpstream); err != nil {
		t.Fatal(err)
	}
	if f.commands() != before {
		t.Fatal("file second restore was not a no-op")
	}
}
func TestOwnershipConfFilePreservesUserEdit(t *testing.T) {
	f := newDNSFixture(t, map[string]fakeDNSValue{})
	path := filepath.Join(f.dir, clashforgeConfName)
	if err := setupTestDNSFiles(t, ModeReplace, 17874, path); err != nil {
		t.Fatal(err)
	}
	// Even retaining our header is insufficient without exact expected content.
	edited := dnsManagedHeader + "server=192.0.2.99\n"
	if err := os.WriteFile(path, []byte(edited), 0644); err != nil {
		t.Fatal(err)
	}
	if err := setupTestDNSFiles(t, ModeUpstream, 17875, path); err == nil {
		t.Fatal("overwrote edited config")
	}
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if string(b) != edited {
		t.Fatal("restore removed edited config")
	}
}
func TestOwnershipConfFileDeleteFailureRetry(t *testing.T) {
	f := newDNSFixture(t, map[string]fakeDNSValue{})
	path := filepath.Join(f.dir, "configs", clashforgeConfName)
	if err := setupTestDNSFiles(t, ModeUpstream, 17874, path); err != nil {
		t.Fatal(err)
	}
	// A symlink is never followed/deleted as an owned config; restore retains the
	// ledger and succeeds after the exact owned file is put back by the caller.
	original := path + ".saved"
	if err := os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(original, path); err != nil {
		t.Fatal(err)
	}
	if err := Restore(ModeNone); err == nil {
		t.Fatal("unsafe delete did not fail")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(original, path); err != nil {
		t.Fatal(err)
	}
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
}
func TestDNSExportQuoting(t *testing.T) {
	input := "package dhcp\nconfig dnsmasq\n option a 'has '\\'' quote\\ and\nnewline'\n list b ''\n option c \"escaped\\\"quote\"\n"
	got, err := dnsExportRecords(input)
	if err != nil {
		t.Fatal(err)
	}
	want := [][]string{{"package", "dhcp"}, {"config", "dnsmasq"}, {"option", "a", "has ' quote\\ and\nnewline"}, {"list", "b", ""}, {"option", "c", "escaped\"quote"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("UCI quoting was not lossless")
	}
	for _, bad := range []string{"option a 'unterminated", "option a \\", "option a \"unterminated"} {
		if _, err := dnsExportRecords(bad); err == nil {
			t.Fatal("malformed UCI accepted")
		}
	}
}
