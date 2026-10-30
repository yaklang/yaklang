"""Write aggregate metadata and a source-only import list; no fixture payloads."""
from pathlib import Path
import collections, hashlib, json

BASE = Path(__file__).resolve().parent

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def main():
    out = BASE / "stage01"
    out.mkdir(exist_ok=False)
    inventory_path = BASE / "result01/inventory.json"
    closure_path = BASE / "closure01/closure.json"
    inventory = json.loads(inventory_path.read_bytes())
    closure = json.loads(closure_path.read_bytes())
    assert closure["inventory_SHA256"] == sha(inventory_path.read_bytes())
    sources = []
    for name, role in (
        ("inventory.py", "Tracked-file magic/ZIP/gzip prefix and consumer census; metadata output only"),
        ("close_audit.py", "Verify old ZIP pins, lexical Go embed matches and pinned archived source references"),
        ("write_report.py", "Aggregate counts/report/source-only import list from the two closed read-only proofs"),
    ):
        file = BASE / name
        sources.append(dict(path=str(file), bytes=file.stat().st_size, SHA256=sha(file.read_bytes()), role=role))
    result = dict(schema=1, audit="M1 physical capture fixture consumer census", source_HEAD=inventory["HEAD"],
                  repository_modified_by_audit=False, Go_build_test_SDK_TShark_started=False,
                  tracked_files=inventory["tracked_files"], index_SHA256=inventory["index_SHA256"],
                  logical_batches=inventory["logical_batches"], packs=inventory["packs"],
                  physical_corpus_archives=len(closure["exact_physical_archives"]),
                  existing_corpus_ZIP_bytes=sum(r["bytes"] for r in closure["exact_physical_archives"]),
                  loose_capture_files=len(inventory["loose_capture_candidates"]),
                  loose_capture_bytes=sum(r["bytes"] for r in inventory["loose_capture_candidates"]),
                  prospective_outer_members=closure["prospective_outer_members"],
                  prospective_outer_source_bytes=closure["outer_uncompressed_source_bytes"],
                  tracked_ZIPs=len(inventory["ZIPs"]),
                  nontraffic_ZIPs=len(inventory["ZIPs"]) - len(closure["exact_physical_archives"]),
                  lexical_go_embed_directives=closure["actual_go_embed_directives"],
                  embed_matches_for_migrating_targets=len(closure["embed_matches_for_migrating_targets"]),
                  inspected_unique_archived_sources=closure["archived_unique_source_count"],
                  inspected_unique_archived_source_bytes=closure["archived_unique_source_bytes"],
                  archived_directory_reference_sources=len(closure["archived_direct_archive_references"]),
                  archived_reference_basenames=dict(collections.Counter(Path(r["member"]).name for r in closure["archived_direct_archive_references"])),
                  errors=len(inventory["errors"]),
                  original_proofs=[dict(path=str(p), SHA256=sha(p.read_bytes())) for p in (inventory_path, closure_path)],
                  unchanged_dependency=closure["frozen_reader"], source_only=sources,
                  source_only_exclusions="No PCAP/PCAPNG, raw packets, keylogs, complete fixture/index/expected data, ZIPs, DLLs, tools, logs or exports are in the proposed import list.",
                  limits=[inventory["limits"], closure["limits"]],
                  production_store_review="Initial draft read-only review only; no new Go/Python store runtime or tests were executed by this child. Root is implementing corrections separately.",
                  M2="Unverified CIFS/SMB changes parked by root in stash6914ec2fa5ceb305fd3a1c4709c92841a5fa42fa and external M2-park-before-M1-55-02; no M2 test pass is claimed.")
    summary = out / "aggregate-summary.json"
    summary.write_text(json.dumps(result, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    report = """M1 can change only the physical storage layout while keeping the old corpus index, batch ZIP bytes, passwords, member hashes, aliases, frozen answers and provenance unchanged.

The closed read-only census covers 17,198 tracked files, 122 ZIP directories, gzip/header signatures and 1,518 unique hash-pinned archived sources (7,286,501 bytes). The traffic subset is exactly 36 existing ZIPs (18,703,582 bytes) plus two loose captures (203,649 bytes), so a preserving outer ZIP has 38 source members totalling 18,907,231 bytes before its own ZIP framing. All 36 old compressed ZIP SHA256 values match the original index. The index still has 288 logical batches, one pack, 24,826 member contracts and 25,174 aliases. The other 86 ZIPs belong to unrelated modules and are outside this migration.

Only the known-signature scan is asserted: no third tracked loose PCAP/PCAPNG/Snoop/Netmon header was found, and no capture-named/magic member was found in the other ZIPs. Encrypted ZIP payloads, unknown capture formats and arbitrary nested non-ZIP archives are not magically proven absent. All indexed archived source reads were hash checked; no packet payload was written by this audit. Lexical matching of 318 real Go embed directive lines against all 38 migrating paths found zero matches.

The shared readBoundedFile routing point can cover four existing physical archive uses without rewriting their contents: archive.go load(), pack.go openPack(), pack.go SourceArchive() for direct batches, and workspace.go exportNames(). The latter must still materialize the 36 old ZIPs at their original paths in an exported compatibility workspace. Its go.mod, unchanged corpus index and copied helper sources then continue working without a second outer store. Temporary synthetic pack fixtures must continue using their local disk files.

The direct consumers requiring migration are:

- common/pcapx/pcapdb/pcap_fixture_test.go:46 (compressed comprehensive capture).
- common/ai/aid/aitool/buildinaitools/yakscripttools/test/pcap_analysis_test.go:147 (exact live fixture vs independent authored capture comparison).
- common/ai/aismoking/live/pcap_analysis.yak:14 (manual live-model smoke input).
- common/pcapx/pcapdb/testdata/build_comprehensive.py:15,90-97 (baseline ZIP regeneration input; new output remains private).
- internal/trafficfixture/archive_test.go:59 (tamper tests need the same exact old ZIP bytes through the store).

The frozen natt-source/.../TFTP-refused-peer-source-01/extract_source.py:18-23 also opens the old pack physically. Keep its historical bytes/SHA unchanged; run it with an exported compatibility workspace as --repo, or provide a separate new adapter. Do not relabel the old source certificate. Of 120 archived source files mentioning the batch directory, 112 are old sealing/packaging tools and eight are build/prepare/upgrade/verify recipes. They are preserved sources, not 120 newly executed runtime obligations. The census lists every source hash and reference. Previously absent compressed sources for the 253 consolidated batches do not become physically recoverable merely by adding an outer ZIP.

The initial central-store draft preserves the relevant routing and exported-workspace fallback. The concrete review items sent to root are: bind every builder input to its frozen inventory SHA/length before packaging; make Python's manifest/size/member/mode checks match Go's bounded contract; explicitly verify the two loose members in store verification or report their separate read scope; and document SourceArchive.StorageFile as an inner storage identity after migration rather than a still-existing checkout file. Preserve all old archive integrity/error checks and the no-loose-shadow rule.

CI's current traffic path filter includes internal/trafficfixture/**, so placing the new store there exercises the traffic workflow without widening that filter. A store-only future commit is not selected by Essential's current common/** filter; if both workflows are required for that path, add the narrowly intended Essential trigger and its scope regression. Keep the complete package gate and required-tests-traffic-unified-v191 inventory unchanged. No test was skipped, weakened or executed by this audit.

The source-only import list consists of inventory.py, close_audit.py and write_report.py with exact hashes in aggregate-summary.json. The existing immutable TFTP reader is a dependency reference, not a duplicate import. Do not import complete inventories, archives, captures or expected objects. CIFS/SMB work is separately parked for M2 and has no claimed test result in M1.
"""
    target = out / "report.md"
    target.write_text(report, encoding="utf-8", newline="\n")
    print(json.dumps(dict(summary=str(summary), summary_SHA256=sha(summary.read_bytes()), report=str(target), report_SHA256=sha(target.read_bytes()), sources=len(sources))))

if __name__ == "__main__":
    main()
