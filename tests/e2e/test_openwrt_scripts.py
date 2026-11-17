#!/usr/bin/env python3
"""Run production OpenWrt scripts in a path-rebased sandbox; never network E2E.

Only absolute filesystem roots are rebased. Function bodies/dispatch and command
arguments are otherwise unchanged. Network/module commands are recording stubs.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

REPO = Path(__file__).resolve().parents[2]


class OpenWrtScripts(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="cf-openwrt-", dir=os.environ.get("TMPDIR"))
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.calls = self.root / "calls"
        self.bin = self.root / "bin"
        self.bin.mkdir()
        for name in ("awk", "sed", "grep", "head", "tr", "cat", "mkdir", "touch", "rm", "cp", "mv", "chmod", "ls", "stat", "timeout"):
            tool = shutil.which(name)
            if tool:
                (self.bin / name).symlink_to(tool)
        for name in ("modprobe", "mknod", "ip", "logger", "nft", "iptables", "ssh", "start"):
            self.executable(self.bin / name, 'printf "%s %s\\n" "${0##*/}" "$*" >> "$CALLS"\nexit 1\n')
        self.executable(self.bin / "id", 'echo 0\n')
        self.executable(self.bin / "logger", 'printf "logger %s\\n" "$*" >> "$CALLS"\n')
        (self.root / "dev").mkdir()
        (self.root / "dev/null").symlink_to("/dev/null")
        self.env = dict(os.environ, PATH=str(self.bin), CALLS=str(self.calls))
        self.config = self.root / "etc/metaclash/config.toml"
        self.config.parent.mkdir(parents=True)
        self.core = self.root / "opt/custom-mihomo"
        self.version = "Mihomo Meta v1.19.32 linux amd64\nUse tags: with_gvisor"
        self.set_core()
        self.executable(self.root / "usr/bin/mihomo", 'echo "Mihomo Meta v1.19.32 linux amd64"\n')
        self.executable(self.root / "usr/bin/mihomo-clashforge", 'echo "Mihomo Meta v1.19.32 linux amd64\\nUse tags: with_gvisor"\n')
        self.executable(self.root / "etc/init.d/clashforge", 'printf "service %s\\n" "$*" >> "$CALLS"\n[ "$1" != start ]\n')
        preflight = REPO / "openwrt/files/usr/share/metaclash/preflight.sh"
        if preflight.exists():
            dest = self.root / "usr/share/metaclash/preflight.sh"
            dest.parent.mkdir(parents=True, exist_ok=True)
            dest.write_text(self.rebase(preflight.read_text()))

    def executable(self, path, body):
        if not path.is_relative_to(self.root):
            raise AssertionError(f"refusing executable outside sandbox: {path}")
        # Replacing a symlink must NOT truncate its real host executable.
        if path.is_symlink():
            path.unlink()
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text("#!/bin/sh\n" + body)
        path.chmod(0o755)

    def rebase(self, source):
        # Single replacement pass prevents rebasing the sandbox prefix itself.
        import re
        return re.sub(r"/(etc|usr|var|dev|sys|proc|tmp)/", lambda m: str(self.root) + m[0], source)

    def set_core(self, output=None):
        output = self.version if output is None else output
        self.executable(self.core, 'printf "%s\\n" ' + "'" + output.replace("'", "'\\''") + "'\nexit 1\n")

    def write_config(self, mode="none", extra=""):
        self.config.write_text(f'[dns]\nmode = "tun"\n[core]\nbinary = "{self.core}"\n[network]\nmode = "{mode}" # selected mode\n{extra}')

    def run_init(self, dispatch="start_service"):
        source = self.rebase((REPO / "openwrt/files/etc/init.d/metaclash").read_text())
        path = self.root / "init.sh"
        path.write_text(source)
        runner = f''' . "{path}"
check_memory() {{ return 0; }}
procd_open_instance() {{ echo "procd_open $*" >> "$CALLS"; }}
procd_set_param() {{ echo "procd_param $*" >> "$CALLS"; }}
procd_close_instance() {{ echo procd_close >> "$CALLS"; }}
procd_add_reload_trigger() {{ echo "trigger $*" >> "$CALLS"; }}
{dispatch}
'''
        return subprocess.run(["/bin/sh", "-c", runner], env=self.env, text=True, capture_output=True)

    def log(self):
        return self.calls.read_text() if self.calls.exists() else ""

    def test_preserves_explicit_core_and_dispatches_manager(self):
        self.write_config()
        before = self.config.read_text()
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr + self.log())
        self.assertEqual(self.config.read_text(), before)
        self.assertIn(f'procd_param command {self.root}/usr/bin/clashforge -config {self.config}', self.log())

    def test_core_check_without_timeout_uses_bounded_daemon_helper(self):
        self.write_config()
        (self.bin / "timeout").unlink()
        self.executable(self.root / "usr/bin/clashforge", 'printf "core_check %s\\n" "$*" >> "$CALLS"\n[ "$1" = -core-version-check ] || exit 125\nshift\nexec "$@" -v\n')
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr + self.log())
        self.assertIn(f"core_check -core-version-check {self.core}", self.log())
        self.assertIn("procd_open", self.log())

    def test_no_timeout_and_no_daemon_helper_fails_closed(self):
        self.write_config()
        (self.bin / "timeout").unlink()
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("procd_open", self.log())

    def test_network_section_not_dns_mode(self):
        self.write_config()
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr + self.log())
        self.assertNotIn("modprobe", self.log())
        self.assertNotIn("mknod", self.log())

    def test_missing_explicit_core_is_not_replaced(self):
        self.write_config()
        self.core.chmod(0o644)
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0, self.log())
        self.assertNotIn("procd_open", self.log())
        self.assertIn(str(self.core), self.config.read_text())

    def test_unrecognized_selected_version_fails(self):
        self.write_config()
        self.set_core("not mihomo")
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0, self.log())
        self.assertNotIn("procd_open", self.log())

    def test_tun_missing_driver_fails_without_ready(self):
        self.write_config("tun")
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0, self.log())
        self.assertNotIn("procd_open", self.log())
        self.assertNotIn("tun ready", self.log())
        self.assertNotIn("mknod", self.log())

    def test_tun_registered_driver_but_missing_node_fails(self):
        self.write_config("tun")
        driver = self.root / "sys/class/misc/tun/dev"
        driver.parent.mkdir(parents=True)
        driver.write_text("10:200\n")
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("character device", self.log())
        self.assertNotIn("modprobe", self.log())
        self.assertNotIn("procd_open", self.log())

    def kernel_fixture(self):
        # Kernel facts are explicit mocks, never a real TUN or module operation.
        driver = self.root / "sys/class/misc/tun/dev"
        driver.parent.mkdir(parents=True)
        driver.write_text("10:200\n")
        (self.root / "dev/net").mkdir()
        (self.root / "dev/net/tun").symlink_to("/dev/null")
        self.executable(self.bin / "stat", 'echo a:c8\n')
        self.executable(self.bin / "ip", 'echo "ip $*" >> "$CALLS"\n')

    def test_tun_mock_prerequisites_dispatches_without_ready_claim(self):
        self.write_config("tun")
        self.kernel_fixture()
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr + self.log())
        self.assertIn("procd_open", self.log())
        self.assertIn("ip -4 rule show", self.log())
        self.assertIn("ip -4 route show table main", self.log())
        self.assertIn("still require runtime verification", self.log())
        self.assertNotIn("tun ready", self.log())

    def test_tun_ip_failure_blocks_dispatch(self):
        self.write_config("tun")
        self.kernel_fixture()
        self.executable(self.bin / "ip", 'exit 1\n')
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("ip-full", self.log())
        self.assertNotIn("procd_open", self.log())

    def test_tun_nonroot_blocks_dispatch(self):
        self.write_config("tun")
        self.executable(self.bin / "id", 'echo 1000\n')
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("CAP_NET_ADMIN", self.log())
        self.assertNotIn("procd_open", self.log())

    def test_tun_cleanup_readback_rejects_residue_and_inspection_failure(self):
        path = REPO / "tests/e2e/vm/verify-tun-cleanup.sh"
        for ip, nft, expect in (("echo '0: from all lookup local'", "echo 'chain forward_lan {}'", 0),
                                ("echo '9021: from all lookup 2022 proto 242'", "exit 0", 1),
                                ("exit 1", "exit 0", 1),
                                ("exit 0", "exit 1", 1),
                                ("exit 0", "echo 'accept comment \"clashforge: allow forwarding into TUN device\"'", 1)):
            with self.subTest(ip=ip, nft=nft):
                self.executable(self.bin / "ip", ip + "\n")
                self.executable(self.bin / "nft", nft + "\n")
                result = subprocess.run(["/bin/sh", str(path)], env=self.env, text=True, capture_output=True)
                self.assertEqual(result.returncode, expect, result.stdout + result.stderr)

    def test_tun_system_stack_does_not_require_gvisor(self):
        self.write_config("tun", '[network.tun]\nstack = "system"\n')
        self.set_core("Mihomo Meta v1.19.32 linux amd64")
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)  # absent driver, NOT gVisor
        self.assertIn("kernel TUN driver unavailable", self.log())
        self.assertNotIn("lacks with_gvisor", self.log())

    def test_version_timeout_is_bounded_failure(self):
        self.write_config()
        self.executable(self.bin / "timeout", 'echo "timeout $*" >> "$CALLS"\nexit 124\n')
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn(f"timeout 5 {self.core} -v", self.log())
        self.assertNotIn("procd_open", self.log())

    def test_single_quotes_and_indented_tables(self):
        self.config.write_text(f" [core] # comment\n binary = '{self.core}' # custom\n [network]\n mode = 'none'\n [dns]\n mode = 'tun'\n")
        result = self.run_init()
        self.assertEqual(result.returncode, 0, result.stderr + self.log())
        self.assertNotIn("modprobe", self.log())

    def test_ambiguous_toml_fails_without_guessing_or_rewrite(self):
        for value in ('mode = "none"\nmode = "tun"', 'mode = "t\\\\un"', 'mode = {value="tun"}', '"mode" = "tun"'):
            with self.subTest(value=value):
                self.config.write_text(f'[core]\nbinary = "{self.core}"\n[network]\n{value}\n')
                before = self.config.read_text()
                result = self.run_init()
                self.assertNotEqual(result.returncode, 0)
                self.assertEqual(self.config.read_text(), before)
        self.assertNotIn("procd_open", self.log())

    def test_reload_trigger_dispatch_remains_network_only(self):
        result = self.run_init("service_triggers")
        self.assertEqual(result.returncode, 0)
        self.assertEqual(self.log().strip(), "trigger network")

    def test_tun_selected_core_needs_gvisor_for_mixed(self):
        self.write_config("tun")
        self.set_core("Mihomo Meta v1.19.32 linux amd64")
        result = self.run_init()
        self.assertNotEqual(result.returncode, 0, self.log())
        self.assertIn("with_gvisor", self.log())
        self.assertNotIn("procd_open", self.log())

    def run_tun_scenario(self, dns_answer):
        (self.root / "dev/net").mkdir()
        (self.root / "dev/net/tun").symlink_to("/dev/null")
        self.executable(self.bin / "sleep", 'exit 0\n')
        self.executable(self.bin / "pgrep", 'echo 1234\n')
        self.executable(self.bin / "dig", 'printf "%s\\n" ' + repr(dns_answer) + '\n')
        self.executable(self.bin / "curl", '''case "$*" in
  */setup/launch) echo '{"success":true}' ;;
  */config) echo '{"ports":{"dns":17874}}' ;;
  *) exit 1 ;;
esac
''')
        self.executable(self.bin / "ip", '''case "$*" in
  '-4 rule show') printf '%s\\n' '9020: from all lookup main suppress_prefixlength 0 proto 242' '9021: from all lookup 2022 proto 242' ;;
  '-4 route show table 2022') echo 'default dev Meta scope link' ;;
  *) echo '2: Meta: <POINTOPOINT,UP> mtu 1500 state UNKNOWN' ;;
esac
''')
        self.executable(self.bin / "nft", '''case "$*" in
  'list tables') exit 0 ;;
  *) echo 'meta nfproto ipv4 oifname "Meta" counter packets 1 bytes 100 accept comment "clashforge: allow forwarding into TUN device" # handle 10' ;;
esac
''')
        source = Path(os.environ.get("OPENWRT_TUN_TEST_SOURCE", str(REPO / "tests/e2e/vm/setup-launch-tun.sh")))
        path = self.root / "scenario.sh"
        path.write_text(self.rebase(source.read_text()))
        return subprocess.run(["/bin/sh", str(path)], env=dict(self.env, REUSE_SUBSCRIPTION="true"), text=True, capture_output=True)

    def test_tun_full_scenario_dns_failure_is_hard_failure(self):
        result = self.run_tun_scenario("")
        self.assertIn("PASS", result.stdout)
        self.assertIn("[TUN-02]", result.stdout)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(result.stdout, r"FAIL[^\n]*\[TUN-06\]")

    def test_tun_full_scenario_control_plane_success(self):
        result = self.run_tun_scenario("198.18.0.5")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertRegex(result.stdout, r"PASS[^\n]*\[TUN-09\]")
        self.assertNotIn("FAIL", result.stdout)

    def test_tun_dns_dispatch_rejects_empty_answer(self):
        self.executable(self.bin / "dig", 'printf "dig %s\\n" "$*" >> "$CALLS"\nexit 0\n')
        self.executable(self.bin / "curl", 'echo "unexpected curl" >> "$CALLS"\nexit 1\n')
        self.executable(self.bin / "sleep", 'exit 0\n')
        path = self.root / "setup-launch-tun.sh"
        path.write_text(self.rebase((REPO / "tests/e2e/vm/setup-launch-tun.sh").read_text()))
        result = subprocess.run(["/bin/sh", str(path), "--check-dns", "17874", "example.com"], env=self.env, text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("dig +short +time=3 +tries=1 @127.0.0.1 -p 17874 example.com A", self.log())
        self.assertNotIn("unexpected curl", self.log())

    def test_tun_dns_dispatch_accepts_exact_core_answer(self):
        self.executable(self.bin / "dig", 'printf "dig %s\\n" "$*" >> "$CALLS"\necho 198.18.0.5\n')
        path = self.root / "setup-launch-tun.sh"
        path.write_text(self.rebase((REPO / "tests/e2e/vm/setup-launch-tun.sh").read_text()))
        result = subprocess.run(["/bin/sh", str(path), "--check-dns", "17874", "example.com"], env=self.env, text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(result.stdout.strip(), "198.18.0.5")
        self.assertIn("-p 17874 example.com A", self.log())

    def test_postinst_start_failure_is_visible(self):
        self.write_config()
        path = self.root / "postinst.sh"
        path.write_text(self.rebase((REPO / "openwrt/files/etc/metaclash/postinst.sh").read_text()))
        result = subprocess.run(["/bin/sh", str(path)], env=self.env, text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn("start", self.log())
        self.assertNotIn("ip rule add", self.log())

    def test_postinst_tun_preflight_failure_does_not_start(self):
        self.write_config("tun")
        path = self.root / "postinst.sh"
        path.write_text(self.rebase((REPO / "openwrt/files/etc/metaclash/postinst.sh").read_text()))
        result = subprocess.run(["/bin/sh", str(path)], env=self.env, text=True, capture_output=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("service start", self.log())
        self.assertNotIn("installed!", result.stdout)

    def test_postinst_offline_staging_is_noop(self):
        path = self.root / "postinst.sh"
        path.write_text(self.rebase((REPO / "openwrt/files/etc/metaclash/postinst.sh").read_text()))
        result = subprocess.run(["/bin/sh", str(path)], env=dict(self.env, IPKG_INSTROOT=str(self.root / "image")), text=True, capture_output=True)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.log(), "")


if __name__ == "__main__":
    unittest.main(verbosity=2)
