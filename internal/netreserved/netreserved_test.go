package netreserved

import "testing"

func TestParsePrefixes_SkipsInvalidEntries(t *testing.T) {
	got := ParsePrefixes([]string{"10.0.0.0/8", "not-a-cidr", "192.168.0.0/16"})
	if len(got) != 2 {
		t.Fatalf("expected 2 valid prefixes, got %d: %v", len(got), got)
	}
}

func TestIANARanges_NonEmpty(t *testing.T) {
	if len(IANARanges) == 0 {
		t.Fatal("IANARanges must not be empty — it's the shared baseline for both fake-ip-range selection and hijack detection")
	}
	if len(ParsePrefixes(IANARanges)) != len(IANARanges) {
		t.Fatal("every entry in IANARanges must be a valid CIDR")
	}
}
