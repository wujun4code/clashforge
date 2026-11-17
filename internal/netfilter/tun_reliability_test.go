package netfilter

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Every networking executable is replaced; PATH never falls back to host tools.
func tunMock(t *testing.T, rules, nft []string, fail string) func() map[string]any {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	state := filepath.Join(dir, "state.json")
	if rules == nil {
		rules = []string{}
	}
	if nft == nil {
		nft = []string{}
	}
	data, _ := json.Marshal(map[string]any{"rules": rules, "nft": nft, "fail": fail, "calls": []string{}})
	if err := os.WriteFile(state, data, 0600); err != nil {
		t.Fatal(err)
	}
	script := `#!` + python + `
import os,sys,json
p=os.environ['TUN_MOCK_STATE']
s=json.load(open(p)); args=sys.argv[1:]; name=os.path.basename(sys.argv[0]); cmd=name+' '+' '.join(args)
s['calls'].append(cmd)
def done(out='',code=0):
 json.dump(s,open(p,'w')); print(out); sys.exit(code)
cmd=cmd.replace(' -d ', ' ')
failures=s['fail'] if isinstance(s['fail'],list) else [s['fail']]
if any(f and f in cmd.replace(' -4 ', ' ') for f in failures): done('injected failure',1)
if any(f in cmd.replace(' -4 ', ' ') for f in s.get('noop',[])): done()
if name=='ip':
 a=[x for x in args if x not in ('-4','-6','-d')]
 if a[:2]==['rule','show']: done('\n'.join(s['rules'] or []))
 if a[:2]==['route','show']: done(s.get('route','default dev Meta scope link'))
 if a[:2]==['link','show']: done(s.get('link','9: Meta: <POINTOPOINT,UP> mtu 1500'))
 if a[:2]==['rule','add']:
  pref=a[a.index('pref')+1]; table=a[a.index('lookup')+1]
  line=pref+': from all lookup '+table
  if 'suppress_prefixlength' in a: line+=' suppress_prefixlength '+a[a.index('suppress_prefixlength')+1]
  if 'protocol' in a: line+=' proto '+a[a.index('protocol')+1]
  s['rules']=(s['rules'] or [])+[line]; done()
 if a[:2]==['rule','del']:
  pref=a[a.index('pref')+1]; table=a[a.index('lookup')+1]
  for r in s['rules'] or []:
   if r.startswith(pref+':') and 'lookup '+table in r:
    if 'protocol' in a and 'proto '+a[a.index('protocol')+1] not in r: continue
    s['rules'].remove(r); done()
  done('No such rule',2)
if name=='nft':
 if 'list' in args: done('\n'.join(s['nft'] or []))
 if 'insert' in args:
  dev=args[args.index('oifname')+1]
  prefix='meta nfproto ipv4 ' if 'nfproto' in args else ''
  s['nft']=(s['nft'] or [])+[prefix+'oifname "'+dev+'" counter packets 0 bytes 0 accept comment "clashforge: allow forwarding into TUN device" # handle 42']; done()
 if 'delete' in args:
  h=args[args.index('handle')+1]; s['nft']=[r for r in (s['nft'] or []) if not r.endswith('# handle '+h)]; done()
done('unexpected mock command '+cmd,99)
`
	for _, name := range []string{"ip", "nft", "iptables", "ip6tables", "modprobe", "uci", "fw4"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	t.Setenv("TUN_MOCK_STATE", state)
	return func() map[string]any {
		t.Helper()
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
}

func TestTUNApplyRejectsFailedTakeover(t *testing.T) {
	for _, failure := range []string{"nft insert", "ip rule add"} {
		t.Run(failure, func(t *testing.T) {
			// Both old and new command families are supported by the recording mock.
			tunMock(t, nil, nil, strings.ReplaceAll(failure, "ip -4", "ip"))
			m := NewManager(Config{Mode: "tun", FirewallBackend: "nftables"})
			err := m.Apply()
			if err == nil || m.IsApplied() {
				t.Fatalf("takeover failure falsely succeeded: error=%v applied=%v", err, m.IsApplied())
			}
		})
	}
}

func TestTUNRejectsUnrelatedPriorityCollision(t *testing.T) {
	read := tunMock(t, []string{"9020: from all lookup main", "9021: from all lookup 999"}, nil, "")
	if err := EnsureTunRouteRule(); err == nil {
		t.Error("unrelated 9020/9021 occupants accepted as valid TUN routes")
	}
	s := read()
	for _, v := range s["calls"].([]any) {
		if strings.Contains(v.(string), "rule add") {
			t.Errorf("mutated rules despite conflict: %v", v)
		}
	}
}

func TestTUNCleanupPreservesForeignRules(t *testing.T) {
	read := tunMock(t, []string{"9020: from all lookup main suppress_prefixlength 0", "9021: from all lookup 2022"}, nil, "")
	if err := RemoveTunRouteRule(); err != nil {
		t.Fatal(err)
	}
	if got := len(read()["rules"].([]any)); got != 2 {
		t.Fatalf("deleted foreign/untagged rules at our priorities: remaining=%d", got)
	}
}

func TestTUNCleanupReportsFailure(t *testing.T) {
	tunMock(t, []string{"9020: from all lookup main suppress_prefixlength 0 proto 242", "9021: from all lookup 2022 proto 242"}, nil, "rule del")
	if err := RemoveTunRouteRule(); err == nil {
		t.Fatal("rule deletion failure swallowed")
	}
}
