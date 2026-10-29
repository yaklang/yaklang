"""Fresh a82 Windows NAT-T tour with the frozen scoped source02 cell auditor.

Consumes byte-identical source01 plan/cases plus source02 checker. The original
source01 full-screen assertion is preserved as evidence and is never executed
here. Defaults stay 64MiB history/20k packet cache; global partial coverage is
required while the selected ESP pane and raw source/highlight remain complete.
"""
from pathlib import Path
import argparse, hashlib, importlib.util, json, os, subprocess, sys, time, traceback

ROOT = Path(__file__).resolve().parent
SOURCE = "a82acd0fff97ae50dfc1bbb403c16af359d7bf8f"

def sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def read(path):
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))

def write(path, value):
    path.write_text(json.dumps(value, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bins", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    parser.add_argument("--source", choices=[SOURCE], required=True)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    try:
        assert os.name == "nt"
        manifest_path = ROOT / "manifest03.json"
        manifest = read(manifest_path)
        assert manifest["allowed_application_build_source"] == SOURCE
        for name, binding in manifest["frozen_files"].items():
            target = ROOT / name
            assert target.stat().st_size == binding["bytes"] and sha(target) == binding["SHA256"], ("frozen source changed", name)
        for binding in manifest["external_inputs"].values():
            target = Path(binding["path"])
            assert target.stat().st_size == binding["bytes"] and sha(target) == binding["SHA256"], ("external source changed", str(target))
        source_plan = read(ROOT / "frozen-inputs/source01-plan.json")
        case = read(ROOT / "frozen-inputs/source01-cases.json")[source_plan["case"]]
        helper, renderer = Path(source_plan["helper"]), Path(source_plan["renderer"])
        binary, receipt = args.bins / "yak.exe", args.bins / "yak-receipt.json"
        producer = read(receipt)
        assert producer["source"] == SOURCE == args.source and producer["exit_code"] == 0
        assert producer["binary_SHA256"] == sha(binary)
        assert sha(helper) == source_plan["helper_SHA256"]
        assert sha(source_plan["helper_receipt"]) == source_plan["helper_receipt_SHA256"]
        assert sha(renderer) == source_plan["renderer_SHA256"]
        assert sha(case["capture"]) == case["capture_SHA256"] and sha(case["keys"]) == case["keylog_SHA256"]
        # Load only the corrected scoped checker, after all bytes are bound.
        checker_path = ROOT / "frozen-inputs/source02-verify_cells.py"
        spec = importlib.util.spec_from_file_location("frozen_NATT_source02_scoped_checker", checker_path)
        checker = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(checker)
        small = {"driver03": Path(__file__), "source03_manifest": manifest_path,
                 "app_build_receipt": receipt}
        small.update({"frozen:" + name: ROOT / name for name in manifest["frozen_files"] if name != Path(__file__).name})
        small.update({"external:" + name: Path(binding["path"]) for name, binding in manifest["external_inputs"].items()})
        large = {"application": binary, "native_console": helper,
                 "original_capture": Path(case["capture"]), "keylog": Path(case["keys"])}
        snapshots = args.out / "source-snapshots"
        snapshots.mkdir()
        before = {}
        for index, (name, target) in enumerate(small.items()):
            data = target.read_bytes()
            assert len(data) <= 1 << 20
            snapshot_name = "%02d-%s.txt" % (index, target.name)
            (snapshots / snapshot_name).write_bytes(data)
            before[name] = dict(path=str(target), SHA256=hashlib.sha256(data).hexdigest(), bytes=len(data),
                                source_snapshot=snapshot_name)
        for name, target in large.items():
            before[name] = dict(path=str(target), SHA256=sha(target), bytes=target.stat().st_size)
        write(args.out / "inputs-before.json", before)
        folder = args.out / source_plan["case"]
        command = [str(helper), str(folder), str(binary), case["capture"], case["keys"],
                   case["query"], case["field"], "--ignore-embedded-secrets"]
        env = {key: value for key, value in os.environ.items() if not key.startswith("SHARK_QA_")}
        env.update(GOMAXPROCS="1", GOMEMLIMIT="1GiB", SHARK_QA_EXPECT_PROTOCOL="ESP",
                   SHARK_QA_EXPECT_QUERY=case["query"], SHARK_QA_EXPECT_PACKETS="20000",
                   SHARK_QA_EXPECT_ADAPTIVE="1", SHARK_QA_SELECT_FIELD="SPI",
                   SHARK_QA_SELECT_FIELD_VALUE="4102537167")
        started = time.monotonic()
        try:
            run = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, env=env,
                                 creationflags=0x08000000, timeout=180)
            exit_code, stdout, stderr = run.returncode, run.stdout, run.stderr
        except subprocess.TimeoutExpired as error:
            exit_code, stdout = -124, error.stdout or b""
            stderr = (error.stderr or b"") + b"\nNative helper timeout preserved."
        (args.out / "stdout.log").write_bytes(stdout)
        (args.out / "stderr.log").write_bytes(stderr)
        after = {name: dict(value, SHA256=sha(value["path"]), bytes=Path(value["path"]).stat().st_size)
                 for name, value in before.items()}
        write(args.out / "inputs-after.json", after)
        proof = dict(scope=__doc__, source=SOURCE, arguments=command,
                     environment_overrides={key: value for key, value in env.items() if key.startswith("SHARK_QA_")},
                     binary_SHA256=sha(binary), build_receipt_SHA256=sha(receipt), driver_SHA256=sha(__file__),
                     manifest_SHA256=sha(manifest_path), source_before_SHA256=sha(args.out / "inputs-before.json"),
                     source_after_SHA256=sha(args.out / "inputs-after.json"), sources_byte_equal=before == after,
                     exit_code=exit_code, seconds=time.monotonic() - started, visual_review_pending=True,
                     history_budget_default_mib=64, packet_cache_default=20000, budget_overrides_applied=False,
                     original_full_screen_source01_checker_executed=False,
                     executed_cell_checker_source="source02 scoped checker, exact frozen bytes",
                     snapshots={})
        for cells in sorted(folder.glob("*.json")):
            state = read(cells)
            if "Rows" not in state:
                continue
            if "-wait-" in cells.stem and any(p.stem > cells.stem for p in folder.glob(cells.stem.rsplit("-", 1)[0] + "-*.json")):
                continue
            rendered = cells.with_suffix(".png")
            render = subprocess.run([sys.executable, "-B", str(renderer), str(cells), str(rendered)],
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, creationflags=0x08000000)
            assert render.returncode == 0, (cells.name, "renderer failed")
            proof["snapshots"][cells.stem] = dict(cells_SHA256=sha(cells), PNG_SHA256=sha(rendered))
        write(args.out / "proof.json", proof)
        assert before == after, "Input bytes changed; preserve failure and use a new generation"
        assert exit_code == 0, "Native helper failed; retain all output and cells"
        proof["source_cell_verification"] = checker.verify(folder, case)
        proof["passed"] = True
        write(args.out / "proof.json", proof)
        print(json.dumps(dict(proof=str(args.out / "proof.json"), SHA256=sha(args.out / "proof.json"),
                              passed=True, visual_review_pending=True)))
    except Exception:
        write(args.out / "failure.json", dict(failure=traceback.format_exc(), no_success_certificate=True))
        raise

if __name__ == "__main__":
    main()
