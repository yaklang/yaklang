#!/usr/bin/env python3
"""Close a source-only SSH plan; no SDK/CLI/native replay is invoked."""
import argparse
import hashlib
import json
from pathlib import Path

def sha(p):
    return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):
    return json.loads(Path(p).read_text(encoding="utf-8-sig"))
def put(p, x):
    with p.open("x", encoding="utf-8") as f:
        json.dump(x, f, indent=2, ensure_ascii=False, sort_keys=True)
        f.write("\n")
def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--plan", required=True, type=Path)
    ap.add_argument("--selftest", required=True, type=Path)
    ap.add_argument("--out", required=True, type=Path)
    a = ap.parse_args()
    a.out.mkdir(exist_ok=False)
    manifest, selftest = read(a.plan/"manifest.json"), read(a.selftest/"summary.json")
    assert selftest["result"] == "PASS"
    assert selftest["plan_manifest_SHA256"] == sha(a.plan/"manifest.json")
    assert selftest["candidate_runtime_read"] is False
    for row in manifest["files"] + manifest["primary_manifests"]:
        assert sha(row["path"]) == row["SHA256"]
    cap = Path(manifest["capture"]["path"])
    runtime = {
        "execution_authority": "Parent agent only, after relevant build has fully closed. This file does not execute a replay.",
        "capture": str(cap), "capture_SHA256": sha(cap),
        "native_args": ["replay", str(cap), "-", "--ignore-embedded-secrets"],
        "CLI_args": ["shark", "-r", str(cap), "--json", "--ignore-embedded-secrets"],
        "field_descriptor_args": ["shark", "--list-fields", "--json"],
        "required_receipts": ["events-receipt.json", "yak-receipt.json"],
        "record": [
            "exact source commit, executable SHA256 and build receipt SHA256",
            "exact argv, closed exit code, stdout/stderr SHA256",
            "all source plan/controller/capture input SHA256 before and after",
            "fresh generation output directory; do not overwrite any failed baseline"],
        "baseline": "Closed bins-48 selected by parent; not executed during this source preparation.",
        "projection": [
            "No re-label or generated-ID normalization establishes correctness.",
            "Use physical frame, per-direction TCP sequence, full payload SHA, sourcecase and new-SYN generation anchors.",
            "Unknown selected method preserves observed SSH and literal negotiated name; it provides no standard method layout, host-key fingerprint, or cryptographic verification.",
            "Known method field selection is exact-name allowlisted, not prefix/substring based.",
            "Post-NEWKEYS sentinels are intentionally synthetic protected-phase bytes, not a conforming ciphertext/AEAD fixture.",
            "First wrong guess is ignored; following correct message remains independently parseable.",
        ],
    }
    put(a.out/"runtime-command-plan.json",runtime)
    audit = {
        "scope": "Three source-code defects identified without candidate replay; compact authored source frozen before baseline execution",
        "candidate_output_read": False,
        "real_traffic_capture_count": 0,
        "source_cases": 12, "physical_frames": manifest["counts"]["physical_frames"],
        "known_layout_names": 12, "exact_markers": 4,
        "issues": [
            {"id": "marker-prefix-filter", "source_path": "common/pcapx/pcaputil/bin_parser_ssh.go",
             "functions": ["sshRealKexNames", "sshNegotiate"],
             "proof": "HasPrefix ext-info-/kex-strict- excludes private names which are not the exact protocol markers.",
             "required": "Only ext-info-c, ext-info-s, kex-strict-c-v00@openssh.com and kex-strict-s-v00@openssh.com are removed by this marker classifier.",
             "cases": ["01-ext-info-private-prefix","02-kex-strict-private-prefix","07-standard-curve-markers-and-reuse"]},
            {"id": "selected-layout-by-prefix", "source_path": "common/pcapx/pcaputil/bin_parser_ssh.go",
             "functions": ["consumeMessage", "sshExchangeMessageName", "sshKexFamily"],
             "proof": "Prefix/Contains classifications of private names incorrectly select or describe standard DH/GEX/ECDH methods.",
             "required": "Select known field layouts and standard message meanings by exact supported algorithm name; retain unknown literal name and generic method message number.",
             "cases": ["03-dh-private-prefix","04-gex-private-prefix","05-curve-substring-private"]},
            {"id": "unknown-reply-fingerprint", "source_path": "common/pcapx/pcaputil/bin_parser_ssh.go",
             "functions": ["consumeMessage", "sshParseHostKey"],
             "proof": "Message31 with profile=transport can hash its first arbitrary SSH string as a host key despite no method layout selection.",
             "required": "Publish host-key fields/fingerprint only after an exact known method's complete reply layout has been validated.",
             "cases": ["05-curve-substring-private","06-unknown-reply-no-hostkey"]},
        ],
        "standard_references": [
            {"RFC": 4251, "section": "6 and 7", "claim": "Exact case-sensitive algorithm/private names and method-specific messages30..49"},
            {"RFC": 4253, "section": "7.1, 7.3 and 8", "claim": "Client-preference selection, ignored wrong guess, independent directional NEWKEYS and fixed-group DH framing"},
            {"RFC": 4419, "section": "3, 4 and 5", "claim": "Two exact GEX names and method-specific messages"},
            {"RFC": 5656, "section": "4, 6.1, 6.3 and 7.1", "claim": "ECDH named-curve method construction and message layouts"},
            {"RFC": 8268, "section": "3", "claim": "Additional fixed-group exact names"},
            {"RFC": 8308, "section": "2.1", "claim": "Two exact ext-info indicators; they may appear at any list position"},
            {"RFC": 8731, "section": "3 and Acknowledgements", "claim": "Curve25519 layout and exact LibSSH alias equivalence"},
            {"vendor": "OpenSSH portable V_9_6_P1 PROTOCOL", "section": "1.9", "claim": "Two exact strict-KEX pseudo-algorithms"},
        ],
        "coverage_limits": [
            "The12 allowlist entries describe existing field grammar support, not recommended cryptographic policy.",
            "P-384/P-521/Curve448 are known methods with unselected complete field grammar in this codec.",
            "No real SSH handshake, private key, signature/MAC verification, DH math, authenticated peer, SFTP, or encrypted rekey is established by these authored fixtures.",
            "This compact set does not exhaust truncation, TCP disorder/loss, or every cipher suite; earlier independent source evidence remains separate.",
            "The fully framed unknown private method can contain bytes resembling known fields; resemblance is not enough to select them.",
        ],
        "frozen_manifest_SHA256": sha(a.plan/"manifest.json"),
        "selftest_summary_SHA256": sha(a.selftest/"summary.json"),
        "counts": selftest["counts"],
        "negative_controls": selftest["negative_controls"],
    }
    put(a.out/"audit-summary.json", audit)
    scripts = [Path(__file__).parent/n for n in ("build_ssh_source49.py","check_ssh_source49.py","close_ssh_source49.py")]
    close = {
        "result": "SOURCE_PREPARATION_CLOSED", "candidate_output_read": False,
        "source_only_archive_candidates": [{"path": str(p.resolve()), "SHA256": sha(p), "bytes": p.stat().st_size}
                                           for p in scripts],
        "plan": {"path": str((a.plan/"manifest.json").resolve()), "SHA256": sha(a.plan/"manifest.json")},
        "selftest": {"path": str((a.selftest/"summary.json").resolve()), "SHA256": sha(a.selftest/"summary.json")},
        "audit": {"path": str((a.out/"audit-summary.json").resolve()), "SHA256": sha(a.out/"audit-summary.json")},
        "runtime_plan": {"path": str((a.out/"runtime-command-plan.json").resolve()), "SHA256": sha(a.out/"runtime-command-plan.json")},
        "excluded_from_archive": ["pcap", "raw binary input", "complete source-facts/expected JSON", "primary downloaded RFC/vendor files", "SDK/CLI exports", "keys", "images", "logs"],
    }
    put(a.out/"manifest.json", close)
    print(json.dumps({"manifest":str((a.out/"manifest.json").resolve()), "SHA256":sha(a.out/"manifest.json"),
                      "audit_SHA256":sha(a.out/"audit-summary.json"),"runtime_plan_SHA256":sha(a.out/"runtime-command-plan.json")}))
if __name__ == "__main__":
    main()

