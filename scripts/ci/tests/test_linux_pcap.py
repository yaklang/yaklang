import os
from pathlib import Path
import subprocess
import tempfile
import unittest


SCRIPT = Path(__file__).resolve().parents[1] / 'configure-linux-pcap.sh'


class LinuxPCapConfigurationTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.libdir = self.root / 'lib'
        self.libdir.mkdir()
        (self.libdir / 'libpcap.so').touch()
        self.github_env = self.root / 'github-env'
        mock = self.root / 'pkg-config'
        mock.write_text('''#!/bin/sh
case "$1" in
  --variable=libdir) printf '%s\\n' "$TEST_PCAP_LIBDIR" ;;
  --modversion) printf '1.10.1\\n' ;;
  *) exit 1 ;;
esac
''')
        mock.chmod(0o755)

    def configure(self, **settings):
        env = dict(os.environ, PATH=str(self.root) + os.pathsep + os.environ['PATH'],
                   GITHUB_ENV=str(self.github_env), TEST_PCAP_LIBDIR=str(self.libdir))
        env.pop('CGO_LDFLAGS', None)
        env.update(settings)
        return subprocess.run(['bash', str(SCRIPT)], env=env, capture_output=True, text=True)

    def test_system_library_precedes_existing_linker_options(self):
        result = self.configure(CGO_LDFLAGS='-L/other/lib -Wl,-z,now')
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.github_env.read_text(),
                         f'CGO_LDFLAGS=-L{self.libdir} -L/other/lib -Wl,-z,now\n')

    def test_empty_flags_are_supported(self):
        result = self.configure()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.github_env.read_text(), f'CGO_LDFLAGS=-L{self.libdir}\n')

    def test_missing_shared_library_does_not_export_flags(self):
        (self.libdir / 'libpcap.so').unlink()
        result = self.configure()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.github_env.exists())

    def test_relative_library_directory_does_not_export_flags(self):
        result = self.configure(TEST_PCAP_LIBDIR='lib')
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.github_env.exists())
