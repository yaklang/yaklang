"""Append the closed Python reader regression result without rewriting census01."""
from pathlib import Path
import hashlib
import json

BASE = Path(__file__).resolve().parent
REPO = Path("C:/Users/V/.codex/worktrees/shark-protocol-infrastructure/yaklang")

def sha(raw):
    return hashlib.sha256(raw).hexdigest()

def main():
    output = BASE / "stage02"
    output.mkdir(exist_ok=False)
    old = BASE / "stage01/aggregate-summary.json"
    summary = json.loads(old.read_bytes())
    summary["prior_aggregate"] = dict(path=str(old), SHA256=sha(old.read_bytes()))
    receipts = []
    for generation in ("01", "02"):
        path = BASE.parent / ("test-M1-capture-python55-" + generation) / "receipt.json"
        receipt = json.loads(path.read_text(encoding="utf-8-sig"))
        assert receipt["before"] == receipt["after"], "test and reader must remain frozen during execution"
        stderr = path.with_name("stderr.txt")
        assert sha(stderr.read_bytes()) == receipt["stderr_SHA256"]
        log_bytes = stderr.read_bytes()
        text = log_bytes.decode("utf-16" if log_bytes.startswith((b"\xff\xfe", b"\xfe\xff")) else "utf-8-sig")
        if generation == "01":
            assert receipt["exit_code"] == 1 and "failures=3" in text
        else:
            assert receipt["exit_code"] == 0 and "Ran 17 tests" in text and "\nOK" in text
        receipts.append(dict(path=str(path), SHA256=sha(path.read_bytes()), exit_code=receipt["exit_code"],
                             source_before_after=receipt["before"], stderr_SHA256=receipt["stderr_SHA256"]))
    test_path = REPO / "internal/trafficfixture/test_capture_store.py"
    assert sha(test_path.read_bytes()) == receipts[1]["source_before_after"][1]["SHA256"]
    summary["Python_reader_regression"] = dict(
        final_methods_passed=17, final_failed=0,
        initial_failure="Three manifest integer-schema subcases accepted bool/float in Python while Go int/int64 decoding rejects them; initial receipt and red log retained.",
        correction="Root added exact integer types. Child strengthened structural negative controls to invoke store() directly, avoiding a later missing-file false pass.",
        fixtures="Only stdlib-authored small ZIPs/manifests in TemporaryDirectory; no real capture or repository store modified.",
        test_source=dict(path=str(test_path), bytes=test_path.stat().st_size, SHA256=sha(test_path.read_bytes())),
        executions=receipts,
    )
    summary["next_platform_findings"] = [
        "PCAPDB openIndex drive path serialized as file://C:/..., making C: an invalid SQLite authority; root is applying a leading-path-slash fix and real create/rw/ro regressions. No child Go execution/result claimed.",
        "Windows Go FileMode exposes 0666/0444 from READONLY attributes, not a private DACL. Unix 0600 cannot serve as its privacy oracle; protected user+SYSTEM directory ACL and file ACL checks were recommended. Implementation/result remains root-owned.",
    ]
    summary["source_only"] = []
    for name in ("inventory.py", "close_audit.py", "write_report.py", "write_addendum.py"):
        path = BASE / name
        summary["source_only"].append(dict(path=str(path), bytes=path.stat().st_size, SHA256=sha(path.read_bytes())))
    summary["source_only_note"] = "Four external audit/report controllers are import candidates. The ordinary repo unittest is separately referenced, not duplicated. No data, archives, logs, receipts or expected objects belong in the source-only import."
    summary["production_store_review"] = "Initial source review plus 17 final Python regression methods closed. Root Go/Windows/full-suite/CI outcomes are intentionally not inferred here."
    target = output / "aggregate-summary.json"
    target.write_text(json.dumps(summary, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    addendum = output / "report-addendum.md"
    addendum.write_text(
        "The migration census remains the original 926 source observation; stage01 and its two closed proofs are unchanged.\n\n"
        "The Python reader now has 17 passing stdlib unittest methods. Its first execution preserved three bool/float integer-schema failures. After root fixed the reader and the child strengthened structural negative checks to call the store parser directly, a fresh execution passed with reader/test hashes stable before and after. Every control used a test-owned temporary ZIP; none read or modified a real capture.\n\n"
        "The new test is internal/trafficfixture/test_capture_store.py. Only the four external Python audit/report controllers are proposed for a source-only archive; the normal test stays in its ordinary repository path. Complete inventories, archives, captures, logs and receipts remain external.\n\n"
        "Two Windows findings remain separately owned by root: SQLite drive-path URI serialization and equivalent ACL-based private export validation. They must not be declared passed from this Python result. No Go, SDK or TShark process was started by the child.\n",
        encoding="utf-8", newline="\n",
    )
    print(json.dumps(dict(summary=str(target), SHA256=sha(target.read_bytes()), report=str(addendum), report_SHA256=sha(addendum.read_bytes()), external_sources=4, Python_tests_passed=17)))

if __name__ == "__main__":
    main()
