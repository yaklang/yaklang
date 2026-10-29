"""Read-only inventory of retained public replay TFTP source objects.

No future replay output is consumed; the result is an observation inventory,
not an oracle that blesses old classifications. It conservatively identifies
unknown-opcode events after an RRQ and distinguishes first observed server-TID
binding from the initial service endpoint using available successful events.
"""
from pathlib import Path
import argparse, base64, hashlib, json

def sha(data):
    return hashlib.sha256(data).hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--replay", type=Path, action="append", required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    args.out.mkdir(parents=True, exist_ok=False)
    summaries, all_objects = [], []
    for root in args.replay:
        files, count, unknown = [], 0, []
        for path in sorted(root.rglob("*.jsonl")):
            if path.name != "events.jsonl":
                continue
            rows, source_count, transfers = [], 0, {}
            digest = hashlib.sha256()
            with path.open("rb") as stream:
                for line_number, line in enumerate(stream, 1):
                    digest.update(line)
                    source_count += 1
                    if b'"protocol":"tftp"' not in line and b'"protocol": "tftp"' not in line:
                        continue
                    event = json.loads(line)
                    if event.get("protocol") != "tftp":
                        continue
                    count += 1
                    raw = base64.b64decode(event.get("raw") or "")
                    opcode = int.from_bytes(raw[:2], "big") if len(raw) >= 2 else None
                    key = json.dumps([event.get("domain"), event.get("flow")], sort_keys=True)
                    state = transfers.setdefault(key, {"initial_source": None, "initial_destination": None,
                                                       "observed_server": None, "successful": []})
                    direction = event.get("direction")
                    peer = event.get("destination") if direction == 0 else event.get("source")
                    record = dict(file=str(path), line=line_number, id=event.get("id"),
                                  flow=event.get("flow"), direction=direction, source=event.get("source"),
                                  destination=event.get("destination"), status=event.get("status"),
                                  opcode=opcode, raw_length=len(raw), error=event.get("error"),
                                  summary=event.get("summary"), source_bytes=event.get("source_bytes"),
                                  before_state=json.loads(json.dumps(state)), full_event_SHA256=sha(line),
                                  peer_equals_initial_service=peer == state["initial_destination"],
                                  peer_equals_observed_server=peer == state["observed_server"])
                    if opcode not in (1, 2, 3, 4, 5, 6):
                        unknown.append(record)
                    if opcode in (1, 2) and not event.get("error"):
                        state.update(initial_source=event.get("source"), initial_destination=event.get("destination"),
                                     observed_server=None, successful=[])
                    if not event.get("error"):
                        state["successful"].append(event.get("id"))
                        server = (event.get("session") or {}).get("Server Transfer ID")
                        if server:
                            state["observed_server"] = server
                    rows.append(record)
                    all_objects.append(dict(replay=str(root), file=str(path), line=line_number, event=event))
            files.append(dict(file=str(path), SHA256=digest.hexdigest(), source_rows=source_count,
                              TFTP_events=len(rows), observations=rows))
        summaries.append(dict(replay=str(root), source=str(root / "summary.json"),
                              producer_SHA256=sha((root / "summary.json").read_bytes()),
                              TFTP_events=count, unknown_opcode_events=unknown, files=files))
    objects = args.out / "complete-retained-TFTP-objects.json"
    objects.write_text(json.dumps(all_objects, indent=2) + "\n", encoding="utf-8")
    result = dict(scope=__doc__, driver_SHA256=sha(Path(__file__).read_bytes()),
                  no_Go_native_or_future_replay=True, inventory=summaries,
                  complete_objects=str(objects), complete_objects_SHA256=sha(objects.read_bytes()),
                  limitation="Unavailable or previously truncated source state is not inferred. The inventory is not proof of wire admission or protocol correctness.")
    summary = args.out / "summary.json"
    summary.write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(summary), SHA256=sha(summary.read_bytes()),
                          replays=[dict(replay=r["replay"], TFTP_events=r["TFTP_events"],
                                        unknown_opcode_events=len(r["unknown_opcode_events"])) for r in summaries])))

if __name__ == "__main__":
    main()
