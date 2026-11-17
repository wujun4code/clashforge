#!/usr/bin/env python3
"""Parse workflow graphs/contracts with PyYAML; actionlint is a separate gate."""
from pathlib import Path
import shlex
import re
import subprocess
import unittest
import yaml
import os
import tempfile

ROOT = Path(__file__).resolve().parents[2]

def workflow(name):
    return yaml.safe_load((ROOT / '.github/workflows' / name).read_text())

def runs(job):
    return '\n'.join(step.get('run', '') for step in job['steps'])

class WorkflowContracts(unittest.TestCase):
    def test_openwrt_installs_full_iproute2_even_with_nodeps(self):
        for name in ('release.yml', 'e2e-openwrt.yml'):
            self.assertIn('Depends: ca-bundle, ip-full', (ROOT / '.github/workflows' / name).read_text())
        text = (ROOT / '.github/workflows/e2e-openwrt.yml').read_text()
        install = 'opkg update && opkg install ip-full && ip -N -d -o rule show >/dev/null'
        self.assertIn('_ssh "' + install + '"', text)
        self.assertLess(text.index(install), text.index('opkg install --nodeps --force-downgrade'))
        self.assertNotIn(install + ' || true', text)

    def test_openwrt_installs_tun_dns_probe_before_scenarios(self):
        steps = workflow('e2e-openwrt.yml')['jobs']['e2e-openwrt']['steps']
        install = next(i for i, step in enumerate(steps) if step.get('id') == 'install')
        script = steps[install]['run']
        required = '_ssh "opkg install bind-dig && command -v dig >/dev/null && dig -v"'
        self.assertIn(required, script)
        self.assertNotIn(required + ' || true', script)
        scenario = next(i for i, step in enumerate(steps)
                        if step.get('uses') == './.github/actions/e2e-scenario')
        self.assertLess(install, scenario)
        tun = next(step for step in steps if 'TUN scenario requires bind-dig' in step.get('run', ''))
        self.assertIn('command -v dig >/dev/null ||', tun['run'])
        self.assertNotIn('continue-on-error', steps[install])
        self.assertNotIn('continue-on-error', tun)

    def test_openwrt_provisions_real_tun_before_scenarios(self):
        steps = workflow('e2e-openwrt.yml')['jobs']['e2e-openwrt']['steps']
        install = next(i for i, s in enumerate(steps) if s.get('id') == 'install')
        script = steps[install]['run']
        commands = [line.strip() for line in script.splitlines()
                    if 'opkg install kmod-tun' in line]
        self.assertEqual(len(commands), 1, 'install must provision the real TUN driver')
        command = commands[0]
        tokens = shlex.split(command)
        self.assertEqual(tokens[0], '_ssh')
        self.assertEqual(len(tokens), 2)
        self.assertIn('opkg install kmod-tun && modprobe tun && test -c /dev/net/tun', tokens[1])
        self.assertIn('/sys/class/misc/tun/dev', tokens[1])
        self.assertIn('10:200', tokens[1])
        self.assertNotIn('|| true', command)
        self.assertNotIn('continue-on-error', steps[install])
        self.assertIn('-p 2222 root@127.0.0.1', script)
        self.assertLess(script.index(command), script.index('opkg install --nodeps'))
        scenario = next(i for i, s in enumerate(steps)
                        if s.get('uses') == './.github/actions/e2e-scenario')
        self.assertLess(install, scenario)
        tun = next(s for s in steps if 'TUN driver/device missing' in s.get('run', ''))
        self.assertIn('test -c /dev/net/tun ||', tun['run'])
        self.assertNotIn('continue-on-error', tun)
        self.assertNotIn('if', tun)
        self.assertNotRegex(command, r'\b(?:mknod|ln)\b')

    def test_openwrt_tun_provision_failures_stop_install(self):
        steps = workflow('e2e-openwrt.yml')['jobs']['e2e-openwrt']['steps']
        script = next(s['run'] for s in steps if s.get('id') == 'install')
        commands = [line.strip() for line in script.splitlines()
                    if 'opkg install kmod-tun' in line]
        self.assertEqual(len(commands), 1, 'install must provision the real TUN driver')
        command = commands[0]
        # Recording-only shell functions: never install/load/create on the host.
        sandbox = r'''
_ssh() { eval "$1"; }
opkg() { echo opkg; builtin test "$FAIL" != opkg; }
modprobe() { echo modprobe; builtin test "$FAIL" != modprobe; }
cat() {
    builtin test "$1" = /sys/class/misc/tun/dev || return 98
    if builtin test "$FAIL" = cat; then echo 10:200; return 1; fi
    if builtin test "$FAIL" = identity; then echo 1:3; else echo 10:200; fi
}
test() {
    case "$1" in
        -c) echo device; builtin test "$2" = /dev/net/tun && builtin test "$FAIL" != device ;;
        -r) builtin test "$2" = /sys/class/misc/tun/dev && builtin test "$FAIL" != no_sysfs ;;
        *) builtin test "$@" ;;
    esac
}
'''
        for failure in ('none', 'opkg', 'modprobe', 'device', 'cat', 'identity', 'no_sysfs'):
            with self.subTest(failure=failure):
                result = subprocess.run(['bash', '-e', '-c', sandbox + command + '\necho reached-install'],
                    env={**os.environ, 'FAIL': failure}, capture_output=True, text=True)
                success = failure in ('none', 'no_sysfs')
                self.assertEqual(result.returncode == 0, success, result.stderr)
                self.assertEqual('reached-install' in result.stdout, success)
                if failure == 'opkg':
                    self.assertNotIn('modprobe', result.stdout)
                if failure in ('opkg', 'modprobe'):
                    self.assertNotIn('device', result.stdout)

    def test_flutter_sdk_download_is_scoped_http1_and_cached(self):
        data = workflow('android-ci.yml')
        for name in ('test', 'e2e'):
            steps = data['jobs'][name]['steps']
            setup = next(i for i, s in enumerate(steps) if s.get('uses', '').startswith('subosito/flutter-action@'))
            self.assertTrue(any('.github/scripts/configure-flutter-curl.sh' in s.get('run', '') for s in steps[:setup]))
            self.assertEqual(steps[setup]['env']['CURL_HOME'], '${{ env.FLUTTER_CURL_HOME }}')
            self.assertTrue(steps[setup]['with']['cache'])
        with tempfile.TemporaryDirectory() as tmp:
            envfile = Path(tmp) / 'github-env'
            result = subprocess.run(['sh', str(ROOT / '.github/scripts/configure-flutter-curl.sh')],
                env={**os.environ, 'RUNNER_TEMP': tmp, 'GITHUB_ENV': str(envfile)}, capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            curl_home = Path(envfile.read_text().strip().split('=', 1)[1])
            self.assertTrue(curl_home.is_relative_to(tmp))
            self.assertEqual((curl_home / '.curlrc').read_text(), 'http1.1\n')
            # Do not add retry-all-errors to curl's redirected stdout download:
            # partial SDK bytes could be concatenated across retries.
            self.assertNotIn('retry-all-errors', (curl_home / '.curlrc').read_text())

    def test_vm_workflow_never_executes_host_firewall_commands(self):
        # bash -nv expands no commands; require firewall tokens to occur only
        # inside the quoted _ssh payload, never as host commands or substitutions.
        files = [ROOT / '.github/workflows/e2e-openwrt.yml', ROOT / '.github/actions/e2e-scenario/action.yml']
        vm_firewall = []
        for path in files:
            data = yaml.safe_load(path.read_text())
            steps = data.get('runs', {}).get('steps', [])
            steps += [s for j in data.get('jobs', {}).values() for s in j.get('steps', [])]
            for step in steps:
                script = step.get('run', '')
                if not script:
                    continue
                script = re.sub(r'\$\{\{.*?\}\}', 'EXPRESSION', script)
                syntax = subprocess.run(['/bin/bash', '-n'], input=script, text=True, capture_output=True)
                self.assertEqual(syntax.returncode, 0, syntax.stderr)
                # shlex keeps quoted remote command bodies intact; separators
                # stay separate, so sudo/env or line-wrapping cannot hide a host
                # iptables/nft executable. Inspect substitutions separately too.
                tokens = list(shlex.shlex(script, posix=True, punctuation_chars=True))
                for token in tokens:
                    if re.fullmatch(r'(?:.*/)?(?:ip6?tables(?:-legacy|-nft)?|nft)', token):
                        self.fail(f'{path.name}: unquoted host firewall command')
                    if re.search(r'\$\([^)]*\b(?:iptables|ip6tables|nft)\b', token):
                        self.fail(f'{path.name}: host firewall command substitution')
                vm_firewall += [t for t in tokens if 'nft ' in t]
        self.assertTrue(any('nft delete table inet __cfnat_test' in t for t in vm_firewall))
        workflow_text = (ROOT / '.github/workflows/e2e-openwrt.yml').read_text()
        self.assertIn('uninstall-clashforge', workflow_text)
        self.assertIn('sh /tmp/e2e-stop-verify.sh', workflow_text)
        self.assertIn('sh /tmp/e2e-verify-tun-cleanup.sh', workflow_text)
        self.assertNotIn('192.168.10.1', workflow_text)

    def test_subscription_regressions_are_ci_gates(self):
        for file in ('ci.yml', 'e2e-openwrt.yml'):
            text = '\n'.join(runs(job) for job in workflow(file)['jobs'].values() if 'steps' in job)
            self.assertIn('python3 tests/e2e/test_subscription_contract.py', text)
            self.assertIn('python3 tests/e2e/test_ci_contract.py', text)
            self.assertIn('./tests/integration', text)

    def test_android_keeps_real_subscription_and_lifecycle_gates(self):
        text = runs(workflow('android-ci.yml')['jobs']['e2e'])
        self.assertNotIn('apt-get install -y tor', text)
        runner = (ROOT / 'tests/android/run_emulator_e2e.sh').read_text()
        self.assertIn('adb reverse --remove tcp:9050', runner)
        self.assertIn('adb shell rm -f /data/local/tmp/clashforge_ci_tor.enable', runner)
        self.assertNotIn('adb reverse tcp:9050 tcp:9050', runner)
        app = (ROOT / 'mobile/integration_test/app_e2e_test.dart').read_text()
        self.assertNotIn('markTestSkipped', app)
        for gate in ('assertExpectedAnyTlsRoute(data, _expectedNode)',
                     'isNot(equals(baselineIp))', 'equals(baselineIp)',
                     'await _waitForExternalTunProbeIfRequired(baselineIp)',
                     "reason: 'VPN must be off'"):
            self.assertIn(gate, app)

    def test_android_uploads_only_encrypted_native_diagnostics(self):
        steps = workflow('android-ci.yml')['jobs']['e2e']['steps']
        uploads = [s for s in steps if s.get('uses', '').startswith('actions/upload-artifact@')]
        self.assertEqual(len(uploads), 1)
        self.assertEqual(uploads[0]['with']['path'], '${{ runner.temp }}/clashforge-e2e-diagnostics.cms')
        self.assertEqual(uploads[0]['with']['retention-days'], 3)
        self.assertIn('always()', uploads[0]['if'])
        encrypt = next(i for i, s in enumerate(steps) if 'encrypt_diagnostics.sh' in s.get('run', ''))
        self.assertLess(encrypt, steps.index(uploads[0]))
        certificate = (ROOT / '.github/ci-diagnostics-recipient.pem').read_text()
        self.assertIn('BEGIN CERTIFICATE', certificate)
        self.assertNotIn('PRIVATE KEY', certificate)

    def test_all_workflow_topologies(self):
        for path in sorted((ROOT / '.github/workflows').glob('*.yml')):
            with self.subTest(path=path.name):
                jobs = workflow(path.name)['jobs']
                def visit(name, chain):
                    self.assertIn(name, jobs)
                    self.assertNotIn(name, chain, 'cyclic workflow dependency')
                    needs = jobs[name].get('needs', [])
                    if isinstance(needs, str): needs = [needs]
                    for dep in needs: visit(dep, chain + [name])
                for name in jobs: visit(name, [])
                for name, job in jobs.items():
                    if 'steps' not in job: continue
                    checkout = [i for i,s in enumerate(job['steps']) if s.get('uses','').startswith('actions/checkout@')]
                    local = [i for i,s in enumerate(job['steps']) if s.get('uses','').startswith('./') or '.github/scripts/download-mihomo.py' in s.get('run','')]
                    for i in local: self.assertTrue(checkout and min(checkout) < i, f'{path.name}/{name}: local code before checkout')

    def test_quickstart_is_executed_in_both_test_domains(self):
        for file in ('ci.yml','e2e-openwrt.yml'):
            commands = [line.strip() for job in workflow(file)['jobs'].values() if 'steps' in job for line in runs(job).splitlines() if 'go test ' in line and './cmd/clashforge' in line]
            self.assertTrue(commands, file)
            for command in commands:
                self.assertIn('./internal/quickstart', shlex.split(command), file)
                self.assertIn('-p 2', command)

    def test_hyperv_uses_pinned_digest_downloader_before_packaging(self):
        jobs = workflow('openwrt-hyperv-image.yml')['jobs']
        steps = jobs['build-hyperv-image']['steps']
        download = [i for i,s in enumerate(steps) if 'Download mihomo' in s.get('name','')]
        self.assertEqual(len(download),1)
        command = steps[download[0]]['run']
        self.assertIn('python3 .github/scripts/download-mihomo.py mihomo-linux-amd64-compatible /tmp/mihomo',command)
        self.assertIn('MIHOMO_VER=v1.19.32',command)
        self.assertNotIn('releases/latest',command)
        self.assertNotIn('gunzip',command)
        self.assertTrue(any('/tmp/mihomo' in s.get('run','') for s in steps[download[0]+1:]))
        for file in ('release.yml','e2e-openwrt.yml','openwrt-hyperv-image.yml'):
            text = '\n'.join(runs(job) for job in workflow(file)['jobs'].values() if 'steps' in job)
            self.assertIn('.github/scripts/download-mihomo.py',text)
            self.assertNotIn('mihomo-linux-arm64 -v',text)
            self.assertNotIn('mihomo-linux-arm64 --version',text)

if __name__ == '__main__':
    unittest.main(verbosity=2)
