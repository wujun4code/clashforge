"""Offline CA preparation and source-build wiring contracts (stdlib only)."""
import hashlib
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'mobile/scripts/prepare-mihomo-ca.py'
FIXTURE = Path(__file__).parent / 'fixtures/mihomo-ca.pem'
PREPARE = 'python3 mobile/scripts/prepare-mihomo-ca.py /tmp/mihomo-src'


class PrepareCATest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / 'source'
        self.target = self.source / 'component/ca/ca-certificates.crt'
        self.target.parent.mkdir(parents=True)
        self.original = b'original target must survive invalid input\n'
        self.target.write_bytes(self.original)
        self.bundle = self.root / 'bundle.pem'

    def run_prepare(self, data=None, source=None):
        self.assertTrue(SCRIPT.is_file(), 'shared CA preparer is missing')
        if data is not None:
            self.bundle.write_bytes(data)
        return subprocess.run([sys.executable, str(SCRIPT),
            str(source or self.source), str(self.bundle)],
            capture_output=True, text=True)

    def assert_rejected(self, data=None, source=None):
        result = self.run_prepare(data, source)
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(result.stdout, '')
        self.assertNotIn('PRIVATE-SENTINEL', result.stderr)
        self.assertEqual(self.target.read_bytes(), self.original)
        self.assertEqual(list(self.target.parent.iterdir()), [self.target])

    def test_valid_bundle_copied_exactly_and_output_is_count_hash_only(self):
        data = FIXTURE.read_bytes()
        result = self.run_prepare(data)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.target.read_bytes(), data)
        self.assertEqual(result.stdout,
            f'count=1 sha256={hashlib.sha256(data).hexdigest()}\n')
        self.assertEqual(result.stderr, '')

    def test_multiple_pem_blocks_are_validated(self):
        data = FIXTURE.read_bytes() * 2
        result = self.run_prepare(data)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.target.read_bytes(), data)
        # OpenSSL deduplicates identical certificates in its trust store.
        self.assertRegex(result.stdout, r'^count=1 sha256=[0-9a-f]{64}\n$')

    def test_empty_rejected_without_target_change(self):
        self.assert_rejected(b'')

    def test_whitespace_rejected_without_target_change(self):
        self.assert_rejected(b' \n\t')

    def test_invalid_rejected_without_target_change(self):
        self.assert_rejected(b'PRIVATE-SENTINEL not a certificate')

    def test_malformed_pem_rejected_without_target_change(self):
        self.assert_rejected(b'-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n')

    def test_valid_plus_invalid_block_rejected_without_target_change(self):
        self.assert_rejected(FIXTURE.read_bytes() +
            b'-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----\n')

    def test_non_pem_trailer_rejected_without_target_change(self):
        self.assert_rejected(FIXTURE.read_bytes() + b'PRIVATE-SENTINEL')

    def test_non_ascii_rejected_without_target_change(self):
        self.assert_rejected(b'\xff\xfe')

    def test_missing_bundle_rejected_without_target_change(self):
        self.assert_rejected()

    def test_missing_source_rejected_without_target_change(self):
        self.assert_rejected(FIXTURE.read_bytes(), self.root / 'missing-source')

    def test_missing_ca_directory_rejected_without_target_change(self):
        other = self.root / 'other-source'
        other.mkdir()
        self.assert_rejected(FIXTURE.read_bytes(), other)
        self.assertEqual(list(other.iterdir()), [])


class SourceBuildWiringTest(unittest.TestCase):
    def workflow_source_steps(self, filename):
        text = (ROOT / '.github/workflows' / filename).read_text()
        return [step for step in re.split(r'(?m)^      - ', text)
                if 'go build -C /tmp/mihomo-src' in step]

    def check_step(self, step):
        self.assertEqual(step.count(PREPARE), 1)
        self.assertLess(step.index('git clone'), step.index(PREPARE))
        self.assertLess(step.index(PREPARE), step.index('go build -C /tmp/mihomo-src'))
        self.assertNotRegex(step, r'prepare-mihomo-ca[^\n]*(?:\|\| true|; true|&$)')

    def test_ci_both_source_builds_prepare_ca_before_build(self):
        steps = self.workflow_source_steps('android-ci.yml')
        self.assertEqual(len(steps), 2)
        for step in steps:
            with self.subTest(step=step.splitlines()[0]):
                self.check_step(step)

    def test_release_source_build_prepares_ca_before_build(self):
        steps = self.workflow_source_steps('android-release.yml')
        self.assertEqual(len(steps), 1)
        self.check_step(steps[0])

    def test_download_script_prepares_only_in_build_mode_with_explicit_override(self):
        text = (ROOT / 'mobile/scripts/download-mihomo.sh').read_text()
        build, download = text.split('# ── Download pre-built binaries', 1)
        command = 'python3 "$SCRIPT_DIR/prepare-mihomo-ca.py" "$SRC" "${2:-/etc/ssl/certs/ca-certificates.crt}"'
        self.assertEqual(build.count(command), 1)
        self.assertLess(build.index('if [ "$BUILD_MODE" = "--build" ]'), build.index(command))
        self.assertLess(build.index(command), build.index('go build -C'))
        self.assertNotIn('prepare-mihomo-ca', download)

    def test_host_regression_is_in_existing_ci(self):
        text = (ROOT / '.github/workflows/ci.yml').read_text()
        self.assertIn('python3 tests/android/test_mihomo_ca.py', text)

    def test_default_is_official_ubuntu_bundle_no_implicit_discovery(self):
        self.assertTrue(SCRIPT.is_file(), 'shared CA preparer is missing')
        text = SCRIPT.read_text()
        self.assertIn('/etc/ssl/certs/ca-certificates.crt', text)
        self.assertNotIn('create_default_context', text)
        self.assertNotIn('load_default_certs', text)
        self.assertNotIn('get_default_verify_paths', text)


if __name__ == '__main__':
    unittest.main(verbosity=2)
