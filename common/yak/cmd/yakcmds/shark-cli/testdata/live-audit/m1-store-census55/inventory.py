"""Read-only tracked-fixture/magic and direct archive-consumer census.

No capture contents are exported. Only names, metadata, hashes and source lines
are written. Encrypted ZIP member payloads and unsupported compression are not
claimed inspected; immutable batch/pack index inventories are recorded instead.
"""
from pathlib import Path
import argparse, collections, gzip, hashlib, json, subprocess, zipfile

MAGIC = {
    bytes.fromhex("d4c3b2a1"): "pcap-le-us", bytes.fromhex("a1b2c3d4"): "pcap-be-us",
    bytes.fromhex("4d3cb2a1"): "pcap-le-ns", bytes.fromhex("a1b23c4d"): "pcap-be-ns",
    bytes.fromhex("34cdb2a1"): "pcap-modified-le", bytes.fromhex("a1b2cd34"): "pcap-modified-be",
    bytes.fromhex("0a0d0d0a"): "pcapng", b"snoop\0\0\0": "snoop",
    b"RTSS": "netmon1", b"GMBU": "netmon2",
}
CAP_SUFFIXES = (".pcap", ".pcapng", ".cap", ".snoop", ".erf", ".ncf", ".netmon", ".trc")
TEXT_SUFFIXES = {".go", ".py", ".yak", ".yml", ".yaml", ".sh", ".ps1", ".toml", ".txt", ".md", ".json"}

def sha(data):
    return hashlib.sha256(data).hexdigest()

def kind(prefix):
    return next((label for magic, label in MAGIC.items() if prefix.startswith(magic)), None)

def capture_name(name):
    low = name.lower()
    return low.endswith(CAP_SUFFIXES) or any(low.endswith(s + z) for s in CAP_SUFFIXES for z in (".gz", ".bz2", ".xz", ".zst", ".zip"))

def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--repo", type=Path, required=True)
    ap.add_argument("--out", type=Path, required=True)
    args = ap.parse_args()
    args.out.mkdir(exist_ok=False)
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=args.repo, text=True).strip()
    tracked_raw = subprocess.check_output(["git", "ls-files", "-z"], cwd=args.repo)
    names = [n.decode("utf-8") for n in tracked_raw.split(b"\0") if n]
    base = "common/bin-parser/testdata/corpus-batches/"
    index_raw = (args.repo / base / "index.json").read_bytes()
    index = json.loads(index_raw)
    physical_names = {b["file"] for b in index["batches"] if not b.get("pack")}
    physical_names.update(p["file"] for p in index.get("packs", []))
    source_hits, embed, found, gzip_found, zip_members, zip_info, errors = [], [], [], [], [], [], []
    stat = collections.Counter()
    for name in names:
        path = args.repo / name
        if not path.is_file() or path.is_symlink():
            stat["missing_or_nonregular"] += 1
            continue
        stat["regular_files"] += 1
        try:
            with path.open("rb") as f:
                prefix = f.read(64)
            k = kind(prefix)
            if k or capture_name(name):
                found.append(dict(path=name, bytes=path.stat().st_size, magic=k, SHA256=sha(path.read_bytes()), suffix_capture=capture_name(name)))
            if prefix.startswith(b"\x1f\x8b"):
                try:
                    with gzip.open(path, "rb") as f:
                        gk = kind(f.read(64))
                    if gk:
                        gzip_found.append(dict(path=name, inner_magic=gk, compressed_SHA256=sha(path.read_bytes())))
                except Exception as exc:
                    errors.append(dict(path=name, operation="gzip-prefix", error=type(exc).__name__))
            if name.endswith(".zip") or prefix.startswith((b"PK\x03\x04", b"PK\x05\x06", b"PK\x07\x08")):
                try:
                    counts = collections.Counter()
                    with zipfile.ZipFile(path) as z:
                        for member in z.infolist():
                            counts["members"] += 1
                            if member.is_dir():
                                continue
                            mk = None
                            if member.flag_bits & 1:
                                counts["encrypted_uninspected"] += 1
                            elif member.file_size > 25 << 20:
                                counts["large_uninspected"] += 1
                            else:
                                try:
                                    with z.open(member) as f:
                                        mk = kind(f.read(64))
                                    counts["prefix_inspected"] += 1
                                except Exception:
                                    counts["unsupported_or_bad_uninspected"] += 1
                            if mk or capture_name(member.filename):
                                zip_members.append(dict(archive=name, member=member.filename, bytes=member.file_size, encrypted=bool(member.flag_bits & 1), method=member.compress_type, magic=mk, suffix_capture=capture_name(member.filename)))
                    zip_info.append(dict(path=name, bytes=path.stat().st_size, SHA256=sha(path.read_bytes()), counts=dict(counts)))
                except Exception as exc:
                    errors.append(dict(path=name, operation="ZIP-directory", error=type(exc).__name__))
            if path.suffix.lower() in TEXT_SUFFIXES and path.stat().st_size <= 5 << 20:
                data = path.read_bytes()
                if b"\0" in data:
                    continue
                text = data.decode("utf-8", errors="replace")
                direct = "corpus-batches" in text or "pcap_analysis.pcap" in text or "comprehensive.pcapng.gz" in text or any(p in text for p in physical_names)
                for lineno, line in enumerate(text.splitlines(), 1):
                    if "go:embed" in line:
                        embed.append(dict(path=name, line=lineno, source=line.strip(), SHA256=sha(data)))
                    if direct and ("corpus-batches" in line or "pcap_analysis.pcap" in line or "comprehensive.pcapng.gz" in line or any(p in line for p in physical_names)):
                        source_hits.append(dict(path=name, line=lineno, source=line.strip()[:600], SHA256=sha(data)))
        except Exception as exc:
            errors.append(dict(path=name, operation="file-scan", error=type(exc).__name__))
    result = dict(schema=1, no_repo_writes=True, no_Go_or_packet_decoder=True, HEAD=head,
                  tracked_list_SHA256=sha(tracked_raw), tracked_files=len(names), counters=dict(stat),
                  index_SHA256=sha(index_raw), schema_version=index["schema_version"],
                  logical_batches=len(index["batches"]), packs=len(index.get("packs", [])),
                  physical_index_archive_names=sorted(physical_names), loose_capture_candidates=found,
                  gzip_capture_headers=gzip_found, ZIPs=zip_info, ZIP_capture_named_or_magic_members=zip_members,
                  direct_source_refs=source_hits, go_embed_directives=embed, errors=errors,
                  limits="Known magic signatures only; ZIP directories, bounded non-encrypted member header reads, gzip first64. Nested ZIP/unsupported/encrypted bodies not claimed scanned.")
    target = args.out / "inventory.json"
    target.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    print(json.dumps(dict(path=str(target), SHA256=sha(target.read_bytes()), tracked=len(names), loose=found,
                          gzip=gzip_found, ZIPs=len(zip_info), ZIP_capture_members=len(zip_members),
                          direct_ref_files=len({r["path"] for r in source_hits}), embed_directives=len(embed), errors=len(errors))))

if __name__ == "__main__":
    main()
