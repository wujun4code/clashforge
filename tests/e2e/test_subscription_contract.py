#!/usr/bin/env python3
"""Execute the traffic test with a single-node, recording-only sandbox.

No real curl, mihomo, firewall tools or service commands are available in PATH.
Fixture addresses/credentials below are synthetic, never private subscription data.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]


class SubscriptionSandbox(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='cf-v11-', dir=os.environ.get('TMPDIR'))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('dirname', 'mkdir', 'grep', 'sed', 'wc', 'head', 'tail', 'cat', 'seq'):
            tool = shutil.which(name)
            if not tool:
                raise RuntimeError('missing sandbox prerequisite: ' + name)
            (self.bin / name).symlink_to(tool)
        (self.bin / 'python3').symlink_to(sys.executable)
        self.stub('sleep', '#!/bin/sh\nexit 0\n')
        self.calls = self.root / 'calls.jsonl'
        self.state = self.root / 'selected'
        self.stub('genconfig', '#!/bin/sh\n[ "$1" = -h ] && { echo overrides; exit; }\nwhile [ "$1" != -out ]; do shift; done\nshift\nprintf "proxies: []\\n" > "$1"\n')
        self.stub('mihomo', '#!' + sys.executable + '\n' + '''import os, signal, sys, time
if '-v' in sys.argv: print('Mihomo Meta fixture'); sys.exit()
if '-t' in sys.argv: print('test is successful'); sys.exit()
def stop(*args):
    open(os.environ['CF_STOPPED'], 'w').write('stopped')
    sys.exit()
signal.signal(signal.SIGTERM, stop)
print('[TCP] fixture route', flush=True)
open(os.environ['CF_STARTED'], 'w').write('started')
while True: time.sleep(.01)
''')
        self.stub('curl', '#!' + sys.executable + '\n' + '''import json, os, sys, time
from pathlib import Path
args = sys.argv[1:]
with open(os.environ['CF_CALLS'], 'a') as f: f.write(json.dumps(args)+'\\n')
url = next((a for a in reversed(args) if a.startswith('http')), '')
case = os.environ.get('CF_CASE', '')
node = os.environ.get('E2E_EXPECTED_NODE', 'Tokyo-AnyTLS')
state = Path(os.environ['CF_STATE'])
if url.endswith('/version'):
    for _ in range(200):
        if Path(os.environ['CF_STARTED']).exists(): break
        time.sleep(.005)
    print('{}')
elif url.endswith('/proxies'):
    proxies = {node: {'type': 'AnyTLS'}, '🚀 节点选择': {'type':'Selector','all':[node,'DIRECT'],'now':'DIRECT'}, 'DIRECT':{'type':'Direct'}}
    if case == 'missing': proxies.pop(node)
    if case == 'wrong-type': proxies[node]['type'] = 'Direct'
    if case == 'no-membership': proxies['🚀 节点选择']['all'] = ['DIRECT']
    print(json.dumps({'proxies':proxies}))
elif '/proxies/' in url:
    if '-X' in args and args[args.index('-X')+1] == 'PUT':
        payload = json.loads(args[args.index('-d')+1])
        if case != 'stale-readback': state.write_text(payload['name'])
        print('400' if case == 'put-fail' else '204')
    else:
        print(json.dumps({'type':'Selector','all':[node,'DIRECT'],'now':state.read_text() if state.exists() else 'DIRECT'}))
elif 'api.ipify.org' in url:
    if case == 'proxy-fail' and '--proxy' in args: sys.exit(7)
    if case == 'direct-fail' and '--proxy' not in args: sys.exit(7)
    print('203.0.113.2' if '--proxy' in args and case != 'same-ip' else '203.0.113.1')
elif 'ipinfo.io' in url:
    print(json.dumps({'country':'SG' if case == 'wrong-country' else os.environ.get('E2E_EXPECTED_COUNTRY', 'JP'), 'city':'Osaka' if case == 'wrong-city' else os.environ.get('E2E_EXPECTED_CITY', 'Tokyo')}))
else:
    print('HTTP/1.1 200 Connection established\\nTLSv1.3\\nSSL connection using TLSv1.3\\nCN=www.google.com')
''')
        source_path = Path(os.environ.get('CF_TEST_ENTRYPOINT', REPO / 'tests/docker/entrypoint.sh'))
        source = source_path.read_text()
        # Filesystem roots only: execute the real entrypoint logic and EXIT trap.
        source = source.replace('/var/run/metaclash', str(self.root / 'runtime')).replace('/testdata/sample-clash-config.yaml', str(self.root / 'input.yaml')).replace('/tmp/mihomo.log', str(self.root / 'mihomo.log'))
        self.script = self.root / 'entrypoint.sh'
        self.script.write_text(source)
        shutil.copy(REPO / 'tests/docker/subscription_contract.py', self.root)
        (self.root / 'input.yaml').write_text('# synthetic fixture; genconfig is a recording stub\n')

    def stub(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o755)

class SubscriptionTrafficContract(SubscriptionSandbox):
    def run_case(self, case='', **overrides):
        env = dict(os.environ, PATH=str(self.bin), CF_CALLS=str(self.calls), CF_STATE=str(self.state), CF_CASE=case, CF_STARTED=str(self.root/'started'), CF_STOPPED=str(self.root/'stopped'))
        for name in ('E2E_EXPECTED_NODE', 'E2E_EXPECTED_TYPE', 'E2E_EXPECTED_COUNTRY', 'E2E_EXPECTED_CITY'):
            env.pop(name, None)
        env.update(overrides)
        result = subprocess.run(['/bin/bash', str(self.script)], env=env, text=True, capture_output=True, timeout=15)
        self.assertTrue((self.root/'stopped').exists(), 'EXIT trap must stop the started core')
        calls = [json.loads(x) for x in self.calls.read_text().splitlines()]
        return result, calls

    def test_single_anytls_tokyo_runs_proxy_tls_and_cleanup(self):
        result, calls = self.run_case()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('ALL TESTS PASSED', result.stdout)
        self.assertEqual(self.state.read_text(), 'Tokyo-AnyTLS')
        writes = [a for a in calls if 'PUT' in a]
        self.assertEqual(len(writes), 1)
        self.assertIn('%F0%9F%9A%80%20', ' '.join(writes[0]))
        self.assertTrue(any('--proxy' in a and 'https://api.ipify.org' in a for a in calls))
        self.assertTrue(any('--proxy' in a and 'https://www.google.com' in a for a in calls))

    def test_overrides_still_require_real_node_route(self):
        result, _ = self.run_case(E2E_EXPECTED_NODE='Synthetic-AnyTLS', E2E_EXPECTED_COUNTRY='US', E2E_EXPECTED_CITY='Synthetic City')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.state.read_text(), 'Synthetic-AnyTLS')

    def test_missing_node_fails(self):
        self.assertNotEqual(self.run_case('missing')[0].returncode, 0)

    def test_wrong_type_fails(self):
        self.assertNotEqual(self.run_case('wrong-type')[0].returncode, 0)

    def test_selector_without_node_fails(self):
        self.assertNotEqual(self.run_case('no-membership')[0].returncode, 0)

    def test_failed_put_fails(self):
        self.assertNotEqual(self.run_case('put-fail')[0].returncode, 0)

    def test_stale_readback_fails(self):
        self.assertNotEqual(self.run_case('stale-readback')[0].returncode, 0)

    def test_direct_exit_fails(self):
        self.assertNotEqual(self.run_case('same-ip')[0].returncode, 0)

    def test_failed_proxy_fails(self):
        self.assertNotEqual(self.run_case('proxy-fail')[0].returncode, 0)

    def test_failed_baseline_fails(self):
        self.assertNotEqual(self.run_case('direct-fail')[0].returncode, 0)

    def test_wrong_country_fails(self):
        self.assertNotEqual(self.run_case('wrong-country')[0].returncode, 0)

    def test_wrong_city_fails(self):
        self.assertNotEqual(self.run_case('wrong-city')[0].returncode, 0)


class OpenWrtSubscriptionContract(SubscriptionSandbox):
    # Execute the real subscription prefix, optionally through SL-03/readback.
    # Stop before select-proxy and the network/lifecycle tail on this host.
    def run_subscription(self, case='', reuse=False, launch=False):
        self.stub('jsonfilter', '#!' + sys.executable + '\n' + '''import json, re, sys
try:
    data=json.load(sys.stdin)
    expr=sys.argv[-1]
    if expr == '@.ok': values=[data.get('ok')]
    elif expr == '@.data.status': values=[data['data'].get('status')]
    elif expr == '@.data.id': values=[data['data'].get('id')]
    elif expr == '@.data.active_source.type': values=[data['data']['active_source'].get('type')]
    elif expr == '@.data.active_source.sub_id': values=[data['data']['active_source'].get('sub_id')]
    elif expr == '@.data.nodes[*].name': values=[n['name'] for n in data['data']['nodes']]
    else:
        match=re.fullmatch(r'@\\.data\\.nodes\\[@\\.name="(.*)"\\]\\.type', expr)
        if not match: sys.exit(1)
        values=[n['type'] for n in data['data']['nodes'] if n['name'] == match[1]]
    for value in values:
        if value is not None: print('true' if value is True else 'false' if value is False else value)
except (ValueError, KeyError, TypeError): sys.exit(1)
''')
        self.stub('curl', '#!' + sys.executable + '\n' + '''import json, os, sys
from pathlib import Path
args=sys.argv[1:]
url=args[-1]
with open(os.environ['CF_CALLS'], 'a') as f: f.write(json.dumps(args)+'\\n')
case=os.environ['CF_CASE']
if url.endswith('/sync-update'):
    if case == 'malformed': print('not-json')
    else: print(json.dumps({'ok':case != 'sync-error','data':{'status':'ok'}}))
elif url.endswith('/core/apply'):
    payload=json.loads(args[args.index('-d')+1])
    selected=payload.get('source')
    valid=selected == {'type':'sub_id','sub_id':'synthetic-sub','sync':False}
    if valid and case != 'apply-fail':
        Path(os.environ['CF_STATE']).write_text('synthetic-sub')
        print('data: {"type":"done","success":true}')
    else:
        print('data: {"type":"done","success":false,"error":"synthetic-private-marker"}')
elif url.endswith('/setup/launch'):
    print('data: {"type":"done","success":true}')
elif url.endswith('/config/active-source'):
    active={'type':'subscription','sub_id':Path(os.environ['CF_STATE']).read_text() if Path(os.environ['CF_STATE']).exists() else 'missing'}
    if case == 'readback-id': active['sub_id']='other-sub'
    if case == 'readback-type': active['type']='file'
    if case == 'readback-null': active=None
    if case == 'readback-malformed': print('not-json')
    else: print(json.dumps({'ok':case != 'readback-error','data':{'active_source':active},'private':'synthetic-private-marker'}))
elif url.endswith('/nodes'):
    nodes=[{'name':'Tokyo-AnyTLS','type':'anytls'}]
    if case == 'empty': nodes=[]
    if case == 'wrong-type': nodes[0]['type']='direct'
    if case == 'missing': nodes[0]['name']='Synthetic-Other'
    if case == 'mixed-types': nodes=[{'name':'Tokyo-AnyTLS','type':'direct'},{'name':'Synthetic-Other','type':'anytls'}]
    print(json.dumps({'ok':case != 'nodes-error','data':{'nodes':nodes}}))
else: print(json.dumps({'ok':True,'data':{'id':'synthetic-sub'}}, separators=(',', ':')))
''')
        snapshot = self.root/'snapshot'
        snapshot.mkdir()
        (snapshot/'sub-id').write_text('synthetic-sub')
        source = (REPO/'tests/e2e/vm/setup-launch.sh').read_text()
        source = source.split('# Explicitly establish and read back' if launch else '# ── SL-03: setup/launch')[0]
        if launch and case in ('missing-source', 'wrong-subid'):
            lines=source.splitlines()
            for index, line in enumerate(lines):
                if line.startswith('LAUNCH_PAYLOAD='):
                    if case == 'missing-source':
                        lines[index]=line.replace('\\"source\\":{\\"type\\":\\"sub_id\\",\\"sub_id\\":\\"${SUB_ID}\\",\\"sync\\":false},', '')
                    else:
                        lines[index]=line.replace('${SUB_ID}', 'wrong-sub')
            source='\n'.join(lines)
        source = source.replace('/tmp/cf-snapshot', str(snapshot))
        source += ('\nprintf "AFTER_LAUNCH\\n"\n' if launch else '')
        source += '\nprintf "SUBSCRIPTION_PREFIX_COMPLETED\\n"\n'
        script = self.root/'subscription.sh'
        script.write_text(source)
        env = dict(os.environ, PATH=str(self.bin), CF_CALLS=str(self.calls), CF_CASE=case, CF_STATE=str(self.state), SUBSCRIPTION_URL='https://subscription.example.invalid/synthetic', REUSE_SUBSCRIPTION='true' if reuse else 'false', E2E_EXPECTED_NODE='Tokyo-AnyTLS', E2E_EXPECTED_TYPE='anytls')
        return subprocess.run(['/bin/sh', str(script)], env=env, capture_output=True, text=True, timeout=10)

    def test_openwrt_single_node_sync(self):
        result=self.run_subscription()
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn('SUBSCRIPTION_PREFIX_COMPLETED', result.stdout)

    def test_openwrt_reuse_checks_actual_cache(self):
        result=self.run_subscription(reuse=True)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn('/nodes', self.calls.read_text())
        self.assertNotIn('/sync-update', self.calls.read_text())

    def test_openwrt_failed_sync_is_not_success(self):
        self.assertNotEqual(self.run_subscription('sync-error').returncode, 0)

    def test_openwrt_empty_cache_fails_on_reuse(self):
        self.assertNotEqual(self.run_subscription('empty', reuse=True).returncode, 0)

    def test_openwrt_malformed_json_fails(self):
        self.assertNotEqual(self.run_subscription('malformed').returncode, 0)

    def test_openwrt_missing_expected_node_fails(self):
        self.assertNotEqual(self.run_subscription('missing').returncode, 0)

    def test_openwrt_wrong_transport_fails(self):
        self.assertNotEqual(self.run_subscription('wrong-type').returncode, 0)

    def test_openwrt_type_must_belong_to_expected_node(self):
        self.assertNotEqual(self.run_subscription('mixed-types').returncode, 0)

    def test_openwrt_error_cache_fails(self):
        self.assertNotEqual(self.run_subscription('nodes-error').returncode, 0)

    def test_launch_fresh_selects_exact_subscription(self):
        self.assert_launch_payload(False)

    def test_launch_reuse_selects_exact_subscription_without_sync(self):
        self.assert_launch_payload(True)

    def assert_launch_payload(self, reuse):
        result=self.run_subscription(reuse=reuse, launch=True)
        self.assertEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertIn('AFTER_LAUNCH', result.stdout)
        calls=[json.loads(line) for line in self.calls.read_text().splitlines()]
        apply=[call for call in calls if call[-1].endswith('/core/apply')]
        self.assertEqual(len(apply), 1)
        self.assertEqual(json.loads(apply[0][apply[0].index('-d')+1]), {
            'source':{'type':'sub_id','sub_id':'synthetic-sub','sync':False},
            'dns':{'enable':True,'mode':'fake-ip','dnsmasq_mode':'upstream','apply_on_start':True},
            'network':{'mode':'tproxy','firewall_backend':'nftables','bypass_lan':True,'bypass_china':False,'apply_on_start':True}})
        self.assertEqual(calls[-1][-1], 'http://127.0.0.1:7777/api/v1/config/active-source')
        self.assertEqual(sum(call[-1].endswith('/sync-update') for call in calls), 0 if reuse else 1)

    def assert_launch_failure(self, case):
        result=self.run_subscription(case, launch=True)
        self.assertNotEqual(result.returncode, 0, result.stdout+result.stderr)
        self.assertNotIn('AFTER_LAUNCH', result.stdout)
        self.assertNotIn('synthetic-private-marker', result.stdout+result.stderr)
        self.assertNotIn('https://subscription.example.invalid/synthetic', result.stdout+result.stderr)
        if case in ('missing-source', 'wrong-subid'):
            calls=[json.loads(line) for line in self.calls.read_text().splitlines()]
            apply=[call for call in calls if call[-1].endswith('/core/apply')]
            self.assertEqual(len(apply), 1)
            payload=json.loads(apply[0][apply[0].index('-d')+1])
            if case == 'missing-source': self.assertNotIn('source', payload)
            else: self.assertEqual(payload['source']['sub_id'], 'wrong-sub')

    def test_launch_missing_source_fails(self): self.assert_launch_failure('missing-source')
    def test_launch_wrong_subid_fails(self): self.assert_launch_failure('wrong-subid')
    def test_launch_apply_failure_fails(self): self.assert_launch_failure('apply-fail')
    def test_launch_readback_wrong_id_fails(self): self.assert_launch_failure('readback-id')
    def test_launch_readback_wrong_type_fails(self): self.assert_launch_failure('readback-type')
    def test_launch_readback_null_fails(self): self.assert_launch_failure('readback-null')
    def test_launch_readback_false_ok_fails(self): self.assert_launch_failure('readback-error')
    def test_launch_readback_malformed_fails(self): self.assert_launch_failure('readback-malformed')


if __name__ == '__main__':
    unittest.main(verbosity=2)
