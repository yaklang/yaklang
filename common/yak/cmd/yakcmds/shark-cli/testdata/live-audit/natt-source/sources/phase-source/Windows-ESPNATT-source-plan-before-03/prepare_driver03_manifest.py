"""Freeze Python-only driver03 against immutable source01/source02 bytes.

Preparation only. Reads no capture/key bytes, writes no Go source and does not
run a compiler, native replay, app, console helper or cell auditor.
"""
from pathlib import Path
import hashlib, json

ROOT = Path(__file__).resolve().parent
PHASE = ROOT.parent
SOURCE = "a82acd0fff97ae50dfc1bbb403c16af359d7bf8f"

def sha(path):
    return hashlib.sha256(Path(path).read_bytes()).hexdigest()

def read(path):
    return json.loads(Path(path).read_text(encoding="utf-8-sig"))

def record(path):
    return dict(path=str(path), bytes=path.stat().st_size, SHA256=sha(path))

old01 = PHASE / "Windows-ESPNATT-source-plan-before-01"
old02 = PHASE / "Windows-ESPNATT-source-plan-before-02"
assert sha(old01 / "manifest01.json") == "f2dff3ccd6c777e39bcd1710b5fdc53cdee529a1ee7188368b26a7844966dd59"
assert sha(old02 / "manifest02.json") == "6cdac0578d3c5bd92dc961df84434c9f03000d580d6209d36f47d4788f99afd3"
assert sha(old02 / "verify_cells.py") == "1268288292a910e843d614c2d2ebfdf9bf489e79558ed5de69911aa1d3fd7fa5"
old_manifest = read(old01 / "manifest01.json")
for name, pin in old_manifest["files"].items():
    path = old01 / name
    assert path.stat().st_size == pin["bytes"] and sha(path) == pin["SHA256"], name
frozen = ROOT / "frozen-inputs"
frozen.mkdir(exist_ok=False)
copies = {
    "source01-plan.json": old01 / "plan.json",
    "source01-cases.json": old01 / "cases.json",
    "source01-manifest.json": old01 / "manifest01.json",
    "source02-manifest.json": old02 / "manifest02.json",
    "source02-verify_cells.py": old02 / "verify_cells.py",
}
for name, path in copies.items():
    (frozen / name).write_bytes(path.read_bytes())
    assert sha(frozen / name) == sha(path)
plan = read(old01 / "plan.json")
external = {
    "original_source01_plan": old01 / "plan.json",
    "original_source01_cases": old01 / "cases.json",
    "original_source01_manifest": old01 / "manifest01.json",
    "original_source01_driver_NOT_executed": old01 / "run_windows_NATT.py",
    "original_source01_full_screen_checker_NOT_executed": old01 / "verify_cells.py",
    "original_source02_manifest": old02 / "manifest02.json",
    "original_source02_scoped_checker": old02 / "verify_cells.py",
    "original_parameter_driver": Path(plan["base_parameter_driver"]),
    "renderer": Path(plan["renderer"]),
    "helper_build_receipt": Path(plan["helper_receipt"]),
}
assert sha(external["original_parameter_driver"]) == plan["base_parameter_driver_SHA256"]
assert sha(external["renderer"]) == plan["renderer_SHA256"]
assert sha(external["helper_build_receipt"]) == plan["helper_receipt_SHA256"]
files = [path for path in ROOT.rglob("*") if path.is_file()]
assert all(path.suffix in (".py", ".json") for path in files)
manifest = dict(scope=__doc__, allowed_application_build_source=SOURCE,
                no_Go_or_UI_execution_during_preparation=True,
                no_new_Go_file_or_archive_written=True,
                frozen_files={path.relative_to(ROOT).as_posix(): {"bytes": path.stat().st_size, "SHA256": sha(path)} for path in sorted(files)},
                external_inputs={name: record(path) for name, path in external.items()},
                exact_original_copies={name: record(path) for name, path in copies.items()},
                actual_checker="frozen-inputs/source02-verify_cells.py",
                old_full_screen_checker_execution_forbidden=True,
                unchanged_default_budgets=dict(protocol_history_mib=64, packet_cache=20000),
                source_constraints=dict(original_frame_bytes=138, ESP_PDU_bytes=96,
                                        selected_SPI_PDU_range=[0, 4], selected_SPI_physical_range=[42, 46],
                                        global_partial_footer_required=True, selected_pane_detail_unavailable_forbidden=True),
                helper_Go_source_registration_only=dict(build_source=plan["helper_build_source"],
                                                        source_SHA256=plan["helper_source_file_SHA256"],
                                                        Go_source_not_copied=True))
target = ROOT / "manifest03.json"
target.write_text(json.dumps(manifest, indent=2) + "\n", encoding="utf-8")
print(json.dumps(dict(manifest=str(target), SHA256=sha(target), driver_SHA256=sha(ROOT / "run_windows_NATT03.py"),
                      checker_SHA256=sha(frozen / "source02-verify_cells.py"), native_or_UI_executed=False)))
