"""Fresh literal Wireshark OSPFv3 reference over the 37 unmodified inputs.

Only packet fields/PDML and closed invocation metadata are retained externally.
No TLS keylog is provided, no native output or candidate codec is read, and
every source/binary/controller SHA is checked before and after execution.
"""
from pathlib import Path
import argparse, hashlib, json, os, subprocess, time, traceback, xml.etree.ElementTree as ET

def sha(path):
    h = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            h.update(block)
    return h.hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ("inputs", "tshark", "out"):
        parser.add_argument("--" + name, type=Path, required=True)
    a = parser.parse_args()
    a.out.mkdir(exist_ok=False)
    config = a.out / "empty-config"; config.mkdir()
    environment = dict(os.environ, WIRESHARK_CONFIG_DIR=str(config))
    inputs = json.loads(a.inputs.read_bytes())
    receipts = []
    try:
        for source in inputs["inputs"]:
            capture = Path(source["capture"])
            assert sha(capture) == source["capture_SHA256"]
            folder = a.out / ("input-%02d" % source["input"]); folder.mkdir()
            output, errors = folder / "ospfv3.pdml", folder / "stderr.log"
            args = [str(a.tshark), "-n", "-2", "-r", str(capture), "-Y", "ospf.version == 3", "-T", "pdml"]
            before = {str(path): sha(path) for path in (a.inputs, a.tshark, capture, Path(__file__))}
            start = time.monotonic()
            with output.open("xb") as stdout, errors.open("xb") as stderr:
                run = subprocess.run(args, stdout=stdout, stderr=stderr, env=environment, creationflags=0x08000000, timeout=90)
            after = {name: sha(Path(name)) for name in before}
            receipt = dict(input=source["input"], arguments=args, exit_code=run.returncode, seconds=time.monotonic() - start,
                           source_before=before, source_after=after, source_bytes_equal=before == after,
                           PDML_SHA256=sha(output), stderr_SHA256=sha(errors), keylog_supplied=False)
            assert before == after and run.returncode == 0
            selected = []
            for packet in ET.parse(output).getroot().findall("packet"):
                number = int(packet.find(".//field[@name='frame.number']").get("show"))
                version = packet.find(".//field[@name='ospf.version']")
                assert version is not None and version.get("show") == "3"
                selected.append(number)
            receipt["OSPFv3_physical_frames"] = selected
            receipt["OSPFv3_frame_count"] = len(selected)
            (folder / "receipt.json").write_text(json.dumps(receipt, indent=2) + "\n", encoding="utf-8")
            receipts.append(receipt)
            print(json.dumps(dict(input=source["input"], OSPFv3_frames=len(selected))), flush=True)
        result = dict(scope=__doc__, input_count=37, inputs_SHA256=sha(a.inputs), reference_executable=str(a.tshark),
                      reference_executable_SHA256=sha(a.tshark), driver_SHA256=sha(Path(__file__)),
                      receipts=receipts, total_reference_OSPFv3_frames=sum(row["OSPFv3_frame_count"] for row in receipts),
                      no_native_or_future_codec_output_read=True, original_labels_not_normalized=True)
        target = a.out / "summary.json"
        target.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
        print(json.dumps(dict(summary=str(target), SHA256=sha(target), reference_frames=result["total_reference_OSPFv3_frames"])))
    except Exception:
        (a.out / "failure.json").write_text(json.dumps(dict(traceback=traceback.format_exc(), closed_receipts=receipts), indent=2) + "\n", encoding="utf-8")
        raise

if __name__ == "__main__":
    main()
