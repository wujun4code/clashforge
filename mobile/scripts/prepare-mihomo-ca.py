#!/usr/bin/env python3
"""Validate and embed a public CA bundle before a mihomo source build.

Usage: prepare-mihomo-ca.py SOURCE_DIR [CA_BUNDLE]
The default matches upstream's Ubuntu build; other hosts must pass an explicit
public PEM bundle. No trust-store discovery, downloads or fallback are used.
"""
import argparse
import hashlib
import os
from pathlib import Path
import re
import ssl
import sys
import tempfile

DEFAULT_BUNDLE = '/etc/ssl/certs/ca-certificates.crt'
CERTIFICATE = re.compile(
    r'-----BEGIN CERTIFICATE-----\s+[A-Za-z0-9+/=\s]+?-----END CERTIFICATE-----')


def prepare(source, bundle):
    target = source / 'component/ca/ca-certificates.crt'
    if not source.is_dir() or not target.parent.is_dir():
        raise ValueError('missing mihomo CA directory')
    data = bundle.read_bytes()
    pem = data.decode('ascii')
    blocks = CERTIFICATE.findall(pem)
    if not blocks or CERTIFICATE.sub('', pem).strip():
        raise ValueError('expected a nonempty certificate-only PEM bundle')
    # An empty SSLContext avoids silently adding the host's default roots.
    context = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    for block in blocks:
        # Check every block: a valid prefix must not hide a malformed suffix.
        context.load_verify_locations(cadata=block)
    count = context.cert_store_stats()['x509']
    if count < 1:
        raise ValueError('no certificates loaded')

    # Validate everything first, then atomically replace the embedded file.
    temporary = None
    try:
        with tempfile.NamedTemporaryFile(dir=target.parent, prefix='.mihomo-ca-',
                                         delete=False) as output:
            temporary = Path(output.name)
            output.write(data)
        temporary.chmod(0o644)
        os.replace(temporary, target)
        temporary = None
    finally:
        if temporary is not None:
            temporary.unlink(missing_ok=True)
    return count, hashlib.sha256(data).hexdigest()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('source_dir', type=Path)
    parser.add_argument('ca_bundle', type=Path, nargs='?', default=Path(DEFAULT_BUNDLE))
    args = parser.parse_args()
    try:
        count, digest = prepare(args.source_dir, args.ca_bundle)
    except (OSError, ValueError, ssl.SSLError):
        # Do not echo file contents, paths, or exception details into CI logs.
        print('ERROR: CA preparation failed; require a readable valid public PEM bundle and mihomo CA directory.',
              file=sys.stderr)
        return 1
    print(f'count={count} sha256={digest}')
    return 0


if __name__ == '__main__':
    sys.exit(main())
