package api

import "testing"

func TestParseDHCPLeases(t *testing.T) {
	raw := []byte(`
1745729999 aa:bb:cc:dd:ee:01 192.168.1.101 iphone *
1745729999 aa:bb:cc:dd:ee:02 192.168.1.102 * *
`)
	clients := parseDHCPLeases(raw)
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
	if clients[0].IP != "192.168.1.101" || clients[0].Hostname != "iphone" {
		t.Fatalf("unexpected first client: %#v", clients[0])
	}
	if clients[1].Hostname != "" {
		t.Fatalf("hostname '*' should be normalized to empty, got %#v", clients[1])
	}
}

func TestParseIPNeigh(t *testing.T) {
	raw := []byte(`
192.168.1.1 dev br-lan lladdr aa:bb:cc:dd:ee:ff REACHABLE
192.168.1.2 dev br-lan INCOMPLETE
192.168.1.3 dev br-lan lladdr aa:bb:cc:dd:ee:11 STALE
`)
	clients := parseIPNeigh(raw)
	if len(clients) != 2 {
		t.Fatalf("expected 2 clients, got %d", len(clients))
	}
	if clients[0].IP != "192.168.1.1" || clients[0].MAC != "aa:bb:cc:dd:ee:ff" || clients[0].Interface != "br-lan" {
		t.Fatalf("unexpected first client: %#v", clients[0])
	}
	if clients[1].IP != "192.168.1.3" {
		t.Fatalf("unexpected second client: %#v", clients[1])
	}
}

func TestParseProcARP(t *testing.T) {
	raw := []byte(`IP address       HW type     Flags       HW address            Mask     Device
192.168.1.100     0x1         0x2         aa:bb:cc:dd:ee:12     *        br-lan
192.168.1.101     0x1         0x2         00:00:00:00:00:00     *        br-lan
`)
	clients := parseProcARP(raw)
	if len(clients) != 1 {
		t.Fatalf("expected 1 client, got %d", len(clients))
	}
	if clients[0].IP != "192.168.1.100" || clients[0].MAC != "aa:bb:cc:dd:ee:12" {
		t.Fatalf("unexpected client parsed: %#v", clients[0])
	}
}
