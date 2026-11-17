#!/usr/bin/env python3
"""Production CLI -> production init/preflight, isolated from host networking."""
import os
from pathlib import Path
import subprocess
import unittest
import tomllib

import re
import shutil
import tempfile

REPO = Path(__file__).resolve().parents[2]


class ClashforgeReset(unittest.TestCase):
    def executable(self, path, body):
        if not path.is_relative_to(self.root):
            raise AssertionError(f"refusing executable outside sandbox: {path}")
        if path.is_symlink():
            path.unlink()
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text('#!/bin/sh\n' + body)
        path.chmod(0o755)

    def rebase(self, source):
        return re.sub(r'/(etc|usr|var|dev|sys|proc|tmp)/',
                      lambda m: str(self.root) + m[0], source)

    def log(self):
        return self.calls.read_text() if self.calls.exists() else ''

    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix='cf-cli-', dir=os.environ.get('TMPDIR'))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.calls = self.root / 'calls'
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        for name in ('awk', 'grep', 'head', 'cat', 'mkdir', 'touch', 'rm', 'timeout', 'find'):
            tool = shutil.which(name)
            assert tool, f'missing sandbox prerequisite: {name}'
            (self.bin / name).symlink_to(tool)
        for name in ('ip', 'nft', 'uci', 'pgrep', 'service', 'modprobe', 'mknod', 'ssh'):
            self.executable(self.bin / name,
                            'echo "${0##*/} $*" >> "$CALLS"\nexit 1\n')
        self.executable(self.bin / 'logger', 'echo "logger $*" >> "$CALLS"\n')
        self.executable(self.bin / 'sleep', 'exit 0\n')
        (self.root / 'dev').mkdir()
        (self.root / 'dev/null').symlink_to('/dev/null')
        self.env = dict(os.environ, PATH=str(self.bin), CALLS=str(self.calls))
        self.config = self.root / 'etc/metaclash/config.toml'
        self.config.parent.mkdir(parents=True)
        preflight = self.root / 'usr/share/metaclash/preflight.sh'
        preflight.parent.mkdir(parents=True)
        preflight.write_text(self.rebase((REPO / 'openwrt/files/usr/share/metaclash/preflight.sh').read_text()))
        self.executable(self.root / 'usr/bin/mihomo-clashforge',
                        'echo "Mihomo Meta v1.19.32 linux amd64"\nexit 1\n')
        # Actual package layout: no template or postinst under /etc required.
        self.assets = [self.root / 'usr/share/metaclash/preflight.sh']
        self.executable(self.bin / 'opkg', '''printf 'opkg %s\\n' "$*" >> "$CALLS"
case "$1" in
 files) cat "$PACKAGE_FILES" ;;
 install) exit 0 ;; # same-version install is a no-op; never restore assets
 *) exit 1 ;;
esac
''')
        self.package_files = self.root / 'package-files'
        self.env['PACKAGE_FILES'] = str(self.package_files)
        self.sync_assets()
        init = self.root / 'production-init'
        init.write_text(self.rebase((REPO / 'openwrt/files/etc/init.d/metaclash').read_text()))
        self.executable(self.root / 'etc/init.d/clashforge', f'''case "$1" in
 enabled|enable) exit 0 ;;
 stop) rm -f "$READY"; exit 0 ;;
 start) ;;
 *) exit 1 ;;
esac
[ "${{FAIL_START:-0}}" = 0 ] || exit 23
. "{init}"
procd_open_instance() {{ echo procd_open >> "$CALLS"; }}
procd_set_param() {{ echo "procd_param $*" >> "$CALLS"; }}
procd_close_instance() {{ touch "$READY"; }}
start_service
''')
        self.ready = self.root / 'api-ready'
        self.env['READY'] = str(self.ready)
        mem = self.root / 'proc/meminfo'
        mem.parent.mkdir(parents=True)
        mem.write_text('MemAvailable: 999999 kB\n')
        self.cli = self.root / 'clashforgectl'
        source = Path(os.environ.get('CLASHFORGECTL_TEST_SOURCE', REPO / 'scripts/clashforgectl.sh'))
        self.cli.write_text(self.rebase(source.read_text()))
        for name in ('etc/metaclash/nodes.json', 'etc/metaclash/.secret',
                     'etc/metaclash/subscriptions/user.yaml', 'usr/share/metaclash/cache/user',
                     'var/run/metaclash/config.yaml', 'tmp/clashforge-config.bak'):
            p = self.root / name
            p.parent.mkdir(parents=True, exist_ok=True)
            p.write_text('synthetic user data\n')
        self.config.write_text('[core]\nbinary="/missing-user-core"\n')
        self.ipk = self.root / 'local.ipk'
        self.ipk.write_bytes(b'synthetic local package; opkg is a recording stub')

    def sync_assets(self):
        self.package_files.write_text('Package clashforge files:\n' + '\n'.join(map(str, self.assets)) + '\n')

    def run_cli(self, *args, **env):
        return subprocess.run(['/bin/sh', str(self.cli), *args], env=dict(self.env, **env),
                              text=True, capture_output=True, timeout=10)

    def assert_ok(self, result):
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr + self.log())

    def test_reset_start_reaches_real_init_with_valid_first_install(self):
        result = self.run_cli('reset', '--yes', '--start')
        self.assert_ok(result)
        self.assertTrue(self.ready.exists(), result.stderr + self.log())
        config = tomllib.loads(self.config.read_text())
        self.assertEqual(config['core']['binary'], str(self.root / 'usr/bin/mihomo-clashforge'))
        self.assertFalse(config['core']['auto_start_core'])
        self.assertEqual(config['network']['mode'], 'none')
        self.assertFalse(config['network']['apply_on_start'])
        self.assertFalse(config['dns']['apply_on_start'])
        self.assertGreater(config['ports']['ui'], 0)
        self.assertIn('Selected core:', self.log())

    def test_reset_preserves_package_assets_and_wipes_user_data(self):
        # Also cover packages that ship support scripts/templates/geodata.
        for name in ('etc/metaclash/postinst.sh', 'etc/metaclash/config.toml.example',
                     'usr/share/metaclash/Country.mmdb'):
            p = self.root / name
            p.write_text('packaged support\n')
            self.assets.append(p)
        self.sync_assets()
        before = {p: p.read_bytes() for p in self.assets}
        self.assert_ok(self.run_cli('reset', '--yes'))
        for p, content in before.items():
            self.assertEqual(p.read_bytes(), content)
        for name in ('etc/metaclash/nodes.json', 'etc/metaclash/.secret',
                     'etc/metaclash/subscriptions/user.yaml', 'usr/share/metaclash/cache/user',
                     'var/run/metaclash/config.yaml', 'tmp/clashforge-config.bak'):
            self.assertFalse((self.root / name).exists(), name)
        self.assertFalse(self.ready.exists())

    def test_reset_then_same_version_local_upgrade(self):
        self.assert_ok(self.run_cli('reset', '--yes'))
        before = self.config.read_bytes() if self.config.exists() else b''
        self.assert_ok(self.run_cli('upgrade', '--local-ipk', str(self.ipk)))
        self.assertTrue(self.ready.exists(), self.log())
        self.assertEqual(self.config.read_bytes(), before)
        self.assertTrue(self.ipk.exists())
        self.assertNotIn('--force-reinstall', self.log())

    def test_reset_start_failure_is_nonzero(self):
        result = self.run_cli('reset', '--yes', '--start', FAIL_START='1')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('ClashForge started.', result.stderr)
        self.assertFalse(self.ready.exists())

    def test_upgrade_start_failure_preserves_local_ipk_and_config(self):
        before = self.config.read_bytes()
        result = self.run_cli('upgrade', '--local-ipk', str(self.ipk), FAIL_START='1')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertNotIn(' installed!', result.stderr)
        self.assertEqual(self.config.read_bytes(), before)
        self.assertTrue(self.ipk.exists())

    def test_filesystem_failure_is_not_reported_as_reset_success(self):
        self.executable(self.bin / 'rm', 'echo "injected filesystem failure" >&2\nexit 13\n')
        result = self.run_cli('reset', '--yes')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('Reset complete.', result.stderr)
        self.assertFalse(self.ready.exists())

    def test_preflight_missing_core_blocks_ready(self):
        (self.root / 'usr/bin/mihomo-clashforge').unlink()
        result = self.run_cli('reset', '--yes', '--start')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertIn('selected core is not executable', result.stderr)
        self.assertNotIn('procd_open', self.log())
        self.assertFalse(self.ready.exists())

    def test_find_failure_cannot_report_reset_success(self):
        self.executable(self.bin / 'find', 'exit 17\n')
        result = self.run_cli('reset', '--yes')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertNotIn('Reset complete.', result.stderr)

    def test_package_inventory_failure_is_nondestructive(self):
        self.executable(self.bin / 'opkg', 'exit 18\n')
        before = self.config.read_bytes()
        result = self.run_cli('reset', '--yes')
        self.assertNotEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.config.read_bytes(), before)
        self.assertTrue(self.assets[0].exists())

    def assert_reset_rejected_without_changes(self):
        before = {p.relative_to(self.root): p.read_bytes()
                  for p in self.root.rglob('*') if p.is_file()}
        result = self.run_cli('reset', '--yes', '--start')
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        after = {p.relative_to(self.root): p.read_bytes()
                 for p in self.root.rglob('*') if p.is_file()
                 and p != self.calls}
        self.assertEqual(after, before)
        self.assertEqual(self.log().splitlines(), ['opkg files clashforge'])
        self.assertFalse(self.ready.exists())

    def test_untrusted_nonempty_package_inventory_is_nondestructive(self):
        for inventory in ("Package clashforge is not installed.\n",
                          "Package clashforge files:\n",
                          "Package another files:\n/usr/share/another/preflight.sh\n"):
            with self.subTest(inventory=inventory):
                self.package_files.write_text(self.rebase(inventory))
                if self.calls.exists():
                    self.calls.unlink()
                self.assert_reset_rejected_without_changes()

    def test_inventory_with_missing_helper_is_nondestructive(self):
        self.assets[0].unlink()
        self.assert_reset_rejected_without_changes()

    def test_persistent_root_symlinks_are_nondestructively_rejected(self):
        for name in ('etc/metaclash', 'usr/share/metaclash'):
            with self.subTest(root=name):
                directory = self.root / name
                outside = self.root / ('outside-' + name.replace('/', '-'))
                directory.rename(outside)
                directory.symlink_to(outside, target_is_directory=True)
                try:
                    before = {p.relative_to(self.root): p.read_bytes()
                              for p in self.root.rglob('*')
                              if p.is_file() and p != self.calls}
                    if self.calls.exists():
                        self.calls.unlink()
                    result = self.run_cli('reset', '--yes', '--start')
                    after = {p.relative_to(self.root): p.read_bytes()
                             for p in self.root.rglob('*')
                             if p.is_file() and p != self.calls}
                    self.assertEqual(after, before, 'outside config/userdata/helper must be unchanged')
                    self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
                    self.assertTrue(directory.is_symlink())
                    self.assertEqual(self.log(), '')
                    self.assertFalse(self.ready.exists())
                finally:
                    directory.unlink()
                    outside.rename(directory)

    def test_declined_reset_is_noop(self):
        before = self.config.read_bytes()
        result = subprocess.run(['/bin/sh', str(self.cli), 'reset'], input='n\n',
                                env=self.env, text=True, capture_output=True, timeout=10)
        self.assert_ok(result)
        self.assertEqual(self.config.read_bytes(), before)
        self.assertEqual(self.log(), '')


if __name__ == '__main__':
    unittest.main(verbosity=2)
