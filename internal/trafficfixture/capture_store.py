"""Read the one hash-pinned physical capture store without extracting sources."""
from functools import lru_cache
import hashlib
import io
import json
from pathlib import Path, PurePosixPath
import re
import stat
import zipfile

MANIFEST = "internal/trafficfixture/testdata/captures.manifest.json"
ARCHIVE = "internal/trafficfixture/testdata/captures.zip"
LIMIT = 64 << 20


def digest(raw):
    return hashlib.sha256(raw).hexdigest()


def strict_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate capture manifest key")
        result[key] = value
    return result


@lru_cache(maxsize=4)
def store(repository):
    root = Path(repository).resolve()
    assert (root / MANIFEST).stat().st_size <= 1 << 20
    index = json.loads((root / MANIFEST).read_bytes(), object_pairs_hook=strict_object)
    assert set(index) == {"version", "archive", "archive_bytes", "archive_sha256", "source_head", "members"}
    assert type(index["version"]) is int and type(index["archive_bytes"]) is int
    assert index["version"] == 1 and index["archive"] == ARCHIVE
    assert re.fullmatch("[0-9a-f]{40}", index["source_head"])
    assert re.fullmatch("[0-9a-f]{64}", index["archive_sha256"])
    assert 0 < len(index["members"]) <= 1024
    assert 0 < (root / ARCHIVE).stat().st_size <= LIMIT
    raw = (root / ARCHIVE).read_bytes()
    assert len(raw) == index["archive_bytes"] and digest(raw) == index["archive_sha256"]
    archive = zipfile.ZipFile(io.BytesIO(raw))
    entries = archive.infolist()
    names = [f.filename for f in entries]
    assert names == sorted(index["members"]) and len(names) == len(set(n.lower() for n in names))
    assert sum(f.file_size for f in entries) <= LIMIT
    for entry in entries:
        name = entry.filename
        assert not name.startswith("/") and "\\" not in name and ":" not in name
        assert all(p not in ("", ".", "..") for p in name.split("/"))
        assert not entry.is_dir() and entry.compress_type == zipfile.ZIP_STORED and not entry.flag_bits & 1
        assert stat.S_ISREG(entry.external_attr >> 16)
        member = index["members"][name]
        assert set(member) == {"bytes", "sha256"} and member["bytes"] == entry.file_size
        assert type(member["bytes"]) is int and 0 <= member["bytes"] <= LIMIT
        assert re.fullmatch("[0-9a-f]{64}", member["sha256"])
    return archive, index


def read_repository_file(repository, relative):
    root = Path(repository).resolve()
    relative = str(relative).replace("\\", "/")
    assert not relative.startswith("/") and ":" not in relative
    assert all(p not in ("", ".", "..") for p in relative.split("/"))
    if not (root / MANIFEST).exists():
        return (root / relative).read_bytes()  # Explicit exported compatibility workspace.
    archive, index = store(str(root))
    if relative not in index["members"]:
        return (root / relative).read_bytes()
    raw = archive.read(relative)
    member = index["members"][relative]
    assert len(raw) == member["bytes"] and digest(raw) == member["sha256"]
    return raw
