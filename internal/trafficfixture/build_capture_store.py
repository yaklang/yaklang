"""Build a deterministic outer ZIP. Never discovers or imports untracked inputs.

Existing historical ZIPs stay byte identical, including their frozen answers,
provenance and licenses. This command does not remove any source file.
Use --output in a private directory; compare before staging the archive.
"""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import zipfile

from capture_store import ARCHIVE, MANIFEST, LIMIT, digest, read_repository_file


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--source-inventory", type=Path)
    args = parser.parse_args()
    root = Path(__file__).resolve().parents[2]
    if args.source_inventory:
        inventory = json.loads(args.source_inventory.read_bytes())
        names = sorted({x["path"] for x in inventory["corpus_ZIPs"] + inventory["suffix_captures"]})
        source_head = inventory["HEAD"]
        tracked = set(subprocess.check_output(["git", "-C", str(root), "ls-files", "-z"]).decode().split("\0"))
        assert all(name in tracked for name in names)
        assert len(names) == len(inventory["corpus_ZIPs"]) + len(inventory["suffix_captures"])
        # Pin original Git blobs too: a path-only inventory cannot authorize
        # silently packaging a modified or untracked working copy.
        frozen = {name: subprocess.check_output(["git", "-C", str(root), "show", source_head + ":" + name]) for name in names}
    else:
        old = json.loads((root / MANIFEST).read_bytes())
        names, source_head = sorted(old["members"]), old["source_head"]
    assert names and len(names) == len(set(n.lower() for n in names))
    assert not args.output.exists() and not args.output.with_suffix(".manifest.json").exists()
    args.output.parent.mkdir(parents=True, exist_ok=True)
    members = {}
    with zipfile.ZipFile(args.output, "x", compression=zipfile.ZIP_STORED) as archive:
        for name in names:
            assert not name.startswith("/") and "\\" not in name and ":" not in name
            assert all(p not in ("", ".", "..") for p in name.split("/"))
            raw = read_repository_file(root, name)
            if args.source_inventory:
                assert raw == frozen[name], "Source differs from the frozen tracked Git blob: " + name
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system, info.external_attr = 3, 0o100644 << 16
            info.compress_type = zipfile.ZIP_STORED
            archive.writestr(info, raw)
            members[name] = {"bytes": len(raw), "sha256": digest(raw)}
    assert args.output.stat().st_size < LIMIT < 100 << 20
    with zipfile.ZipFile(args.output) as archive:
        assert archive.namelist() == names and archive.testzip() is None
        for name in names:
            assert archive.read(name) == read_repository_file(root, name)
    raw = args.output.read_bytes()
    index = dict(version=1, archive=ARCHIVE, archive_bytes=len(raw), archive_sha256=digest(raw), source_head=source_head, members=members)
    args.output.with_suffix(".manifest.json").write_text(json.dumps(index, indent=2, sort_keys=True) + "\n", encoding="utf-8", newline="\n")
    print(json.dumps({"members": len(names), "archive_bytes": len(raw), "archive_sha256": digest(raw), "byte_equal": True}))


if __name__ == "__main__":
    main()
