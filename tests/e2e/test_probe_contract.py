#!/usr/bin/env python3
"""Exercise the real VM probe with recording curl/DNS fakes, never a network."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]

class ProbeContract(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='cf-probe-', dir=os.environ.get('TMPDIR'))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('awk', 'sed', 'grep', 'head', 'tr', 'cat', 'cut', 'date', 'wc'):
            tool = shutil.which(name)
            if not tool:
                raise RuntimeError(f'missing sandbox prerequisite: {name}')
            (self.bin / name).symlink_to(tool)
        self.snapshot = self.root / 'snapshot'
        self.snapshot.mkdir()
        (self.snapshot / 'direct-ip').write_text('203.0.113.1\n')
        self.summary = self.root / 'summary'
        self.calls = self.root / 'calls'
        curl = self.bin / 'curl'
        curl.write_text('#!' + sys.executable + '\n' + '''import json, os, sys
args = sys.argv[1:]
with open(os.environ['CF_PROBE_CALLS'], 'a') as f: f.write(json.dumps(args)+'\\n')
url = args[-1]
if 'api.ipify.org' in url:
    if os.environ.get('CF_PROBE_FAIL'): sys.exit(7)
    print('203.0.113.2' if '--proxy' in args else '203.0.113.1')
elif url.endswith('/config'): print('{}')
elif '-w' in args:
    if os.environ.get('CF_PROBE_FAIL'): sys.exit(7)
    print('200:0.01' if ':' in args[args.index('-w')+1] else '200')
''')
        curl.chmod(0o755)
        dns = self.bin / 'nslookup'
        dns.write_text('#!/bin/sh\nprintf "Server: 127.0.0.1\\nAddress: 127.0.0.1#53\\nName: fixture\\nAddress: 198.18.0.2\\n"\n')
        dns.chmod(0o755)
        # Rebase only the script's snapshot filesystem root; logic and argv are
        # unchanged. No production file rename/static grep substitutes execution.
        source = (REPO / 'tests/e2e/vm/probe.sh').read_text()
        self.script = self.root / 'probe.sh'
        self.script.write_text(source.replace('/tmp/cf-snapshot', str(self.snapshot)))

    def run_probe(self, phase='running', url='', fail=False, auth=''):
        env = dict(os.environ, PATH=str(self.bin), PHASE=phase, PROXY_NODE_IP='203.0.113.2', MIXED_PORT_URL=url, MIXED_PORT='7893', PROXY_AUTH=auth, GITHUB_STEP_SUMMARY=str(self.summary), CF_PROBE_CALLS=str(self.calls))
        if fail: env['CF_PROBE_FAIL']='1'
        result = subprocess.run(['/bin/sh', str(self.script)], env=env, text=True, capture_output=True, timeout=20)
        calls = [json.loads(line) for line in self.calls.read_text().splitlines()]
        text = result.stdout + self.summary.read_text()
        return result, calls, text

    def assert_explicit_only(self, calls, text, url):
        public = [args for args in calls if args[-1].startswith('https://')]
        self.assertGreaterEqual(len(public), 13)
        for args in public:
            self.assertIn('--proxy', args, args)
            self.assertEqual(args[args.index('--proxy')+1], url)
        self.assertIn('显式 mixed 代理', text)
        self.assertIn('不证明 LAN 透明接管或 rule-mode 分流策略', text)
        for unsupported in ('tproxy 透明拦截', 'tproxy 未生效', '国内直连 + 海外/AI 走代理'):
            self.assertNotIn(unsupported, text)

    def test_running_default_mixed_is_explicit(self):
        result, calls, text = self.run_probe()
        self.assertEqual(result.returncode, 0, result.stderr + text)
        self.assert_explicit_only(calls, text, 'http://127.0.0.1:7893')

    def test_running_configured_proxy_and_auth(self):
        result, calls, text = self.run_probe(url='http://127.0.0.1:17893', auth='fixture:password')
        self.assertEqual(result.returncode, 0, result.stderr + text)
        self.assert_explicit_only(calls, text, 'http://127.0.0.1:17893')
        for args in calls:
            if '--proxy' in args:
                self.assertEqual(args[args.index('--proxy-user')+1], 'fixture:password')

    def test_running_failures_are_not_transparent_claims(self):
        result, calls, text = self.run_probe(url='http://127.0.0.1:7893', fail=True)
        self.assertNotEqual(result.returncode, 0)
        self.assert_explicit_only(calls, text, 'http://127.0.0.1:7893')
        self.assertIn('FAIL', text)

    def test_baseline_and_stopped_do_not_use_explicit_proxy(self):
        for phase in ('baseline', 'stopped'):
            result, calls, text = self.run_probe(phase=phase)
            self.assertEqual(result.returncode, 0, result.stderr + text)
            self.assertTrue(calls)
            self.assertTrue(all('--proxy' not in args for args in calls))

if __name__ == '__main__':
    unittest.main(verbosity=2)
