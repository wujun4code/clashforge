#!/usr/bin/env python3
"""Parse workflow graphs/contracts with PyYAML; actionlint is a separate gate."""
from pathlib import Path
import shlex
import unittest
import yaml

ROOT = Path(__file__).resolve().parents[2]

def workflow(name):
    return yaml.safe_load((ROOT / '.github/workflows' / name).read_text())

def runs(job):
    return '\n'.join(step.get('run', '') for step in job['steps'])

class WorkflowContracts(unittest.TestCase):
    def test_all_workflow_topologies(self):
        for path in sorted((ROOT / '.github/workflows').glob('*.yml')):
            with self.subTest(path=path.name):
                jobs = workflow(path.name)['jobs']
                def visit(name, chain):
                    self.assertIn(name, jobs)
                    self.assertNotIn(name, chain, 'cyclic workflow dependency')
                    needs = jobs[name].get('needs', [])
                    if isinstance(needs, str): needs = [needs]
                    for dep in needs: visit(dep, chain + [name])
                for name in jobs: visit(name, [])
                for name, job in jobs.items():
                    if 'steps' not in job: continue
                    checkout = [i for i,s in enumerate(job['steps']) if s.get('uses','').startswith('actions/checkout@')]
                    local = [i for i,s in enumerate(job['steps']) if s.get('uses','').startswith('./') or '.github/scripts/download-mihomo.py' in s.get('run','')]
                    for i in local: self.assertTrue(checkout and min(checkout) < i, f'{path.name}/{name}: local code before checkout')

    def test_quickstart_is_executed_in_both_test_domains(self):
        for file in ('ci.yml','e2e-openwrt.yml'):
            commands = [line.strip() for job in workflow(file)['jobs'].values() if 'steps' in job for line in runs(job).splitlines() if 'go test ' in line and './cmd/clashforge' in line]
            self.assertTrue(commands, file)
            for command in commands:
                self.assertIn('./internal/quickstart', shlex.split(command), file)
                self.assertIn('-p 2', command)

    def test_hyperv_uses_pinned_digest_downloader_before_packaging(self):
        jobs = workflow('openwrt-hyperv-image.yml')['jobs']
        steps = jobs['build-hyperv-image']['steps']
        download = [i for i,s in enumerate(steps) if 'Download mihomo' in s.get('name','')]
        self.assertEqual(len(download),1)
        command = steps[download[0]]['run']
        self.assertIn('python3 .github/scripts/download-mihomo.py mihomo-linux-amd64-compatible /tmp/mihomo',command)
        self.assertIn('MIHOMO_VER=v1.19.32',command)
        self.assertNotIn('releases/latest',command)
        self.assertNotIn('gunzip',command)
        self.assertTrue(any('/tmp/mihomo' in s.get('run','') for s in steps[download[0]+1:]))
        for file in ('release.yml','e2e-openwrt.yml','openwrt-hyperv-image.yml'):
            text = '\n'.join(runs(job) for job in workflow(file)['jobs'].values() if 'steps' in job)
            self.assertIn('.github/scripts/download-mihomo.py',text)
            self.assertNotIn('mihomo-linux-arm64 -v',text)
            self.assertNotIn('mihomo-linux-arm64 --version',text)

if __name__ == '__main__':
    unittest.main(verbosity=2)
