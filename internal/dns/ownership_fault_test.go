package dns

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOwnershipCompletePlanBeforeMutation(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	t.Setenv("DNS_TEST_FAIL", "uci set")
	if err := Setup(ModeUpstream, 17874); err == nil {
		t.Fatal("expected first mutation failure")
	}
	l, err := openDNSLedger(false)
	if err != nil {
		t.Fatal(err)
	}
	defer l.close()
	if len(l.state.Fields) != 3 {
		t.Fatal("first write preceded complete plan")
	}
	for _, key := range []string{"port", "server", "noresolv"} {
		field := l.state.Fields[key]
		original := initial[key]
		kind := "option"
		if original.List {
			kind = "list"
		}
		if !sameDNSValue(field.Original, dnsValue{Kind: kind, Values: original.Values}) || field.Planned.Kind == "" {
			t.Fatal("original or planned write not durably recorded")
		}
	}
	f.equal(initial)
}
func TestOwnershipAmbiguousCommandFailureRetry(t *testing.T) {
	for _, failure := range []string{"uci set", "uci delete", "uci add_list"} {
		t.Run(failure, func(t *testing.T) {
			initial := userDNS()
			f := newDNSFixture(t, initial)
			t.Setenv("DNS_TEST_FAIL_AFTER", failure)
			if err := Setup(ModeUpstream, 17874); err == nil {
				t.Fatal("write-then-fail not propagated")
			}
			t.Setenv("DNS_TEST_FAIL_AFTER", "")
			if err := Restore(ModeNone); err != nil {
				t.Fatal(err)
			}
			f.equal(initial)
		})
	}
}
func TestOwnershipRejectsFalseSuccess(t *testing.T) {
	initial := userDNS()
	f := newDNSFixture(t, initial)
	t.Setenv("DNS_TEST_NOOP", "uci set")
	if err := Setup(ModeUpstream, 17874); err == nil {
		t.Fatal("accepted command success without the intended mutation")
	}
	t.Setenv("DNS_TEST_NOOP", "")
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
	f.equal(initial)
}
func TestOwnershipNoOpLeavesNoState(t *testing.T) {
	f := newDNSFixture(t, userDNS())
	if err := Restore(ModeUpstream); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(os.Getenv("CLASHFORGE_DNS_STATE_DIR")); !os.IsNotExist(err) {
		t.Fatal("unowned restore created state")
	}
	f.equal(userDNS())
}
func TestOwnershipJournalRemovedOnlyAfterCompleteRestore(t *testing.T) {
	f := newDNSFixture(t, userDNS())
	if err := Setup(ModeUpstream, 17874); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(os.Getenv("CLASHFORGE_DNS_STATE_DIR"), "journal.json")
	t.Setenv("DNS_TEST_FAIL", "service dnsmasq restart")
	if err := Restore(ModeNone); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("restart failure discarded ownership")
	}
	f.equal(userDNS())
	t.Setenv("DNS_TEST_FAIL", "")
	if err := Restore(ModeNone); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("complete restore retained active owner")
	}
}
