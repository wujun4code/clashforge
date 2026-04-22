#!/usr/bin/env python3
"""
build_ipk.py — Build an opkg-compatible IPK package
Usage: PKG_NAME=clashforge_x.x.x_arch.ipk python3 build_ipk.py
Must be run from the directory containing the 'ipk/' staging tree.
"""
import os
import sys
import tarfile

pkg_name = os.environ.get('PKG_NAME')
if not pkg_name:
    print("ERROR: PKG_NAME env var required", file=sys.stderr)
    sys.exit(1)


def make_tar_gz(out_path, base_dir, names):
    """Create a reproducible gzipped tar with uid/gid=0 and mtime=0."""
    with tarfile.open(out_path, 'w:gz', compresslevel=9) as tar:
        for name in sorted(names):
            full = os.path.join(base_dir, name)
            if not os.path.exists(full):
                continue

            def _filt(info):
                info.uid = info.gid = 0
                info.uname = info.gname = 'root'
                info.mtime = 0
                return info

            tar.add(full, arcname='./' + name, recursive=True, filter=_filt)
    print(f"  wrote {out_path} ({os.path.getsize(out_path):,} bytes)")


# ── Collect data members (everything under ipk/ except CONTROL/) ──────────
data_names = []
for root, dirs, files in os.walk('ipk'):
    for f in files:
        p = os.path.relpath(os.path.join(root, f), 'ipk')
        if not p.startswith('CONTROL'):
            data_names.append(p)

make_tar_gz('data.tar.gz', 'ipk', data_names)

# ── Collect control members ────────────────────────────────────────────────
ctrl_order = ['control', 'conffiles', 'postinst', 'prerm', 'postrm']
ctrl_names = [f for f in ctrl_order if os.path.exists(f'ipk/CONTROL/{f}')]
make_tar_gz('control.tar.gz', 'ipk/CONTROL', ctrl_names)

# ── debian-binary ──────────────────────────────────────────────────────────
with open('debian-binary', 'w') as fh:
    fh.write('2.0\n')


# ── Assemble ar archive ────────────────────────────────────────────────────
# ar format: global magic + sequence of members
# Each member header is exactly 60 bytes:
#   name[16] mtime[12] uid[6] gid[6] mode[8] size[10] fmag[2]
def ar_header(name: str, size: int) -> bytes:
    hdr = (
        name.encode().ljust(16)[:16]    # name, padded/truncated to 16
        + b'0           '               # mtime (12 chars)
        + b'0     '                     # uid   (6 chars)
        + b'0     '                     # gid   (6 chars)
        + b'100644  '                   # mode  (8 chars)
        + str(size).encode().ljust(10)[:10]  # size (10 chars)
        + b'`\n'                        # fmag  (2 chars: backtick + newline)
    )
    assert len(hdr) == 60, f"bad header length {len(hdr)}"
    return hdr


members = [
    ('debian-binary',  'debian-binary'),
    ('control.tar.gz', 'control.tar.gz'),
    ('data.tar.gz',    'data.tar.gz'),
]

with open(pkg_name, 'wb') as ipk:
    ipk.write(b'!<arch>\n')           # ar global magic (8 bytes)
    for arc_name, file_name in members:
        with open(file_name, 'rb') as fh:
            data = fh.read()
        ipk.write(ar_header(arc_name, len(data)))
        ipk.write(data)
        if len(data) % 2:              # ar requires even alignment
            ipk.write(b'\n')

final_size = os.path.getsize(pkg_name)
print(f"Built {pkg_name} ({final_size:,} bytes)")

# ── Quick sanity check ─────────────────────────────────────────────────────
import subprocess
result = subprocess.run(['ar', 't', pkg_name], capture_output=True, text=True)
print(f"ar contents: {result.stdout.strip()}")
if 'debian-binary' not in result.stdout:
    print("ERROR: ar verification failed!", file=sys.stderr)
    sys.exit(1)

print("IPK built successfully.")
