#!/usr/bin/env python3
"""Secret-safe assertions for the real subscription traffic smoke test."""
import json
import os
import sys
from urllib.parse import quote


def expected_node():
    return os.environ.get('E2E_EXPECTED_NODE', 'Tokyo-AnyTLS')


def selector_path(data):
    proxies = data['proxies']
    node = expected_node()
    expected_type = os.environ.get('E2E_EXPECTED_TYPE', 'anytls').lower()
    assert proxies[node]['type'].lower() == expected_type
    # At least one real node, never a fixed three-node requirement.
    assert sum(p.get('type', '').lower() == expected_type for p in proxies.values()) >= 1
    for name, group in proxies.items():
        if name != 'GLOBAL' and group.get('type') == 'Selector' and node in group.get('all', []):
            return quote(name, safe='')
    raise AssertionError('no selector containing expected node')


def assert_selection(data):
    assert data.get('type') == 'Selector'
    assert data.get('now') == expected_node()
    assert expected_node() in data.get('all', [])


def assert_geo(data):
    assert data.get('country', '').upper() == os.environ.get('E2E_EXPECTED_COUNTRY', 'JP').upper()
    assert data.get('city', '').casefold() == os.environ.get('E2E_EXPECTED_CITY', 'Tokyo').casefold()


def main():
    try:
        data = json.load(sys.stdin)
        mode = sys.argv[1]
        if mode == 'selector':
            print(selector_path(data))
        elif mode == 'selection':
            assert_selection(data)
        elif mode == 'geo':
            assert_geo(data)
        else:
            raise ValueError('unknown contract')
    except (AssertionError, AttributeError, KeyError, TypeError, ValueError, IndexError) as error:
        # Never echo raw API responses, URLs or private node fields.
        print('subscription contract failed: ' + type(error).__name__, file=sys.stderr)
        return 1
    return 0


if __name__ == '__main__':
    sys.exit(main())
