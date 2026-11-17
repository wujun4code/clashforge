package netfilter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Stateful PATH-only fake: reads reflect mutations, and unknown commands fail.
// No host networking executable is reachable from tests using this harness.
func ownershipMock(t *testing.T) (func() map[string]any, func(map[string]any)) {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	script := `import json,os,sys
p=os.environ['NF_OWNER_STATE']; s=json.load(open(p)); name=os.path.basename(sys.argv[0]); a=sys.argv[1:]; command=name+' '+' '.join(a)
s.setdefault('commands',[]).append(command)
def done(code=0,text=''):
 json.dump(s,open(p,'w')); print(text); sys.exit(code)
if any(x in command for x in s.get('fail',[])): done(42,'injected permission denied: '+command)
if name=='modprobe': done()
if name=='nft':
 if a==['list','tables']: done(0,'table inet metaclash' if s['nft'] else '')
 if a==['-f','-']: sys.stdin.read(); s['nft']=True; done()
 if a==['delete','table','inet','metaclash']:
  if not s['nft']: done(1,'No such file or directory')
  s['nft']=False; done()
if name in ['iptables','ip6tables']:
 if a==['-L','-n']: done(0,'Chain INPUT')
 key=name+' '+' '.join(a[:2]); op=a[2]; rest=a[3:]; entries=s.setdefault('xt',{})
 if op=='-N': entries[key+' '+rest[0]]=[]; done()
 if op in ['-A','-I']:
  chain=key+' '+rest[0]; rule=rest[1:]
  if op=='-I': rule=rule[1:]
  entries.setdefault(chain,[]).append(rule); done()
 if op in ['-D','-F','-X']:
  chain=key+' '+rest[0]
  if chain not in entries: done(1,s.get('xt_absent','No chain/target/match by that name.'))
  if op=='-D':
   if rest[1:] not in entries[chain]: done(1,'Bad rule (does a matching rule exist in that chain?).')
   entries[chain].remove(rest[1:])
  if op=='-F': entries[chain]=[]
  if op=='-X':
   if entries[chain]: done(1,'Directory not empty')
   if any(['-j',rest[0]]==r[-2:] for k,rs in entries.items() if k.startswith(key+' ') for r in rs): done(1,'Too many links')
   del entries[chain]
  done()
if name=='ip':
 family='6' if a and a[0]=='-6' else '4'
 if family=='6': a=a[1:]
 if a and a[0]=='-N': a=a[1:]
 if a[:2] in [['-o','rule'],['-o','route']]: a=a[1:]
 if a[:2]==['rule','show']:
  done(0,'\n'.join(s['rules'].get(family,[])))
 if a[:2]==['route','show']:
  table=a[a.index('table')+1]; done(0,'\n'.join(s['routes'].get(family+':'+table,[])))
 if a[:2]==['rule','add']:
  mark=a[a.index('fwmark')+1]; table=a[a.index('table')+1]
  rules=s['rules'].setdefault(family,[]); rules.append(str(32765-len(rules))+': from all fwmark '+mark+' lookup '+table); done()
 if a[:2]==['route','add']:
  table=a[a.index('table')+1]; s['routes'].setdefault(family+':'+table,[]).append('local default dev lo metric 1024 pref medium' if family=='6' else 'local default dev lo proto boot scope host'); done()
 if a[:2]==['rule','del']:
  mark=a[a.index('fwmark')+1]; table=a[a.index('table')+1]; rules=s['rules'].setdefault(family,[])
  for r in list(rules):
   if 'protocol' in a and ' proto ' in r and r.split(' proto ')[1] not in ['boot','3']: continue
   if 'from all fwmark '+mark+' lookup '+table in r and ('priority' not in a or r.split(':')[0]==a[a.index('priority')+1]): rules.remove(r); done()
  done(2,'RTNETLINK answers: No such file or directory')
 if a[:2]==['route','del']:
  table=a[a.index('table')+1]; routes=s['routes'].setdefault(family+':'+table,[])
  target=' '.join(a[2:a.index('table')])
  if target not in routes: done(2,'RTNETLINK answers: No such process')
  routes.remove(target); done()
 if a[:2]==['route','flush']:
  table=a[a.index('table')+1]; s['routes'][family+':'+table]=[]; done()
done(99,'unsupported fake command: '+command)
`
	for _, name := range []string{"nft", "ip", "iptables", "ip6tables", "modprobe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("#!"+python+"\n"+script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("NF_OWNER_STATE", state)
	initial := map[string]any{"nft": false, "xt": map[string]any{}, "rules": map[string]any{}, "routes": map[string]any{}, "commands": []string{}}
	b, _ := json.Marshal(initial)
	if err := os.WriteFile(state, b, 0600); err != nil {
		t.Fatal(err)
	}
	read := func() map[string]any {
		b, e := os.ReadFile(state)
		if e != nil {
			t.Fatal(e)
		}
		var s map[string]any
		if e = json.Unmarshal(b, &s); e != nil {
			t.Fatal(e)
		}
		return s
	}
	update := func(changes map[string]any) {
		s := read()
		for k, v := range changes {
			s[k] = v
		}
		b, e := json.Marshal(s)
		if e != nil {
			t.Fatal(e)
		}
		if e = os.WriteFile(state, b, 0600); e != nil {
			t.Fatal(e)
		}
	}
	return read, update
}

func assertOwnershipEmpty(t *testing.T, s map[string]any) {
	t.Helper()
	if s["nft"].(bool) {
		t.Error("nft ownership remains")
	}
	for k, v := range s["xt"].(map[string]any) {
		if strings.Contains(k, "METACLASH") || len(v.([]any)) != 0 {
			t.Errorf("iptables ownership remains: %s %v", k, v)
		}
	}
	for _, k := range []string{"rules", "routes"} {
		for family, v := range s[k].(map[string]any) {
			if len(v.([]any)) != 0 {
				t.Errorf("%s %s remains: %v", k, family, v)
			}
		}
	}
}

func TestOwnershipNoBackendRejected(t *testing.T) {
	for _, mode := range []string{"tproxy", "redir", "none"} {
		for _, backend := range []string{"none", "auto"} {
			t.Run(mode+"/"+backend, func(t *testing.T) {
				t.Setenv("PATH", t.TempDir())
				m := NewManager(Config{Mode: mode, FirewallBackend: backend})
				err := m.Apply()
				if mode != "none" && err == nil {
					t.Error("takeover without backend must fail")
				}
				if mode == "none" && err != nil {
					t.Fatal(err)
				}
				if m.IsApplied() {
					t.Error("backendless manager is not applied")
				}
				if err := m.Cleanup(); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestOwnershipPartialApplyCleanupRetry(t *testing.T) {
	for _, backend := range []string{"nftables", "iptables"} {
		t.Run(backend, func(t *testing.T) {
			read, update := ownershipMock(t)
			m := NewManager(Config{Mode: "tproxy", FirewallBackend: backend, TProxyPort: 7895, EnableIPv6: true})
			update(map[string]any{"fail": []string{"ip -6 rule add"}})
			if err := m.Apply(); err == nil || m.IsApplied() {
				t.Fatalf("partial Apply = %v applied=%v", err, m.IsApplied())
			}
			s := read()
			if !s["nft"].(bool) && len(s["xt"].(map[string]any)) == 0 {
				t.Fatal("fixture did not acquire firewall")
			}
			failure := "nft delete"
			if backend == "iptables" {
				failure = "iptables -t mangle -D PREROUTING"
			}
			update(map[string]any{"fail": []string{failure, "ip rule del"}})
			err := m.Cleanup()
			if err == nil || !strings.Contains(err.Error(), failure) || !strings.Contains(err.Error(), "ip rule del") {
				t.Errorf("must join firewall and route failures: %v", err)
			}
			if m.IsApplied() {
				t.Error("cleanup error is not readiness")
			}
			update(map[string]any{"fail": []string{}})
			if err := m.Cleanup(); err != nil {
				t.Fatal(err)
			}
			assertOwnershipEmpty(t, read())
			before := len(read()["commands"].([]any))
			if err := m.Cleanup(); err != nil {
				t.Fatal(err)
			}
			if len(read()["commands"].([]any)) != before {
				t.Error("completed manager cleanup should skip")
			}
		})
	}
}

func TestOwnershipCleanupErrorsAndRetry(t *testing.T) {
	for _, backend := range []string{"nftables", "iptables"} {
		failures := []string{"ip rule del", "ip route del", "ip -6 rule del", "ip -6 route del", "ip rule del fwmark " + fwMarkOutput, "ip route del local default dev lo proto boot scope host table " + routeTableOutput}
		if backend == "iptables" {
			for _, family := range []string{"iptables", "ip6tables"} {
				for _, operation := range []string{"-D PREROUTING -j METACLASH", "-F METACLASH", "-X METACLASH"} {
					failures = append(failures, family+" -t mangle "+operation)
				}
			}
			for _, operation := range []string{"-D OUTPUT -j METACLASH_OUTPUT", "-F METACLASH_OUTPUT", "-X METACLASH_OUTPUT"} {
				failures = append(failures, "iptables -t mangle "+operation)
			}
			for _, operation := range []string{"-D FORWARD -j METACLASH_IPV6", "-F METACLASH_IPV6", "-X METACLASH_IPV6"} {
				failures = append(failures, "ip6tables -t filter "+operation)
			}
			for _, chain := range []string{"PREROUTING", "OUTPUT"} {
				for _, protocol := range []string{"udp", "tcp"} {
					failures = append(failures, "iptables -t nat -D "+chain+" -p "+protocol)
				}
			}
		} else {
			failures = append(failures, "nft delete table")
		}
		for _, failure := range failures {
			t.Run(backend+"/"+failure, func(t *testing.T) {
				read, update := ownershipMock(t)
				m := NewManager(Config{Mode: "tproxy", FirewallBackend: backend, TProxyPort: 7895, DNSPort: 7874, EnableDNSRedirect: true, EnableIPv6: backend == "nftables"})
				if err := m.Apply(); err != nil {
					t.Fatal(err)
				}
				// Include residual IPv6 policy when current interception is IPv4-only.
				if backend == "iptables" {
					s := read()
					rules := s["rules"].(map[string]any)
					rules["6"] = []string{"32765: from all fwmark " + fwMark + " lookup " + routeTable}
					routes := s["routes"].(map[string]any)
					routes["6:"+routeTable] = []string{"local default dev lo scope host"}
					update(map[string]any{"rules": rules, "routes": routes})
				}
				update(map[string]any{"fail": []string{failure}})
				err := m.Cleanup()
				if err == nil || !strings.Contains(err.Error(), "permission denied") {
					t.Errorf("failure must propagate: %v", err)
				}
				if m.IsApplied() {
					t.Error("cleanup failure must clear readiness")
				}
				update(map[string]any{"fail": []string{}})
				if err := m.Cleanup(); err != nil {
					t.Fatal(err)
				}
				assertOwnershipEmpty(t, read())
			})
		}
	}
}

func TestOwnershipNftMissingTableStillCleansRoutes(t *testing.T) {
	read, update := ownershipMock(t)
	m := NewManager(Config{Mode: "tproxy", FirewallBackend: "nftables"})
	if err := m.Apply(); err != nil {
		t.Fatal(err)
	}
	update(map[string]any{"nft": false, "fail": []string{"ip route del"}})
	if err := m.Cleanup(); err == nil {
		t.Error("missing table must not hide residual route failure")
	}
	update(map[string]any{"fail": []string{}})
	if err := m.Cleanup(); err != nil {
		t.Fatal(err)
	}
	assertOwnershipEmpty(t, read())
}

func TestOwnershipCleanupDoesNotFlushForeignRoutes(t *testing.T) {
	read, update := ownershipMock(t)
	b := NftablesBackend{}
	update(map[string]any{"routes": map[string]any{"4:100": []string{"192.0.2.0/24 via 192.0.2.1 dev eth9"}}, "rules": map[string]any{"4": []string{"100: from 192.0.2.0/24 fwmark " + fwMark + " lookup " + routeTable, "101: from all fwmark 0x100 lookup 100", "102: from all fwmark " + fwMark + " lookup " + routeTable + " proto 242", "102: from all fwmark " + fwMark + " lookup " + routeTable}}})
	if err := b.Cleanup(); err != nil {
		t.Fatal(err)
	}
	s := read()
	if got := len(s["routes"].(map[string]any)["4:100"].([]any)); got != 1 {
		t.Error("foreign route was removed")
	}
	if got := len(s["rules"].(map[string]any)["4"].([]any)); got != 3 {
		t.Error("foreign selector removed")
	}
	if !strings.Contains(fmt.Sprint(s["rules"]), "proto 242") {
		t.Error("same-priority foreign protocol rule was removed instead of the owned boot-protocol rule")
	}
	for _, command := range s["commands"].([]any) {
		if strings.Contains(fmt.Sprint(command), "route flush") {
			t.Error("must not flush shared table")
		}
	}
}

func TestOwnershipIptablesNFTAbsentIsIdempotent(t *testing.T) {
	for _, diagnostic := range []string{
		"iptables v1.8.9 (nf_tables): Chain 'METACLASH' does not exist",
		"iptables: RULE_DELETE failed (No such file or directory): rule in chain PREROUTING",
	} {
		t.Run(diagnostic, func(t *testing.T) {
			_, update := ownershipMock(t)
			update(map[string]any{"xt_absent": diagnostic})
			if err := (&IptablesBackend{}).Cleanup(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestOwnershipInspectionFailureRetainsPending(t *testing.T) {
	for _, failure := range []string{"ip -N -o rule show", "ip -6 -N -o rule show", "ip -o route show", "ip -6 -o route show"} {
		t.Run(failure, func(t *testing.T) {
			read, update := ownershipMock(t)
			m := NewManager(Config{Mode: "tproxy", FirewallBackend: "nftables", EnableIPv6: true})
			if err := m.Apply(); err != nil {
				t.Fatal(err)
			}
			update(map[string]any{"fail": []string{failure}})
			if err := m.Cleanup(); err == nil || !strings.Contains(err.Error(), failure) {
				t.Fatalf("inspection error hidden: %v", err)
			}
			update(map[string]any{"fail": []string{}})
			if err := m.Cleanup(); err != nil {
				t.Fatal(err)
			}
			assertOwnershipEmpty(t, read())
		})
	}
}

func TestOwnershipCanonicalRouteBoundary(t *testing.T) {
	for _, tc := range []struct {
		line       string
		ipv6, want bool
	}{
		{"local default dev lo proto boot scope host", false, true},
		{"local default dev lo metric 1024 pref medium", true, true},
		{"local default dev lo proto static scope host", false, false},
		{"local default dev lo proto 242 scope host", false, false},
		{"local default dev eth9 scope host", false, false},
		{"local default dev lo metric 15", false, false},
		{"local default dev lo metric 2048 pref medium", true, false},
		{"local 192.0.2.0/24 dev lo scope host", false, false},
	} {
		if got := defaultTProxyRoute(strings.Fields(tc.line), tc.ipv6); got != tc.want {
			t.Errorf("%s ipv6=%v got %v", tc.line, tc.ipv6, got)
		}
	}
}
