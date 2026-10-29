"""Verify the two sealed control steps against their original UDP frame bytes.

Only the hash-checked existing captures/control members are consumed. This
does not execute Go, rebuild a parser, rewrite an answer, or stitch UDP bytes.
"""
from pathlib import Path
import argparse, hashlib, importlib.util, ipaddress, json, struct, sys

def sha(data):
    return hashlib.sha256(data).hexdigest()

def endpoint(ip, port):
    return str(ipaddress.ip_address(ip)) + ":" + str(port)

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--extracted", type=Path, required=True)
    parser.add_argument("--reader", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    doc = json.loads((args.extracted / "summary.json").read_bytes())
    assert doc["pack_SHA256"] == "3d35fff4990c80ed608e0880438307f0b2bbe5092dae18166c0d04b928442860"
    for member in doc["extracted_members"]:
        raw = Path(member["path"]).read_bytes()
        assert len(raw) == member["bytes"] and sha(raw) == member["logical_SHA256"]
    sys.path.insert(0, str(args.reader.parent))
    spec = importlib.util.spec_from_file_location("frozen_capture_reader", args.reader)
    reader = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(reader)
    result = []
    for item in doc["complete_selected_cases"]:
        case = item["case"]
        name = case["capture"] if item["batch"] == "validation-tftp-negotiation" else "tftp-file/" + case["capture"]
        capture = args.extracted / item["batch"] / name
        assert sha(capture.read_bytes()) == case["sha256"]
        records = list(reader.capture_records(capture))
        assert len(records) == len(case["events"]) == case["packets"]
        steps = []
        for number, (record, event) in enumerate(zip(records, case["events"]), 1):
            frame = record["wire"]
            assert record["link"] == 1 and frame[12:14] == b"\x08\x00" and frame[14] >> 4 == 4
            ihl = (frame[14] & 15) * 4
            total = int.from_bytes(frame[16:18], "big")
            assert frame[23] == 17 and ihl >= 20 and 14 + total <= len(frame)
            udp_start = 14 + ihl
            src_port, dst_port, udp_length, udp_checksum = struct.unpack_from("!HHHH", frame, udp_start)
            assert udp_length >= 8 and udp_start + udp_length == 14 + total
            payload_start = udp_start + 8
            payload = frame[payload_start:udp_start + udp_length]
            assert payload.hex() == event["input_wire"] == event["raw"]
            assert record["time_ns"] == event["timestamp_sec"] * 1000000000
            assert record["interface"] == event.get("interface", 0) and record["section"] == 0
            assert event["packet_refs"] == [number]
            src = endpoint(frame[26:30], src_port)
            dst = endpoint(frame[30:34], dst_port)
            if "source" in event:
                assert event["source"] == src and event["destination"] == dst
            else:
                direction = event["wire_direction"]
                server_port = event["initial_port"] if payload[:2] in (b"\x00\x01", b"\x00\x02") and direction == 0 else event["server_port"]
                expected_src, expected_dst = "192.0.2.1:" + str(event["client_port"]), "192.0.2.2:" + str(server_port)
                if direction == 1:
                    expected_src, expected_dst = expected_dst, expected_src
                assert (src, dst) == (expected_src, expected_dst)
            opcode = int.from_bytes(payload[:2], "big") if len(payload) >= 2 else None
            steps.append(dict(frame=number, seconds=event["timestamp_sec"], source=src, destination=dst,
                              source_range=[payload_start, udp_start + udp_length],
                              wire_hex=payload.hex(), wire_SHA256=sha(payload), wire_length=len(payload),
                              opcode_prefix=opcode, candidate_3_through_6=opcode in (3, 4, 5, 6),
                              expected_error=event["error"], expected_fields=event["fields"],
                              expected_session=event["session"], expected_flow_owner=event.get("flow_owner", "same nonzero first flow by typed test"),
                              expected_packet_refs=event["packet_refs"], original_frame_SHA256=sha(frame)))
        assert steps[1]["source"] == steps[2]["source"] == "192.0.2.2:50000"
        assert steps[1]["candidate_3_through_6"] and not steps[2]["candidate_3_through_6"]
        assert steps[1]["expected_error"] == steps[2]["expected_error"] == "MalformedMessage"
        result.append(dict(id=case["id"], capture=str(capture), capture_SHA256=sha(capture.read_bytes()),
                           independent_records_equal_expected_input_wire=True, steps=steps,
                           observed_refused_peer="192.0.2.2:50000", client="192.0.2.1:40000",
                           domain=dict(Section=0, Interface=0, Encapsulation=""),
                           first_peer_evidence_at_physical_frame=2,
                           noncandidate_residual_same_peer_at_physical_frame=3,
                           residual_datagrams_must_not_be_joined=True))
    args.out.mkdir(parents=True, exist_ok=False)
    value = dict(scope=__doc__, driver_SHA256=sha(Path(__file__).read_bytes()),
                 extracted_producer_SHA256=sha((args.extracted / "summary.json").read_bytes()),
                 capture_reader=str(args.reader), capture_reader_SHA256=sha(args.reader.read_bytes()),
                 cases=result, no_Go_native_or_future_output=True,
                 conclusion="Exact refused-peer routing must survive one malformed prefix without turning it into a confirmed or retired TID in that generation. Other ports remain unowned; all successful semantics and file evidence still require complete independent UDP messages.")
    target = args.out / "summary.json"
    target.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(target), SHA256=sha(target.read_bytes()), cases=len(result),
                          physical_frames=sum(len(c["steps"]) for c in result))))

if __name__ == "__main__":
    main()
