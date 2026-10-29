"""Build valid RADIUS competition controls from the frozen original eight frames.

The original Access-Request omitted NAS identity. Preserve it and all failed
reference runs; append NAS-IP-Address to only records 4/8, update RADIUS and
parent lengths/checksums, then derive C8, A2 and isolated-RRQ B8. NAS identity
is an observed request field, not proof of packet-source identity or integrity.
No native output, Go or future implementation is used by this constructor.
"""
from pathlib import Path
import argparse, hashlib, importlib.util, json, struct

SOURCE_SHA = "78f3537571bc44bb1c1a31f694973ea9d17ca4a1cd53e1b734f6f7e93f944351"
SOURCE_READER_SHA = "bf9fd9c62c9f3837a6bbdcda19310eb87d5a7be545c8262e9ea19c485de71746"

def sha(data):
    return hashlib.sha256(data).hexdigest()

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--reader", type=Path, required=True,
                        help="Frozen source01 prepare_isolation.py; hash is bound below")
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    source = args.input.read_bytes()
    reader_bytes = args.reader.read_bytes()
    assert sha(source) == SOURCE_SHA
    assert sha(reader_bytes) == SOURCE_READER_SHA, (sha(reader_bytes), SOURCE_READER_SHA)
    spec = importlib.util.spec_from_file_location("frozen_isolation_source01", args.reader)
    reader = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(reader)
    header, records = reader.read_pcap(source)
    assert len(records) == 8
    c_records, corrections = [], []
    for number, record in enumerate(records, 1):
        old = reader.inspect(record["frame"])
        expected_name = ["STUN", "CoAP", "TFTP", "RADIUS"][(number - 1) % 4]
        assert old["payload"] == reader.WIRE[expected_name]
        if number not in (4, 8):
            c_records.append(record["full"])
            continue
        assert old["destination_port"] == 1812 and old["payload"][:4] == bytes([1, 7, 0, 26])
        addition = bytes([4, 6, 192, 0, 2, 1])
        payload = old["payload"][:2] + bytes([0, 32]) + old["payload"][4:] + addition
        assert payload[:2] == old["payload"][:2] and payload[4:26] == old["payload"][4:]
        udp_start = old["UDP_start"]
        frame = bytearray(record["frame"][:udp_start + 8] + payload)
        udp_length = len(payload) + 8
        frame[udp_start + 4:udp_start + 6] = udp_length.to_bytes(2, "big")
        frame[udp_start + 6:udp_start + 8] = b"\x00\x00"
        if old["version"] == 4:
            frame[16:18] = (20 + udp_length).to_bytes(2, "big")
            frame[24:26] = b"\x00\x00"
            frame[24:26] = reader.checksum(bytes(frame[14:34])).to_bytes(2, "big")
            pseudo = bytes(frame[26:34]) + b"\x00\x11" + udp_length.to_bytes(2, "big")
            allowed = set(range(16, 18)) | set(range(24, 26))
        else:
            frame[18:20] = udp_length.to_bytes(2, "big")
            pseudo = bytes(frame[22:54]) + udp_length.to_bytes(4, "big") + b"\x00\x00\x00\x11"
            allowed = set(range(18, 20))
        frame[udp_start + 6:udp_start + 8] = (reader.checksum(pseudo + bytes(frame[udp_start:])) or 65535).to_bytes(2, "big")
        allowed |= set(range(udp_start + 4, udp_start + 8))
        allowed |= set(range(old["payload_start"] + 2, old["payload_start"] + 4))
        changed = [index for index, (a, b) in enumerate(zip(record["frame"], frame)) if a != b]
        assert set(changed) <= allowed and len(frame) == len(record["frame"]) + 6
        new = reader.inspect(bytes(frame))
        assert new["payload"] == payload and new["source"] == old["source"] and new["destination"] == old["destination"]
        new_header = record["header"][:8] + struct.pack("<II", len(frame), len(frame))
        c_records.append(new_header + bytes(frame))
        corrections.append(dict(original_frame=number, IP_version=old["version"],
                                original_payload_length=26, new_payload_length=32,
                                original_payload_hex=old["payload"].hex(), new_payload_hex=payload.hex(),
                                NAS_IP_Address=dict(type=4, length=6, value="192.0.2.1",
                                                    PDU_range=[26, 32], value_PDU_range=[28, 32],
                                                    physical_range=[old["payload_start"] + 26, old["payload_start"] + 32]),
                                original_record_SHA256=sha(record["full"]),
                                new_record_SHA256=sha(c_records[-1]),
                                allowed_changed_old_offsets=sorted(allowed), actual_changed_old_offsets=changed,
                                original_and_new_timestamps_equal=new_header[:8] == record["header"][:8]))
    c = header + b"".join(c_records)
    assert all(c_records[n - 1] == records[n - 1]["full"] for n in (1, 2, 3, 5, 6, 7))
    a = header + c_records[3] + c_records[7]
    b_records, rrq_changes = [], []
    for number, record in enumerate(c_records, 1):
        if number in (3, 7):
            frame, change = reader.rewrite_rrq(record[16:])
            b_records.append(record[:16] + frame)
            rrq_changes.append(dict(original_frame=number, **change))
        else:
            b_records.append(record)
    b = header + b"".join(b_records)
    for blob, count in ((a, 2), (b, 8), (c, 8)):
        offset, number = 24, 0
        while offset < len(blob):
            sec, usec, caplen, wirelen = struct.unpack("<IIII", blob[offset:offset + 16])
            assert caplen == wirelen and usec == 0
            frame = blob[offset + 16:offset + 16 + caplen]
            fields = reader.inspect(frame)
            if fields["destination_port"] == 1812:
                assert len(fields["payload"]) == 32 and fields["payload"][26:] == bytes([4, 6, 192, 0, 2, 1])
                assert int.from_bytes(fields["payload"][2:4], "big") == 32
            offset += 16 + caplen
            number += 1
        assert offset == len(blob) and number == count
    args.out.mkdir(parents=True, exist_ok=False)
    outputs = {}
    for name, blob, source_numbers in (("C8-valid-original-peers.pcap", c, list(range(1, 9))),
                                       ("A2-valid-radius-only.pcap", a, [4, 8]),
                                       ("B8-valid-isolated-RRQ.pcap", b, list(range(1, 9)))):
        path = args.out / name
        path.write_bytes(blob)
        outputs[name] = dict(path=str(path), SHA256=sha(blob), frames=len(source_numbers),
                             source_frame_order=source_numbers, original_seconds_preserved=source_numbers)
    summary = dict(scope=__doc__, original_capture=str(args.input), original_capture_SHA256=sha(source),
                   source_reader=str(args.reader), source_reader_SHA256=sha(reader_bytes),
                   driver_SHA256=sha(Path(__file__).read_bytes()), outputs=outputs,
                   original_6_complete_records_unchanged_in_C8=[1, 2, 3, 5, 6, 7],
                   C8_radius_corrections=corrections, B8_RRQ_changes=rrq_changes,
                   B8_other_complete_records_equal_C8=[1, 2, 4, 5, 6, 8],
                   A2_complete_records_equal_C8=[4, 8], all_IPv4_and_UDP_checksums_verified=True,
                   expected_source_grammar_C8_B8=["STUN", "CoAP", "TFTP", "RADIUS"] * 2,
                   valid_radius_native_status_requires_separate_old_binary_run=True,
                   no_Go_native_or_future_producer=True,
                   original_missing_NAS_request_and_failure_evidence_preserved=True)
    path = args.out / "summary.json"
    path.write_text(json.dumps(summary, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(path), SHA256=sha(path.read_bytes()), outputs=outputs)))

if __name__ == "__main__":
    main()
