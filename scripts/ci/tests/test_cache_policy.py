import json
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

CI_DIR = Path(__file__).resolve().parents[1]


class CachePruneTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.deleted = self.root / 'deleted'
        curl = self.root / 'curl'
        curl.write_text('''#!/usr/bin/env python3
import json, os, sys
from pathlib import Path
from urllib.parse import parse_qs, urlparse
root = Path(os.environ['MOCK_CACHE_ROOT'])
url = sys.argv[-1]
if 'DELETE' in sys.argv:
    with (root / 'deleted').open('a') as f:
        f.write(url.rsplit('/', 1)[1] + '\\n')
else:
    page = parse_qs(urlparse(url).query)['page'][0]
    if page == os.environ.get('MOCK_FAIL_PAGE'):
        sys.exit(22)
    print((root / ('page-' + page + '.json')).read_text())
''')
        curl.chmod(0o755)

    def entry(self, cid, key, ref='refs/pull/1/merge', created='2026-10-06T00:00:00Z', size=1):
        return dict(id=cid, key='Linux-essential-gobuild-' + key, ref=ref,
                    created_at=created, last_accessed_at=created, size_in_bytes=size)

    def run_prune(self, pages, **settings):
        for n, page in enumerate(pages, 1):
            (self.root / f'page-{n}.json').write_text(json.dumps(dict(actions_caches=page)))
        env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ['PATH'],
                   MOCK_CACHE_ROOT=str(self.root), GITHUB_TOKEN='mock', REPOSITORY='test/repo',
                   CACHE_KEY_PREFIX='Linux-essential-gobuild-', CACHE_KEEP='0',
                   CACHE_MAX_AGE_DAYS='0', CACHE_MAX_SIZE_GB='0',
                   CACHE_PROTECTED_REF='refs/heads/main', GITHUB_REF='refs/pull/2/merge')
        env.update(settings)
        result = subprocess.run(['bash', str(CI_DIR / 'cache-prune.sh')], env=env,
                                text=True, capture_output=True)
        ids = self.deleted.read_text().splitlines() if self.deleted.exists() else []
        return result, ids

    def test_global_pagination_keeps_newest_role_snapshot(self):
        old = self.entry(1, 'v2-X64-go1.22-deps-ssa-2026-W40')
        new = self.entry(2, 'v2-X64-go1.22-deps-ssa-2026-W41', created='2026-10-07T00:00:00Z')
        # Page-local sorting must not let a newer snapshot on a later page be deleted.
        filler = [dict(old, id=n, key='unrelated') for n in range(100, 199)]
        result, ids = self.run_prune([[old] + filler, [new]])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(ids, ['1'])

    def test_size_pressure_preserves_main_current_ref_and_role_boundaries(self):
        keys = ['ssa', 'utils-core', 'utils-tooling']
        entries = [self.entry(n, f'v2-X64-go1.22-deps-{role}-2026-W41',
                              ref='refs/heads/main', size=1024**3)
                   for n, role in enumerate(keys, 1)]
        entries += [self.entry(4, 'v2-X64-go1.22-deps-ssa-2026-W41',
                               ref='refs/pull/2/merge', size=1024**3),
                    self.entry(5, 'v2-X64-go1.22-deps-ssa-2026-W41', size=1024**3)]
        result, ids = self.run_prune([entries], CACHE_MAX_SIZE_GB='1')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(ids, ['5'])
        self.assertIn('Protected caches exceed', result.stdout)

    def test_failed_inventory_never_deletes(self):
        entries = [self.entry(n, 'legacy-base-' + 'a' * 40 + f'-{n}') for n in range(100)]
        result, ids = self.run_prune([entries, []], MOCK_FAIL_PAGE='2')
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(ids, [])

    def test_incremental_pr_heads_replace_only_the_same_role_and_ref(self):
        old = self.entry(1, 'v2-X64-go1.22-deps-ssa-' + 'a' * 40)
        new = self.entry(2, 'v2-X64-go1.22-deps-ssa-' + 'b' * 40,
                         created='2026-10-07T00:00:00Z')
        other_role = self.entry(3, 'v2-X64-go1.22-deps-utils-core-' + 'a' * 40)
        other_ref = dict(old, id=4, ref='refs/pull/3/merge')
        result, ids = self.run_prune([[old, new, other_role, other_ref]])
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(ids, ['1'])

    def test_age_removes_expired_snapshots_without_touching_other_prefixes(self):
        stale = self.entry(1, 'old', created='2000-01-01T00:00:00Z')
        other = dict(stale, id=2, key='other-workflow')
        result, ids = self.run_prune([[stale, other]], CACHE_MAX_AGE_DAYS='14')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(ids, ['1'])


class CompileOnlyRunnerTest(unittest.TestCase):
    def test_warming_does_not_execute_test_init_and_discards_binary(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            package = root / 'common' / 'fixture'
            package.mkdir(parents=True)
            (root / 'go.mod').write_text('module github.com/yaklang/yaklang\n\ngo 1.22\n')
            (package / 'fixture_test.go').write_text('''package fixture
import ("os"; "testing")
func init() { os.WriteFile(os.Getenv("INIT_MARKER"), []byte("ran"), 0600) }
func TestMustFailIfExecuted(t *testing.T) { t.Fatal("warming executed an assertion") }
''')
            config = root / 'config.json'
            config.write_text(json.dumps([dict(package='./common/fixture', race=True)]))
            marker = root / 'init-ran'
            env = dict(os.environ, TEST_CONFIG=str(config), TEST_LOG_DIR=str(root / 'logs'),
                       TEST_COMPILE_ONLY='1', PACKAGE_WORKERS='2', PACKAGE_PARALLEL='1',
                       INIT_MARKER=str(marker), GOWORK='off')
            command = ['bash', str(CI_DIR / 'test-run-pkg.sh')]
            result = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True)
            self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
            self.assertFalse(marker.exists())
            self.assertEqual(list((root / 'logs').glob('*.test')), [])
            # The same selection must really execute and fail in ordinary PR mode.
            env['TEST_COMPILE_ONLY'] = '0'
            result = subprocess.run(command, cwd=root, env=env, text=True, capture_output=True)
            self.assertNotEqual(result.returncode, 0)
            self.assertTrue(marker.exists())
            log = next((root / 'logs').glob('*.run.log')).read_text()
            self.assertIn('warming executed an assertion', log)
