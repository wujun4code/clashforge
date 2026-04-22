#!/usr/bin/env python3
"""
build_ipk.py — Build an opkg-compatible IPK package for OpenWrt
OpenWrt IPK format: outer .tar.gz containing:
  ./debian-binary
  ./data.tar.gz
  ./control.tar.gz
NOT the Debian ar format!

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

            tar.add(full, arcname=name, recursive=True, filter=_filt)
    print(f"  wrote {out_path} ({os.path.getsize(out_path):,} bytes)")


# ── data.tar.gz — package contents ────────────────────────────────────────
data_names = []
for root, dirs, files in os.walk('ipk'):
    for f in files:
        p = os.path.relpath(os.path.join(root, f), 'ipk')
        if not p.startswith('CONTROL'):
            data_names.append(p)

make_tar_gz('data.tar.gz', 'ipk', data_names)

# ── control.tar.gz — package metadata ─────────────────────────────────────
ctrl_order = ['control', 'conffiles', 'postinst', 'prerm', 'postrm']
ctrl_names = [f for f in ctrl_order if os.path.exists(f'ipk/CONTROL/{f}')]
make_tar_gz('control.tar.gz', 'ipk/CONTROL', ctrl_names)

# ── debian-binary ──────────────────────────────────────────────────────────
with open('debian-binary', 'w') as fh:
    fh.write('2.0\n')

# ── Outer tar.gz (OpenWrt IPK format) ─────────────────────────────────────
# OpenWrt IPK = tar.gz containing: debian-binary, data.tar.gz, control.tar.gz
# Order matters: debian-binary first, then data, then control
with tarfile.open(pkg_name, 'w:gz', compresslevel=9) as outer:
    for fname in ['debian-binary', 'data.tar.gz', 'control.tar.gz']:
        ti = tarfile.TarInfo(name=fname)
        ti.size = os.path.getsize(fname)
        ti.uid = ti.gid = 0
        ti.uname = ti.gname = 'root'
        ti.mtime = 0
        ti.mode = 0o100644
        with open(fname, 'rb') as fh:
            outer.addfile(ti, fh)

final_size = os.path.getsize(pkg_name)
print(f"Built {pkg_name} ({final_size:,} bytes)")

# ── Verify ─────────────────────────────────────────────────────────────────
import subprocess
result = subprocess.run(['tar', 'tzf', pkg_name], capture_output=True, text=True)
print(f"IPK contents: {result.stdout.strip()}")
if 'debian-binary' not in result.stdout or 'data.tar.gz' not in result.stdout:
    print("ERROR: IPK verification failed!", file=sys.stderr)
    sys.exit(1)

print("IPK built successfully (OpenWrt tar.gz format).")
