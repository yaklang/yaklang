#!/usr/bin/env python3
"""Pack, extract, or verify the enterprise traffic analysis fixtures."""

import argparse
import gzip
import hashlib
import io
import os
from pathlib import Path
import tarfile
import tempfile


REPO_ROOT = Path(__file__).resolve().parents[1]
BASE_DIR = REPO_ROOT / "testdata" / "enterprise-traffic-analysis"
ARCHIVE = BASE_DIR / "fixtures.tar.gz"
CASE_DIRS = (
    "dns-tunnel-c2",
    "host-user-identification",
    "http-dga-c2",
    "https-keylog-dridex",
    "lateral-movement-psexec",
    "redline-infostealer-2023-07",
)


def fixture_files():
    files = []
    for case in CASE_DIRS:
        case_dir = BASE_DIR / case
        if not case_dir.is_dir():
            raise SystemExit(f"missing fixture directory: {case_dir}")
        for path in sorted(case_dir.rglob("*")):
            if path.is_symlink():
                raise SystemExit(f"symlinks are not supported: {path}")
            if not path.is_file() or path.name == ".DS_Store":
                continue
            files.append(path.relative_to(BASE_DIR))
    return files


def add_bytes(archive, name, data, mode):
    info = tarfile.TarInfo(name)
    info.size = len(data)
    info.mode = mode
    info.mtime = 0
    info.uid = 0
    info.gid = 0
    info.uname = ""
    info.gname = ""
    archive.addfile(info, io.BytesIO(data))


def pack():
    files = fixture_files()
    directories = set()
    for relative in files:
        parent = relative.parent
        while parent != Path("."):
            directories.add(parent)
            parent = parent.parent

    ARCHIVE.parent.mkdir(parents=True, exist_ok=True)
    descriptor, temporary = tempfile.mkstemp(
        prefix=ARCHIVE.name + ".", suffix=".tmp", dir=ARCHIVE.parent
    )
    os.close(descriptor)
    try:
        with open(temporary, "wb") as raw:
            with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0, compresslevel=9) as compressed:
                with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as archive:
                    for directory in sorted(directories, key=lambda path: path.as_posix()):
                        info = tarfile.TarInfo(directory.as_posix() + "/")
                        info.type = tarfile.DIRTYPE
                        info.mode = 0o755
                        info.mtime = 0
                        info.uid = 0
                        info.gid = 0
                        info.uname = ""
                        info.gname = ""
                        archive.addfile(info)
                    for relative in files:
                        add_bytes(archive, relative.as_posix(), (BASE_DIR / relative).read_bytes(), 0o644)
        os.chmod(temporary, 0o644)
        os.replace(temporary, ARCHIVE)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)

    digest = hashlib.sha256(ARCHIVE.read_bytes()).hexdigest()
    print(f"packed {len(files)} files -> {ARCHIVE}")
    print(f"size={ARCHIVE.stat().st_size} sha256={digest}")


def safe_target(destination, name):
    relative = Path(name)
    if relative.is_absolute() or ".." in relative.parts:
        raise SystemExit(f"unsafe archive path: {name}")
    target = (destination / relative).resolve()
    root = destination.resolve()
    if target != root and root not in target.parents:
        raise SystemExit(f"archive path escapes destination: {name}")
    return target


def extract(force):
    if not ARCHIVE.is_file():
        raise SystemExit(f"missing archive: {ARCHIVE}")
    BASE_DIR.mkdir(parents=True, exist_ok=True)
    with tarfile.open(ARCHIVE, mode="r:gz") as archive:
        members = archive.getmembers()
        for member in members:
            target = safe_target(BASE_DIR, member.name)
            if member.isdir():
                target.mkdir(parents=True, exist_ok=True)
                continue
            if not member.isfile():
                raise SystemExit(f"unsupported archive entry: {member.name}")
            data = archive.extractfile(member)
            if data is None:
                raise SystemExit(f"cannot read archive entry: {member.name}")
            content = data.read()
            if target.exists() and not target.is_file():
                raise SystemExit(f"target is not a regular file: {target}")
            if target.is_file() and not force:
                existing = target.read_bytes()
                if existing != content:
                    raise SystemExit(
                        f"refusing to overwrite modified file: {target}; use --force after backing up"
                    )
                continue
            target.parent.mkdir(parents=True, exist_ok=True)
            temporary = target.with_name(target.name + ".tmp")
            temporary.write_bytes(content)
            os.chmod(temporary, member.mode & 0o777)
            os.replace(temporary, target)
    print(f"extracted {ARCHIVE} -> {BASE_DIR}")


def archive_hashes():
    result = {}
    with tarfile.open(ARCHIVE, mode="r:gz") as archive:
        for member in archive.getmembers():
            if member.isfile():
                data = archive.extractfile(member)
                if data is None:
                    raise SystemExit(f"cannot read archive entry: {member.name}")
                result[member.name] = hashlib.sha256(data.read()).hexdigest()
    return result


def verify():
    if not ARCHIVE.is_file():
        raise SystemExit(f"missing archive: {ARCHIVE}")
    archived = archive_hashes()
    local = {
        relative.as_posix(): hashlib.sha256((BASE_DIR / relative).read_bytes()).hexdigest()
        for relative in fixture_files()
    }
    if archived != local:
        missing = sorted(set(local) - set(archived))
        extra = sorted(set(archived) - set(local))
        changed = sorted(name for name in set(local) & set(archived) if local[name] != archived[name])
        raise SystemExit(
            "fixture archive differs from expanded files: "
            f"missing={missing} extra={extra} changed={changed}"
        )
    print(f"verified {len(local)} files against {ARCHIVE}")
    print(f"sha256={hashlib.sha256(ARCHIVE.read_bytes()).hexdigest()}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("pack", "extract", "verify"))
    parser.add_argument("--force", action="store_true", help="overwrite modified expanded files during extract")
    args = parser.parse_args()
    if args.action == "pack":
        pack()
    elif args.action == "extract":
        extract(args.force)
    else:
        verify()


if __name__ == "__main__":
    main()
