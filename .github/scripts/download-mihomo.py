#!/usr/bin/env python3
"""Fetch the pinned Mihomo asset and require GitHub's SHA256 asset digest.

No target binary is executed (arm64 packaging also runs on amd64 runners).
The digest authenticates bytes against GitHub release metadata, not a signature.
"""
import argparse
import gzip
import hashlib
import json
from pathlib import Path
import re
import shutil
import tempfile
import urllib.request

VERSION = "v1.19.32"
API = f"https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/{VERSION}"
BASE = f"https://github.com/MetaCubeX/mihomo/releases/download/{VERSION}/"
ASSETS = ("mihomo-linux-amd64-compatible", "mihomo-linux-arm64")


def download(asset, output, opener=urllib.request.urlopen):
    if asset not in ASSETS:
        raise ValueError(f"unsupported asset: {asset}")
    request = urllib.request.Request(API, headers={"Accept": "application/vnd.github+json", "User-Agent": "ClashForge-release"})
    with opener(request, timeout=60) as response:
        metadata = json.load(response)
    if metadata.get("tag_name") != VERSION:
        raise ValueError("release tag does not match pinned version")
    name = f"{asset}-{VERSION}.gz"
    matches = [item for item in metadata.get("assets", []) if item.get("name") == name]
    if len(matches) != 1:
        raise ValueError(f"expected exactly one release asset: {name}")
    item = matches[0]
    digest = item.get("digest")
    if not isinstance(digest, str) or not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
        raise ValueError(f"missing/invalid GitHub SHA256 digest for {name}")
    url = BASE + name
    if item.get("browser_download_url") != url:
        raise ValueError("release asset URL does not match pinned tag/name")
    output = Path(output)
    output.parent.mkdir(parents=True, exist_ok=True)
    # Same-directory staging: failed downloads/digests/decompression preserve any
    # prior output, and no unverified executable is exposed at the output path.
    with tempfile.TemporaryDirectory(prefix=".mihomo-", dir=output.parent) as temporary:
        compressed = Path(temporary) / "asset.gz"
        checksum = hashlib.sha256()
        with opener(url, timeout=60) as response, compressed.open("wb") as dest:
            while chunk := response.read(1024 * 1024):
                checksum.update(chunk)
                dest.write(chunk)
        actual = "sha256:" + checksum.hexdigest()
        if actual != digest:
            raise ValueError(f"GitHub asset digest mismatch: expected {digest}, got {actual}")
        staged = Path(temporary) / "mihomo"
        with gzip.open(compressed, "rb") as src, staged.open("wb") as dest:
            shutil.copyfileobj(src, dest)
        if staged.stat().st_size == 0:
            raise ValueError("empty decompressed Mihomo asset")
        staged.chmod(0o755)
        staged.replace(output)
    print(f"verified {name} {actual} -> {output}")
    return actual


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("asset", choices=ASSETS)
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    download(args.asset, args.output)


if __name__ == "__main__":
    main()
