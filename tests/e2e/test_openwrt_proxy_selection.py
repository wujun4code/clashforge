"""Run the VM's actual selector helper against a loopback controller facade.

Set JSONFILTER to an OpenWrt jsonfilter executable for native-parser coverage.
The default recording JSON-path shim keeps the test runnable on dev hosts.
No host firewall, routing, services, subscriptions or upstream routers are used.
"""
import copy
import http.server
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import threading
import unittest
import urllib.parse

REPO = Path(__file__).resolve().parents[2]
HELPER = REPO / 'tests/e2e/vm/select-proxy.sh'
NODE = 'Tokyo-AnyTLS'
MAIN = '🚀 节点选择 % "quoted"'
ROOT = '🐟 漏网之鱼'
SHIM = '''#!/usr/bin/env python3
import json,re,sys
try:
 a=sys.argv; d=json.load(open(a[a.index('-i')+1])); e=a[a.index('-e')+1]
 if e=='@.ok': out=[d.get('ok')]
 elif e=='@.data.content': out=[d['data']['content']]
 elif e=='@.data.ports.mihomo_api': out=[d['data']['ports']['mihomo_api']]
 elif e=='@.mode': out=[d['mode']]
 elif e=='@.rules[@.type="Match"].proxy': out=[r['proxy'] for r in d['rules'] if r['type']=='Match']
 elif e=='@.data.proxies[*].name': out=[x['name'] for x in d['data']['proxies'].values()]
 elif e in ('@.group','@.name'): out=[d[e[2:]]]
 else:
  m=re.fullmatch(r'@\\.data\\.proxies\\[("(?:[^"\\\\]|\\\\.)*")\\]\\.(type|now|all\\[\\*\\])',e)
  x=d['data']['proxies'][json.loads(m[1])]; k=m[2]; out=x['all'] if k=='all[*]' else [x.get(k)]
 for x in out:
  if x is not None: print('true' if x is True else 'false' if x is False else x)
except Exception: sys.exit(1)
'''


class SelectionTest(unittest.TestCase):
    def run_case(self, case='', generated=False, caller=None, limited_tr=False, minimal_tools=False):
        main, root = ('Proxy', 'Final') if generated else (MAIN, ROOT)
        proxies = {
            NODE: {'name': NODE, 'type': 'AnyTLS'},
            main: {'name': main, 'type': 'Selector', 'all': ['DIRECT', NODE], 'now': 'DIRECT'},
            root: {'name': root, 'type': 'Selector', 'all': [main, '🎯 全球直连'], 'now': '🎯 全球直连'},
            '🎯 全球直连': {'name': '🎯 全球直连', 'type': 'Selector', 'all': ['DIRECT', main], 'now': 'DIRECT'},
            'GLOBAL': {'name': 'GLOBAL', 'type': 'Selector', 'all': ['DIRECT', NODE], 'now': 'DIRECT'},
            'Unrelated': {'name': 'Unrelated', 'type': 'Selector', 'all': ['DIRECT', NODE], 'now': 'DIRECT'},
        }
        if case == 'missing': del proxies[NODE]
        if case == 'wrong-protocol': proxies[NODE]['type'] = 'Direct'
        if case == 'no-membership': proxies[main]['all'] = ['DIRECT']
        if case == 'cycle': proxies[main]['all'] = [root]
        if case == 'broken-root': proxies[root]['all'] = ['DIRECT']
        events = []
        state = {'puts': 0, 'gets': 0}
        class Handler(http.server.BaseHTTPRequestHandler):
            def log_message(self, format, *args): pass
            def reply(self, data, status=200):
                self.send_response(status); self.end_headers()
                self.wfile.write(json.dumps(data, ensure_ascii=False).encode())
            def do_GET(self):
                events.append(('GET', self.path))
                if self.path.endswith('/config'):
                    return self.reply({'ok': True, 'data': {'ports': {'mihomo_api': 'invalid' if case == 'bad-port' else server.server_port}, 'secret': 'synthetic-private-marker'}})
                if self.path == '/configs':
                    if case == 'controller-failed': return self.reply({}, 500)
                    return self.reply({'mode': 'global' if case == 'global-mode' else 'rule'})
                if self.path == '/rules':
                    rules = [{'type': 'Match', 'proxy': 'DIRECT' if case == 'direct-match' else root}]
                    if case == 'missing-match': rules = []
                    if case == 'multiple-match': rules.append({'type': 'Match', 'proxy': 'Unrelated'})
                    return self.reply({'rules': rules})
                if self.path.endswith('/config/mihomo'):
                    target = 'DIRECT' if case == 'direct-match' else root
                    mode = 'global' if case == 'global-mode' else 'rule'
                    return self.reply({'ok': True, 'data': {'content': f'mode: {mode}\nsecret: synthetic-private-marker\nrules:\n  - MATCH,{target}\n'}})
                state['gets'] += 1
                payload = copy.deepcopy(proxies)
                if case == 'leaf-changed' and state['gets'] > 2: payload[NODE]['type'] = 'Direct'
                if case == 'broken-final' and state['gets'] > 2: payload[root]['now'] = 'DIRECT'
                if case == 'malformed':
                    self.send_response(200); self.end_headers(); self.wfile.write(b'not json'); return
                self.reply({'ok': case != 'get-error', 'data': {'proxies': payload}})
            def do_PUT(self):
                name = urllib.parse.unquote(self.path.split('/proxies/')[1].rsplit('/select', 1)[0])
                data = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                events.append(('PUT', name, data))
                state['puts'] += 1
                if case == 'put-failed': return self.reply({'ok': False}, 500)
                if case not in ('stale', 'wrapped-empty-error'):
                    proxies[name]['now'] = data['name']
                # Faithfully model the facade's misleading wrapped success.
                self.reply({'ok': True, 'data': {'ok': True}})
        with tempfile.TemporaryDirectory(prefix='cf-selection-', dir=os.environ.get('TMPDIR')) as tmp:
            bindir = Path(tmp) / 'bin'; bindir.mkdir()
            native = os.environ.get('JSONFILTER') or shutil.which('jsonfilter')
            if native: (bindir / 'jsonfilter').symlink_to(native)
            else:
                shim = bindir / 'jsonfilter'; shim.write_text(SHIM); shim.chmod(0o700)
            if limited_tr:
                # OpenWrt 23.05.5's BusyBox tr silently treats POSIX character
                # classes literally. Observed on the actual E2E VM: AnyTLS
                # stays AnyTLS with [:upper:]/[:lower:], but A-Z/a-z works.
                tr = bindir / 'tr'
                tr.write_text('''#!/usr/bin/env python3
import sys
data = sys.stdin.buffer.read()
args = sys.argv[1:]
if args == ['[:upper:]', '[:lower:]']:
    pass
elif args == ['A-Z', 'a-z']:
    data = data.translate(bytes.maketrans(b'ABCDEFGHIJKLMNOPQRSTUVWXYZ', b'abcdefghijklmnopqrstuvwxyz'))
else:
    raise SystemExit('unexpected tr invocation in selector fixture')
sys.stdout.buffer.write(data)
''')
                tr.chmod(0o700)
            if minimal_tools:
                # The actual OpenWrt image has no od. Do not inherit the
                # developer host's larger utility set and hide that gap.
                for tool in ['python3', 'curl', 'tr', 'sed', 'grep', 'cp', 'cmp', 'rm', 'mktemp', 'awk', 'sh']:
                    if not (bindir / tool).exists():
                        executable = shutil.which(tool)
                        assert executable is not None, tool
                        (bindir / tool).symlink_to(executable)
            server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
            thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
            script = HELPER
            if caller is not None:
                # Execute the actual caller's selection block, not a rewritten
                # condition. Other lifecycle/network setup is outside this unit.
                source = (REPO / f'tests/e2e/vm/setup-launch{caller}.sh').read_text()
                block = source[source.index('if CF_API="$CF_API" sh '):]
                block = block[:block.index('\nsleep 5' if caller else '\n# 等待接管生效')]
                script = Path(tmp) / 'caller.sh'
                script.write_text('set -e\nFAILED=0\nrecord() { [ "$1" != FAIL ] || FAILED=1; return 0; }\n' + block + '\nexit "$FAILED"\n')
                (Path(tmp) / 'select-proxy.sh').symlink_to(HELPER)
            try:
                result = subprocess.run(['/bin/sh', str(script)], env=dict(os.environ,
                    PATH=str(bindir) if minimal_tools else str(bindir) + os.pathsep + os.environ['PATH'], TMPDIR=tmp,
                    CF_API=f'http://127.0.0.1:{server.server_port}/api/v1'),
                    capture_output=True, text=True, timeout=20)
            finally:
                server.shutdown(); server.server_close(); thread.join()
            if caller is not None:
                script.unlink()
                (Path(tmp) / 'select-proxy.sh').unlink()
            self.assertEqual(sorted(p.name for p in Path(tmp).iterdir()), ['bin'], 'private response scratch leaked')
            self.assertNotIn('synthetic-private-marker', result.stdout + result.stderr)
        return result, events, proxies, main, root

    def test_success_closes_emoji_percent_quote_rule_chain(self):
        result, events, proxies, main, root = self.run_case()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('MATCH chain read back', result.stdout)
        self.assertEqual(proxies[main]['now'], NODE)
        self.assertEqual(proxies[root]['now'], main)
        self.assertEqual(proxies['GLOBAL']['now'], 'DIRECT')
        self.assertEqual(proxies['Unrelated']['now'], 'DIRECT')
        self.assertEqual(proxies['🎯 全球直连']['now'], 'DIRECT')
        puts = [i for i, e in enumerate(events) if e[0] == 'PUT']
        self.assertEqual(len(puts), 2)
        for i in puts: self.assertEqual(events[i + 1][0], 'GET')
        self.assertEqual(events[-1][0], 'GET')

    def test_busybox_tr_without_classes_selects_real_anytls(self):
        result, events, proxies, main, root = self.run_case(limited_tr=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(proxies[main]['now'], NODE)
        self.assertEqual(proxies[root]['now'], main)
        for group in ['GLOBAL', 'Unrelated', '🎯 全球直连']:
            self.assertEqual(proxies[group]['now'], 'DIRECT')
        self.assertEqual(len([e for e in events if e[0] == 'PUT']), 2)

    def test_busybox_tr_without_classes_rejects_wrong_protocol(self):
        result, events, *_ = self.run_case('wrong-protocol', limited_tr=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('expected AnyTLS node missing or wrong protocol', result.stderr)
        self.assertFalse(any(e[0] == 'PUT' for e in events))

    def test_busybox_tr_without_classes_checks_final_leaf(self):
        result, *_ = self.run_case('leaf-changed', limited_tr=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('final leaf protocol changed', result.stderr)

    def test_minimal_openwrt_tools_without_od_preserve_utf8_selection(self):
        result, events, proxies, main, root = self.run_case(limited_tr=True, minimal_tools=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(proxies[main]['now'], NODE)
        self.assertEqual(proxies[root]['now'], main)
        self.assertEqual(proxies['🎯 全球直连']['now'], 'DIRECT')
        self.assertEqual(len([e for e in events if e[0] == 'PUT']), 2)

    def test_generated_proxy_final_chain(self):
        result, _, proxies, main, root = self.run_case(generated=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(proxies[root]['now'], main)
        self.assertEqual(proxies[main]['now'], NODE)

    def test_missing_node(self): self.assert_failure('missing')
    def test_wrong_protocol(self): self.assert_failure('wrong-protocol')
    def test_no_membership(self): self.assert_failure('no-membership')
    def test_cycle(self): self.assert_failure('cycle')
    def test_broken_root(self): self.assert_failure('broken-root')
    def test_direct_match(self): self.assert_failure('direct-match')
    def test_global_mode(self): self.assert_failure('global-mode')
    def test_bad_controller_port(self): self.assert_failure('bad-port')
    def test_controller_failure(self): self.assert_failure('controller-failed')
    def test_missing_match(self): self.assert_failure('missing-match')
    def test_multiple_matches(self): self.assert_failure('multiple-match')
    def test_put_failed(self): self.assert_failure('put-failed')
    def test_stale_readback(self): self.assert_failure('stale')
    def test_wrapped_empty_error(self): self.assert_failure('wrapped-empty-error')
    def test_leaf_changed(self): self.assert_failure('leaf-changed')
    def test_broken_final(self): self.assert_failure('broken-final')
    def test_malformed(self): self.assert_failure('malformed')
    def test_get_error(self): self.assert_failure('get-error')

    def test_tproxy_caller_stale_is_hard_failure(self):
        result, *_ = self.run_case('stale', caller='')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_tun_caller_stale_is_hard_failure(self):
        result, *_ = self.run_case('stale', caller='-tun')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_tun_caller_selection_success(self):
        result, *_ = self.run_case(caller='-tun')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def assert_failure(self, case):
        result, _, _, _, _ = self.run_case(case)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('FAIL proxy selection:', result.stderr)

    def test_callers_select_before_warmup(self):
        source = (REPO / 'tests/e2e/vm/setup-launch.sh').read_text()
        self.assertIn('/select-proxy.sh', source)
        self.assertLess(source.index('/select-proxy.sh'), source.index('# ── 代理连接预热'))
        self.assertIn('exit 1', source[source.index('/select-proxy.sh'):source.index('# 等待接管生效')])
        tun = (REPO / 'tests/e2e/vm/setup-launch-tun.sh').read_text()
        self.assertIn('/select-proxy.sh', tun)


if __name__ == '__main__':
    unittest.main(verbosity=2)
