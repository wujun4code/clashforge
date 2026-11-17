#!/usr/bin/env python3
"""Exercise the real downloader against an HTTP fixture, including bad digests."""
import copy
import gzip
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import importlib.util
import json
import os
from pathlib import Path
import tempfile
import threading
import unittest
import urllib.request

SPEC = importlib.util.spec_from_file_location("download_mihomo", Path(__file__).resolve().parents[2] / ".github/scripts/download-mihomo.py")
assert SPEC is not None and SPEC.loader is not None
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class DownloadMihomo(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="cf-digest-", dir=os.environ.get("TMPDIR"))
        self.addCleanup(self.tmp.cleanup)
        self.output = Path(self.tmp.name) / "mihomo"
        self.output.write_bytes(b"previous executable")
        self.payload = gzip.compress(b"local HTTP test fixture, not a real core")
        self.digest = "sha256:" + hashlib.sha256(self.payload).hexdigest()
        name = f"{MODULE.ASSETS[0]}-{MODULE.VERSION}.gz"
        self.metadata = {"tag_name": MODULE.VERSION, "assets": [{"name": name, "digest": self.digest, "browser_download_url": MODULE.BASE + name}]}
        test = self

        class Handler(BaseHTTPRequestHandler):
            def do_GET(self):
                body = json.dumps(test.metadata).encode() if self.path == "/metadata" else test.payload
                self.send_response(200)
                self.end_headers()
                self.wfile.write(body)

            def log_message(self, format, *args):
                pass

        self.server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()
        self.addCleanup(self.close_server)

    def close_server(self):
        self.server.shutdown()
        self.thread.join()
        self.server.server_close()

    def opener(self, request, timeout):
        url = request.full_url if isinstance(request, urllib.request.Request) else request
        # Only the transport is redirected. Production metadata/tag/URL/digest
        # validation and streaming/decompression/write behavior run unchanged.
        path = "/metadata" if url == MODULE.API else "/asset"
        return urllib.request.urlopen(f"http://127.0.0.1:{self.server.server_port}{path}", timeout=timeout)

    def test_valid_digest_downloads_and_unpacks(self):
        self.assertEqual(MODULE.download(MODULE.ASSETS[0], self.output, self.opener), self.digest)
        self.assertEqual(self.output.read_bytes(), b"local HTTP test fixture, not a real core")
        self.assertTrue(self.output.stat().st_mode & 0o111)

    def test_digest_missing_invalid_mismatch_preserve_output(self):
        for digest in (None, "", "sha256:bogus", "sha256:" + "0" * 64):
            with self.subTest(digest=digest):
                self.metadata["assets"][0]["digest"] = digest
                with self.assertRaisesRegex(ValueError, "digest"):
                    MODULE.download(MODULE.ASSETS[0], self.output, self.opener)
                self.assertEqual(self.output.read_bytes(), b"previous executable")

    def test_wrong_tag_url_or_duplicate_asset_preserve_output(self):
        original = copy.deepcopy(self.metadata)
        variants = [dict(original, tag_name="v0.0.0"), dict(original, assets=original["assets"] * 2)]
        wrong_url = copy.deepcopy(original)
        wrong_url["assets"][0]["browser_download_url"] = "https://example.com/untrusted"
        variants.append(wrong_url)
        for metadata in variants:
            self.metadata = metadata
            with self.subTest(metadata=metadata):
                with self.assertRaises(ValueError):
                    MODULE.download(MODULE.ASSETS[0], self.output, self.opener)
                self.assertEqual(self.output.read_bytes(), b"previous executable")

    def test_bad_gzip_with_valid_digest_preserves_output(self):
        self.payload = b"not gzip"
        self.metadata["assets"][0]["digest"] = "sha256:" + hashlib.sha256(self.payload).hexdigest()
        with self.assertRaises(gzip.BadGzipFile):
            MODULE.download(MODULE.ASSETS[0], self.output, self.opener)
        self.assertEqual(self.output.read_bytes(), b"previous executable")


if __name__ == "__main__":
    unittest.main(verbosity=2)
