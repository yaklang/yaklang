"""Bind tracked census to embed matching and immutable archived-source readers."""
from pathlib import Path, PurePosixPath
import argparse, collections, fnmatch, hashlib, importlib.util, json, shlex, zipfile

def sha(data):
    return hashlib.sha256(data).hexdigest()

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", type=Path, required=True)
    ap.add_argument("--inventory", type=Path, required=True)
    ap.add_argument("--zstd", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    args = ap.parse_args()
    args.out.mkdir(exist_ok=False)
    inv = json.loads(args.inventory.read_bytes())
    batch_rel = "common/bin-parser/testdata/corpus-batches"
    index_raw = (args.repo / batch_rel / "index.json").read_bytes()
    assert sha(index_raw) == inv["index_SHA256"]
    index = json.loads(index_raw)
    targets = [batch_rel + "/" + n for n in inv["physical_index_archive_names"]]
    targets += [r["path"] for r in inv["loose_capture_candidates"]]
    matches = []
    directives = []
    for item in inv["go_embed_directives"]:
        if not item["path"].endswith(".go") or not item["source"].startswith("//go:embed "):
            continue
        directives.append(item)
        directory = PurePosixPath(item["path"]).parent
        for target in targets:
            try:
                relative = PurePosixPath(target).relative_to(directory)
            except ValueError:
                continue
            for token in shlex.split(item["source"].removeprefix("//go:embed ")):
                pattern = token.removeprefix("all:").split("/")
                parts = relative.parts
                if len(pattern) <= len(parts) and all(fnmatch.fnmatchcase(part, glob) for part, glob in zip(parts, pattern)):
                    matches.append(dict(target=target, pattern=token, **item))
    physical = []
    pins = {b["file"]: b for b in index["batches"] if not b.get("pack")}
    pins.update({p["file"]: p for p in index.get("packs", [])})
    for name in inv["physical_index_archive_names"]:
        raw = (args.repo / batch_rel / name).read_bytes()
        assert sha(raw) == pins[name]["sha256"]
        physical.append(dict(path=batch_rel + "/" + name, bytes=len(raw), SHA256=sha(raw)))
    helper = args.repo / "common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/natt-source/sources/phase-source/TFTP-refused-peer-source-01/extract_source.py"
    expected_helper_sha = "d53403c09addf8d5ad1c68357726d0d47319cd7a57195201bea1ca066087dc97"
    assert sha(helper.read_bytes()) == expected_helper_sha
    spec = importlib.util.spec_from_file_location("frozen_fixture_reader", helper)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    reader = module.Reader(args.repo, args.zstd)
    inspected = []
    refs = []
    seen = set()
    source_suffixes = {".go", ".py", ".sh", ".mjs", ".yaml", ".yml", ".yak", ".ps1"}
    for batch in index["batches"]:
        archive = None
        for name, pin in batch["members"].items():
            if Path(name).suffix not in source_suffixes or pin["sha256"] in seen:
                continue
            seen.add(pin["sha256"])
            if batch.get("pack"):
                assert batch["pack"] == reader.pack["id"]
                raw, _ = reader.member(batch["id"], name)
            else:
                if archive is None:
                    archive = zipfile.ZipFile(args.repo / batch_rel / batch["file"])
                raw = archive.read(name, pwd=batch["password"].encode())
                assert len(raw) == pin["bytes"] and sha(raw) == pin["sha256"]
            item = dict(batch=batch["id"], member=name, bytes=len(raw), SHA256=sha(raw))
            inspected.append(item)
            text = raw.decode("utf-8", errors="replace")
            hits = []
            for lineno, line in enumerate(text.splitlines(), 1):
                if "corpus-batches" in line or any(n in line for n in inv["physical_index_archive_names"]):
                    hits.append(dict(line=lineno, source=line.strip()[:600]))
            if hits:
                aliases = [a for a, n in batch["aliases"].items() if n == name]
                refs.append(dict(**item, aliases=aliases, references=hits))
        if archive is not None:
            archive.close()
    result = dict(schema=1, no_repository_writes=True, no_Go_or_TShark_or_SDK=True,
                  inventory=str(args.inventory), inventory_SHA256=sha(args.inventory.read_bytes()),
                  index_SHA256=sha(index_raw), exact_physical_archives=physical,
                  prospective_outer_members=len(targets), outer_uncompressed_source_bytes=sum(r["bytes"] for r in physical) + sum(r["bytes"] for r in inv["loose_capture_candidates"]),
                  actual_go_embed_directives=len(directives), embed_matches_for_migrating_targets=matches,
                  frozen_reader=dict(path=str(helper), SHA256=expected_helper_sha),
                  Zstd_DLL=dict(path=str(args.zstd), SHA256=sha(args.zstd.read_bytes())),
                  archived_unique_source_count=len(inspected), archived_unique_source_bytes=sum(r["bytes"] for r in inspected),
                  archived_sources=inspected, archived_direct_archive_references=refs,
                  limits="Only tracked files and pinned logical code sources; generic future consumer paths and unrecognized capture formats are not asserted absent.")
    target = args.out / "closure.json"
    target.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps(dict(path=str(target), SHA256=sha(target.read_bytes()), physical=len(physical), outer_members=len(targets),
                          source_bytes=result["outer_uncompressed_source_bytes"], embed_matches=len(matches),
                          archived_sources=len(inspected), archived_direct_ref_sources=len(refs))))

if __name__ == "__main__":
    main()
