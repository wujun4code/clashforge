"""Exercise real authenticated encryption with a disposable test-only key."""
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]


class DiagnosticsEncryptionTest(unittest.TestCase):
    def test_only_ciphertext_is_exported_and_local_key_decrypts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            (root / '.github').mkdir()
            key = root / 'test-only.key'
            cert = root / '.github/ci-diagnostics-recipient.pem'
            generated = subprocess.run(['openssl', 'req', '-newkey', 'rsa:2048',
                '-x509', '-sha256', '-days', '1', '-nodes', '-subj', '/CN=Test-only',
                '-keyout', str(key), '-out', str(cert)], capture_output=True)
            self.assertEqual(generated.returncode, 0)
            payload = b'MihomoCore synthetic-private-endpoint.invalid tls handshake error\n'
            (root / 'clashforge-e2e-logcat.txt').write_bytes(payload)
            prebuild = b'synthetic-private-prebuild stdout and stderr\n'
            (root / 'clashforge-e2e-flutter.txt').write_bytes(prebuild)
            completed = subprocess.run(['bash', str(ROOT / 'tests/android/encrypt_diagnostics.sh')],
                env=dict(os.environ, RUNNER_TEMP=directory, GITHUB_WORKSPACE=directory),
                capture_output=True, text=True)
            self.assertEqual(completed.returncode, 0, completed.stderr)
            self.assertNotIn('synthetic-private-endpoint', completed.stdout + completed.stderr)
            encrypted = root / 'clashforge-e2e-diagnostics.cms'
            self.assertNotIn(payload, encrypted.read_bytes())
            self.assertEqual(encrypted.stat().st_mode & 0o777, 0o600)
            plaintext = subprocess.run(['openssl', 'cms', '-decrypt', '-binary',
                '-inform', 'DER', '-in', str(encrypted), '-recip', str(cert),
                '-inkey', str(key)], capture_output=True)
            self.assertEqual(plaintext.returncode, 0)
            with tarfile.open(fileobj=io.BytesIO(plaintext.stdout), mode='r:gz') as archive:
                self.assertEqual(archive.getnames(), ['clashforge-e2e-logcat.txt', 'clashforge-e2e-flutter.txt'])
                flutter_member = archive.extractfile('clashforge-e2e-flutter.txt')
                assert flutter_member is not None
                self.assertEqual(flutter_member.read(), prebuild)
                member = archive.extractfile('clashforge-e2e-logcat.txt')
                self.assertIsNotNone(member)
                assert member is not None
                self.assertEqual(member.read(), payload)
            self.assertFalse(list(root.glob('clashforge-diagnostics.*')))


if __name__ == '__main__':
    unittest.main()
