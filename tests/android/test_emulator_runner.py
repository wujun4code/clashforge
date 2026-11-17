"""Host-only runner regressions; no device, credentials, or host networking."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


class EmulatorRunnerTest(unittest.TestCase):
    def run_case(self, flutter_exit):
        runner = Path(os.environ.get('RUNNER_UNDER_TEST',
                                    str(Path(__file__).with_name('run_emulator_e2e.sh')))).resolve()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            (root / 'mobile').mkdir()
            bin_dir = root / 'bin'
            bin_dir.mkdir()
            scripts = {
                'adb': '''#!/bin/bash
case "$*" in
  'shell /data/local/tmp/cf-tun-probe') printf '192.0.2.10\\n';;
  *e2e_tun_probe_result.json*)
    if [[ "$*" == *'sh -c'* ]]; then
      echo 'ERROR: attempted result write without readiness' >&2; exit 37
    fi;;
                'logcat -v time') echo 'MihomoCore fixture-private-endpoint.invalid tls handshake error'; sleep 2;;
esac
exit 0
''',
                'flutter': f'#!/bin/bash\necho "fixture Flutter failure fixture-private-endpoint.invalid"\nexit {flutter_exit}\n',
                'sleep': '#!/bin/bash\n/bin/sleep 0.01\n',
            }
            for name, text in scripts.items():
                path = bin_dir / name
                path.write_text(text)
                path.chmod(0o755)
            env = dict(os.environ, PATH=str(bin_dir) + ':' + os.environ['PATH'],
                       GITHUB_WORKSPACE=str(root), RUNNER_TEMP=str(root),
                       SUBSCRIPTION_URL='https://subscription.example.invalid/test-only')
            result = subprocess.run(['bash', str(runner)], env=env,
                                    capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, flutter_exit or 1, result.stdout + result.stderr)
            self.assertNotIn('attempted result write', result.stderr)
            self.assertIn('readiness marker was not observed', result.stdout)
            self.assertNotIn('fixture-private-endpoint.invalid', result.stdout + result.stderr)
            self.assertIn('fixture-private-endpoint.invalid', (root / 'clashforge-e2e-flutter.txt').read_text())
            self.assertEqual((root / 'clashforge-e2e-flutter.txt').stat().st_mode & 0o777, 0o600)
            self.assertIn('Private core log classification', result.stdout)
            self.assertEqual((root / 'clashforge-e2e-logcat.txt').stat().st_mode & 0o777, 0o600)

    def test_flutter_failure_is_not_masked_by_result_write_after_uninstall(self):
        self.run_case(9)

    def test_flutter_success_without_readiness_is_not_a_tun_pass(self):
        self.run_case(0)


if __name__ == '__main__':
    unittest.main()
