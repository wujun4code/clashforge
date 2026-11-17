"""Recording-only RED/GREEN prebuild contract: no network, Gradle or device."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
VERSION = '28.2.13676358'
WRAPPER = Path(os.environ.get('PREBUILD_SCRIPT', ROOT / 'tests/android/prebuild_debug_apk.sh'))


class PrebuildTest(unittest.TestCase):
    def case(self, code=0, output='yes', installed='missing', install='ok', old=False, consume=False):
        with tempfile.TemporaryDirectory() as d:
            root = Path(d)
            sdk = root / 'sdk'
            tools = root / 'flutter/bin'
            tools.mkdir(parents=True)
            extension = root / 'flutter/packages/flutter_tools/gradle/src/main/kotlin/FlutterExtension.kt'
            extension.parent.mkdir(parents=True)
            extension.write_text(f'val ndkVersion: String = "{VERSION}"\n')
            mobile = root / 'mobile'
            mobile.mkdir()
            runner = root / 'private'
            runner.mkdir()
            props = sdk / f'ndk/{VERSION}/source.properties'
            props.parent.mkdir(parents=True)
            if installed != 'missing':
                props.write_text('Pkg.Revision = ' + (VERSION if installed == 'ok' else '27.0.0') + '\n')
            flutter = tools / 'flutter'
            flutter.write_text('''#!/bin/bash
printf '%s\\n' "$@" > "$RECORD/flutter-args"
echo synthetic-private-stdout
echo synthetic-private-stderr >&2
if [ "$CONSUME" = yes ]; then cat >/dev/null; fi
if [ "$OUTPUT" = yes ]; then
 mkdir -p build/app/outputs/flutter-apk
 echo apk > build/app/outputs/flutter-apk/app-debug.apk
fi
exit "$CODE"
''')
            flutter.chmod(0o755)
            manager = sdk / 'cmdline-tools/latest/bin/sdkmanager'
            manager.parent.mkdir(parents=True)
            manager.write_text('''#!/bin/bash
echo "$*" >> "$RECORD/manager"
echo synthetic-private-sdk >&2
if [ "$INSTALL" = fail ]; then exit 17; fi
if [ "$INSTALL" = lie ]; then exit 0; fi
if [ "$INSTALL" = stdin-ok ]; then cat >/dev/null; fi
printf 'Pkg.Revision = 28.2.13676358\\n' > "$PROPS"
if [ "$INSTALL" = timeout-residue ]; then exit 124; fi
if [ "$INSTALL" = failure-residue ]; then exit 17; fi
''')
            manager.chmod(0o755)
            env = dict(os.environ, PATH=str(tools) + ':' + os.environ['PATH'],
                       GITHUB_WORKSPACE=str(root), RUNNER_TEMP=str(runner), ANDROID_HOME=str(sdk),
                       SUBSCRIPTION_URL='synthetic-only', RECORD=str(root), PROPS=str(props),
                       CODE=str(code), OUTPUT=output, INSTALL=install, CONSUME='yes' if consume else 'no')
            apk = mobile / 'build/app/outputs/flutter-apk/app-debug.apk'
            apk.parent.mkdir(parents=True)
            apk.write_text('stale')
            if old:
                apk.unlink()
                result = subprocess.run(['bash', '-e', '-c', 'flutter build apk --debug 2>&1 | tail -20'],
                                        cwd=mobile, env=env, text=True, capture_output=True)
                self.assertEqual(result.returncode, 0, 'RED: old pipeline masks Flutter exit')
                self.assertFalse(apk.exists())
                return
            result = subprocess.run(['bash', str(WRAPPER)],
                                    env=env, text=True, capture_output=True, timeout=10)
            for marker in ('synthetic-private-stdout', 'synthetic-private-stderr', 'synthetic-private-sdk'):
                self.assertNotIn(marker, result.stdout + result.stderr)
            log = runner / 'clashforge-e2e-flutter.txt'
            self.assertEqual(log.stat().st_mode & 0o777, 0o600)
            attempts = (root / 'manager').read_text().splitlines() if (root / 'manager').exists() else []
            if installed == 'ok':
                self.assertEqual(attempts, [])
            else:
                self.assertEqual(len(attempts), 1 if install in ('ok', 'stdin-ok') else 2)
                self.assertTrue(all(f'ndk;{VERSION}' in a and f'--sdk_root={sdk}' in a for a in attempts))
                self.assertIn('synthetic-private-sdk', log.read_text())
            if install not in ('ok', 'stdin-ok') and installed != 'ok':
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse((root / 'flutter-args').exists())
            else:
                self.assertEqual(result.returncode, code if code else (0 if output == 'yes' else 1))
                for marker in ('synthetic-private-stdout', 'synthetic-private-stderr'):
                    self.assertIn(marker, log.read_text())
                args = (root / 'flutter-args').read_text().splitlines()
                self.assertEqual(args, ['build', 'apk', '--debug', '--dart-define=SUBSCRIPTION_URL=synthetic-only'])
                if output != 'yes': self.assertFalse(apk.exists(), 'stale APK must not pass')

    def test_sdk_stdin_cannot_swallow_flutter(self):
        self.case(code=23, output='no', install='stdin-ok')

    def test_flutter_stdin_cannot_skip_apk_validation(self):
        self.case(code=0, output='no', installed='ok', consume=True)

    def test_red_old_pipeline_is_false_green(self):
        self.case(code=23, output='no', old=True)

    def test_green_exit_and_output_contract(self):
        for code, output in ((23, 'no'), (23, 'yes'), (0, 'no'), (0, 'yes')):
            with self.subTest(code=code, output=output): self.case(code=code, output=output)

    def test_failed_install_cannot_pass_with_valid_version_residue(self):
        for install in ('timeout-residue', 'failure-residue'):
            with self.subTest(install=install):
                self.case(install=install)

    def test_valid_cache_skips_even_failing_installer(self):
        self.case(installed='ok', install='timeout-residue')

    def test_ndk_source_properties_and_bounded_installer(self):
        for installed, install in (('ok', 'ok'), ('wrong', 'ok'), ('missing', 'fail'), ('wrong', 'lie')):
            with self.subTest(installed=installed, install=install): self.case(installed=installed, install=install)


if __name__ == '__main__':
    unittest.main(verbosity=2)
