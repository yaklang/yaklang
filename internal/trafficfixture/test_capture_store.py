"""Standard-library controls for the physical store reader; no real captures.

Run with: python3 -B -m unittest discover -s internal/trafficfixture -p test_capture_store.py
Every ZIP and manifest below is authored in a test-owned temporary directory.
"""
import copy
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import stat
import tempfile
import unittest
from unittest import mock
import warnings
import zipfile

_SPEC = importlib.util.spec_from_file_location(
    "capture_store_under_test", Path(__file__).with_name("capture_store.py")
)
capture_store = importlib.util.module_from_spec(_SPEC)
_SPEC.loader.exec_module(capture_store)


class CaptureStoreTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="yak-capture-store-python-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        capture_store.store.cache_clear()
        self.addCleanup(capture_store.store.cache_clear)

    def write_manifest(self, index):
        path = self.root / capture_store.MANIFEST
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(json.dumps(index), encoding="utf-8")
        capture_store.store.cache_clear()

    def fixture(self, entries=None, *, compression=zipfile.ZIP_STORED, mode=stat.S_IFREG | 0o644):
        if entries is None:
            entries = [("captures/a.bin", b"wire"), ("captures/b.bin", b"second")]
        buffer = io.BytesIO()
        members = {}
        with warnings.catch_warnings():
            warnings.simplefilter("ignore", UserWarning)  # An intentional duplicate-name control.
            with zipfile.ZipFile(buffer, "w") as archive:
                for name, raw in entries:
                    info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
                    info.create_system = 3
                    info.external_attr = mode << 16
                    info.compress_type = compression
                    archive.writestr(info, raw)
                    members[name] = {"bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()}
        raw = buffer.getvalue()
        archive_path = self.root / capture_store.ARCHIVE
        archive_path.parent.mkdir(parents=True, exist_ok=True)
        archive_path.write_bytes(raw)
        index = {
            "version": 1,
            "archive": capture_store.ARCHIVE,
            "archive_bytes": len(raw),
            "archive_sha256": hashlib.sha256(raw).hexdigest(),
            "source_head": "92620d72b0c8a1d62ce923e4e69cc1fad5d4c66d",
            "members": members,
        }
        self.write_manifest(index)
        return index, raw

    def assert_rejected(self, relative="captures/a.bin"):
        # Invalid inputs are rejected, rather than loaded through disk fallback.
        with self.assertRaises((AssertionError, ValueError, TypeError, KeyError, OSError, zipfile.BadZipFile)):
            capture_store.read_repository_file(self.root, relative)

    def assert_store_rejected(self):
        # Prove rejection by the inventory/parser itself. A later missing-file
        # error is not evidence that an invalid member or manifest was refused.
        with self.assertRaises((AssertionError, ValueError, TypeError, KeyError, zipfile.BadZipFile)):
            capture_store.store(str(self.root))

    def rewrite_archive_pin(self, raw):
        index = json.loads((self.root / capture_store.MANIFEST).read_bytes())
        (self.root / capture_store.ARCHIVE).write_bytes(raw)
        index["archive_bytes"] = len(raw)
        index["archive_sha256"] = hashlib.sha256(raw).hexdigest()
        self.write_manifest(index)

    def test_valid_store_reads_are_owned_and_do_not_extract(self):
        self.fixture()
        before = {p.relative_to(self.root).as_posix(): p.read_bytes() for p in self.root.rglob("*") if p.is_file()}
        first = capture_store.read_repository_file(self.root, "captures/a.bin")
        self.assertEqual(b"wire", first)
        changed = bytearray(first)
        changed[0] = ord("!")
        self.assertEqual(b"wire", capture_store.read_repository_file(self.root, "captures/a.bin"))
        self.assertEqual(b"second", capture_store.read_repository_file(self.root, r"captures\b.bin"))
        after = {p.relative_to(self.root).as_posix(): p.read_bytes() for p in self.root.rglob("*") if p.is_file()}
        self.assertEqual(before, after, "a read may not write or extract any member")
        self.assertFalse((self.root / "captures").exists())

    def test_indexed_member_cannot_be_shadowed_by_loose_copy(self):
        self.fixture()
        loose = self.root / "captures/a.bin"
        loose.parent.mkdir(parents=True)
        loose.write_bytes(b"untrusted loose copy")
        self.assertEqual(b"wire", capture_store.read_repository_file(self.root, "captures/a.bin"))
        self.assertEqual(b"untrusted loose copy", loose.read_bytes())

    def test_exported_workspace_without_manifest_keeps_disk_compatibility(self):
        loose = self.root / "common/bin-parser/testdata/corpus-batches/history.zip"
        loose.parent.mkdir(parents=True)
        loose.write_bytes(b"historical archive identity")
        self.assertEqual(loose.read_bytes(), capture_store.read_repository_file(self.root, loose.relative_to(self.root)))

    def test_nonmember_source_file_keeps_disk_semantics(self):
        self.fixture()
        loose = self.root / "README.md"
        loose.write_bytes(b"ordinary source")
        self.assertEqual(b"ordinary source", capture_store.read_repository_file(self.root, "README.md"))
        with self.assertRaises(FileNotFoundError):
            capture_store.read_repository_file(self.root, "unknown.bin")

    def test_empty_member_is_observed_not_missing(self):
        self.fixture([("empty.bin", b"")])
        self.assertEqual(b"", capture_store.read_repository_file(self.root, "empty.bin"))

    def test_invalid_caller_paths_are_rejected_in_both_layouts(self):
        paths = ("", ".", "../outside", "/absolute", "C:/outside", "a//b", "a/./b", "a/../b", r"..\outside")
        for installed in (False, True):
            if installed:
                self.fixture()
            for path in paths:
                with self.subTest(store=installed, path=path):
                    self.assert_rejected(path)

    def test_invalid_member_paths_are_not_normalized_into_valid_names(self):
        for name in ("../outside", "/absolute", "C:/outside", "a\\b", "a//b", "a/./b", "a/../b"):
            with self.subTest(name=name):
                self.fixture([(name, b"wire")])
                self.assert_store_rejected()

    def test_duplicate_unsorted_and_case_colliding_members_are_rejected(self):
        for names in (("a.bin", "a.bin"), ("A.bin", "a.bin"), ("b.bin", "a.bin")):
            with self.subTest(names=names):
                self.fixture([(name, b"wire") for name in names])
                self.assert_store_rejected()

    def test_directories_symlinks_and_nonregular_entries_are_rejected(self):
        for mode in (stat.S_IFLNK | 0o777, stat.S_IFDIR | 0o755, stat.S_IFIFO | 0o644):
            with self.subTest(mode=mode):
                self.fixture([("a.bin", b"wire")], mode=mode)
                self.assert_store_rejected()

    def test_compressed_and_encrypted_outer_entries_are_rejected(self):
        self.fixture([("a.bin", b"wire")], compression=zipfile.ZIP_DEFLATED)
        self.assert_store_rejected()
        _, raw = self.fixture([("a.bin", b"wire")])
        encrypted = bytearray(raw)
        local, central = encrypted.index(b"PK\x03\x04"), encrypted.index(b"PK\x01\x02")
        encrypted[local + 6] |= 1
        encrypted[central + 8] |= 1
        self.rewrite_archive_pin(encrypted)
        self.assert_store_rejected()

    def test_manifest_fields_hashes_and_inventory_are_checked(self):
        original, _ = self.fixture()
        changes = (
            lambda d: d.update(version=2),
            lambda d: d.update(archive="other.zip"),
            lambda d: d.update(source_head="not-a-commit"),
            lambda d: d.update(archive_sha256="0" * 64),
            lambda d: d.update(archive_bytes=d["archive_bytes"] + 1),
            lambda d: d.update(unrecognized=True),
            lambda d: d.pop("source_head"),
            lambda d: d.update(members={}),
            lambda d: d["members"].pop("captures/a.bin"),
            lambda d: d["members"].update({"missing.bin": {"bytes": 4, "sha256": "0" * 64}}),
            lambda d: d["members"]["captures/a.bin"].update(bytes=5),
            lambda d: d["members"]["captures/a.bin"].update(bytes=True),
            lambda d: d["members"]["captures/a.bin"].update(sha256="invalid"),
            lambda d: d["members"]["captures/a.bin"].update(unrecognized=True),
        )
        for change in changes:
            with self.subTest(change=change):
                edited = copy.deepcopy(original)
                change(edited)
                self.write_manifest(edited)
                self.assert_store_rejected()

    def test_integer_schema_matches_go_rejecting_bool_and_float(self):
        original, _ = self.fixture()
        for key, value in (("version", True), ("version", 1.0), ("archive_bytes", float(original["archive_bytes"]))):
            with self.subTest(key=key, value=value):
                edited = copy.deepcopy(original)
                edited[key] = value
                self.write_manifest(edited)
                self.assert_store_rejected()

    def test_duplicate_json_keys_at_root_and_member_are_rejected(self):
        original, _ = self.fixture()
        base = json.dumps(original)
        for raw in (base.replace('"version": 1', '"version": 1, "version": 1', 1),
                    base.replace('"bytes": 4', '"bytes": 4, "bytes": 4', 1)):
            with self.subTest(raw_sha=hashlib.sha256(raw.encode()).hexdigest()):
                (self.root / capture_store.MANIFEST).write_text(raw, encoding="utf-8")
                capture_store.store.cache_clear()
                self.assert_store_rejected()

    def test_member_hash_is_checked_in_addition_to_outer_hash(self):
        original, _ = self.fixture()
        original["members"]["captures/a.bin"]["sha256"] = "0" * 64
        self.write_manifest(original)
        self.assert_rejected()

    def test_bad_member_crc_is_not_hidden_by_new_outer_pin(self):
        _, raw = self.fixture([("a.bin", b"wire")])
        corrupt = bytearray(raw)
        # The authored single-member header has no extra fields. Alter one
        # payload byte, then rebind the outer SHA while retaining its old CRC.
        start = 30 + len("a.bin")
        corrupt[start] ^= 1
        self.rewrite_archive_pin(corrupt)
        with self.assertRaises(zipfile.BadZipFile):
            capture_store.read_repository_file(self.root, "a.bin")

    def test_bad_or_missing_store_does_not_fall_back_to_loose_member(self):
        _, raw = self.fixture()
        loose = self.root / "captures/a.bin"
        loose.parent.mkdir(parents=True)
        loose.write_bytes(b"loose")
        (self.root / capture_store.ARCHIVE).write_bytes(raw + b"changed")
        self.assert_rejected()
        (self.root / capture_store.ARCHIVE).unlink()
        capture_store.store.cache_clear()
        self.assert_rejected()

    def test_manifest_archive_and_collection_bounds(self):
        original, raw = self.fixture()
        (self.root / capture_store.MANIFEST).write_bytes(b" " * ((1 << 20) + 1))
        self.assert_store_rejected()
        self.write_manifest(original)
        with mock.patch.object(capture_store, "LIMIT", len(raw) - 1):
            self.assert_store_rejected()
        edited = copy.deepcopy(original)
        edited["members"] = {"member%04d" % i: {"bytes": 0, "sha256": hashlib.sha256(b"").hexdigest()} for i in range(1025)}
        self.write_manifest(edited)
        self.assert_store_rejected()


if __name__ == "__main__":
    unittest.main()
