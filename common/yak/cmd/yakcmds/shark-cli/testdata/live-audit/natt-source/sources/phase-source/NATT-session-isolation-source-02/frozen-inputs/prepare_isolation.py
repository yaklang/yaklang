"""Derive two immutable isolation PCAPs from the original eight source frames.

A retains only the IPv4/IPv6 RADIUS records, with their original timestamps.
B changes only the two RRQ destination IPs and the dependent checksums. The
other six complete PCAP records remain byte-for-byte equal to the source.
This producer uses no Go, native parser, Wireshark output or future oracle.
"""
from pathlib import Path
import argparse, hashlib, ipaddress, json, struct

SOURCE_SHA = "78f3537571bc44bb1c1a31f694973ea9d17ca4a1cd53e1b734f6f7e93f944351"
WIRE = {
    "STUN": bytes.fromhex("000100002112a4420102030405060708090a0b0c"),
    "CoAP": bytes.fromhex("40010001b773656e736f72730474656d70"),
    "TFTP": b"\x00\x01example.txt\x00octet\x00",
    "RADIUS": bytes([1, 7, 0, 26]) + bytes(range(1, 17)) + b"\x01\x06test",
}

def sha(data):
    return hashlib.sha256(data).hexdigest()

def checksum(data):
    if len(data) & 1:
        data += b"\x00"
    value = sum(struct.unpack("!%dH" % (len(data) // 2), data))
    while value >> 16:
        value = (value & 65535) + (value >> 16)
    return (~value) & 65535

def inspect(frame):
    ether = int.from_bytes(frame[12:14], "big")
    if ether == 0x0800:
        version, ip_start, udp_start = 4, 14, 34
        assert frame[14] == 0x45 and frame[23] == 17
        assert int.from_bytes(frame[16:18], "big") == len(frame) - 14
        assert checksum(frame[14:34]) == 0
        src, dst = frame[26:30], frame[30:34]
        dst_range, ip_checksum_range = [30, 34], [24, 26]
        pseudo = src + dst + b"\x00\x11" + (len(frame) - 34).to_bytes(2, "big")
    else:
        assert ether == 0x86dd and frame[14] >> 4 == 6 and frame[20] == 17
        version, ip_start, udp_start = 6, 14, 54
        assert int.from_bytes(frame[18:20], "big") == len(frame) - 54
        src, dst = frame[22:38], frame[38:54]
        dst_range, ip_checksum_range = [38, 54], None
        pseudo = src + dst + (len(frame) - 54).to_bytes(4, "big") + b"\x00\x00\x00\x11"
    udp = frame[udp_start:]
    assert int.from_bytes(udp[4:6], "big") == len(udp)
    assert int.from_bytes(udp[6:8], "big") != 0 and checksum(pseudo + udp) == 0
    assert int.from_bytes(udp[:2], "big") == 4500
    return dict(version=version, source=str(ipaddress.ip_address(src)),
                destination=str(ipaddress.ip_address(dst)), source_port=4500,
                destination_port=int.from_bytes(udp[2:4], "big"),
                IP_start=ip_start, UDP_start=udp_start, payload_start=udp_start + 8,
                destination_range=dst_range, IP_checksum_range=ip_checksum_range,
                UDP_checksum_range=[udp_start + 6, udp_start + 8], payload=udp[8:])

def read_pcap(data):
    assert data[:4] == bytes.fromhex("d4c3b2a1")
    assert struct.unpack("<IHHIIII", data[:24]) == (0xa1b2c3d4, 2, 4, 0, 0, 65535, 1)
    records, offset = [], 24
    while offset < len(data):
        sec, usec, caplen, wirelen = struct.unpack("<IIII", data[offset:offset + 16])
        assert caplen == wirelen
        full = data[offset:offset + 16 + caplen]
        assert len(full) == caplen + 16
        records.append(dict(sec=sec, usec=usec, header=full[:16], frame=full[16:], full=full))
        offset += 16 + caplen
    assert offset == len(data) and len(records) == 8
    return data[:24], records

def rewrite_rrq(frame):
    old = inspect(frame)
    assert old["payload"] == WIRE["TFTP"] and old["destination_port"] == 69
    dst = ipaddress.ip_address("192.0.2.3" if old["version"] == 4 else "2001:db8::3").packed
    value = bytearray(frame)
    start, end = old["destination_range"]
    value[start:end] = dst
    udp_start = old["UDP_start"]
    value[udp_start + 6:udp_start + 8] = b"\x00\x00"
    udp = bytes(value[udp_start:])
    if old["version"] == 4:
        value[24:26] = b"\x00\x00"
        value[24:26] = checksum(bytes(value[14:34])).to_bytes(2, "big")
        pseudo = bytes(value[26:34]) + b"\x00\x11" + len(udp).to_bytes(2, "big")
    else:
        pseudo = bytes(value[22:54]) + len(udp).to_bytes(4, "big") + b"\x00\x00\x00\x11"
    value[udp_start + 6:udp_start + 8] = (checksum(pseudo + udp) or 65535).to_bytes(2, "big")
    result = bytes(value)
    new = inspect(result)
    assert new["payload"] == old["payload"]
    allowed = set(range(start, end)) | set(range(udp_start + 6, udp_start + 8))
    if old["IP_checksum_range"]:
        allowed |= set(range(*old["IP_checksum_range"]))
    changed = [index for index, (a, b) in enumerate(zip(frame, result)) if a != b]
    assert set(changed) <= allowed and len(result) == len(frame)
    return result, dict(old_destination=old["destination"], new_destination=new["destination"],
                        allowed_changed_offsets=sorted(allowed), actual_changed_offsets=changed,
                        payload_SHA256=sha(old["payload"]), complete_before_SHA256=sha(frame),
                        complete_after_SHA256=sha(result))

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--input", type=Path, required=True)
    parser.add_argument("--out", type=Path, required=True)
    args = parser.parse_args()
    source = args.input.read_bytes()
    assert sha(source) == SOURCE_SHA
    header, records = read_pcap(source)
    details = []
    names = ["STUN", "CoAP", "TFTP", "RADIUS"] * 2
    ports = [3478, 5683, 69, 1812] * 2
    for number, (record, name, port) in enumerate(zip(records, names, ports), 1):
        item = inspect(record["frame"])
        assert record["sec"] == number and record["usec"] == 0
        assert item["payload"] == WIRE[name] and item["destination_port"] == port
        assert item["version"] == (4 if number <= 4 else 6)
        details.append(dict(source_frame=number, source_grammar=name, seconds=record["sec"],
                            source=str(ipaddress.ip_address("192.0.2.1" if number <= 4 else "2001:db8::1")),
                            original_destination=item["destination"], source_port=4500,
                            destination_port=port, payload_hex=item["payload"].hex(),
                            payload_range=[item["payload_start"], len(record["frame"])],
                            original_record_SHA256=sha(record["full"])))
    a = header + records[3]["full"] + records[7]["full"]
    b_records, changes = [], []
    for number, record in enumerate(records, 1):
        if number in (3, 7):
            frame, change = rewrite_rrq(record["frame"])
            b_records.append(record["header"] + frame)
            changes.append(dict(source_frame=number, **change))
        else:
            b_records.append(record["full"])
    b = header + b"".join(b_records)
    assert len(b) == len(source)
    args.out.mkdir(parents=True, exist_ok=False)
    paths = {"A_radius_only": args.out / "A-radius-only.pcap", "B_isolated_RRQ": args.out / "B-isolated-RRQ.pcap"}
    for key, value in [("A_radius_only", a), ("B_isolated_RRQ", b)]:
        paths[key].write_bytes(value)
    result = dict(scope=__doc__, source_capture=str(args.input), source_SHA256=sha(source),
                  driver_SHA256=sha(Path(__file__).read_bytes()), no_Go_or_native_execution=True,
                  no_future_producer_input=True,
                  A=dict(path=str(paths["A_radius_only"]), SHA256=sha(a), frames=2,
                         original_source_frames=[4, 8], complete_records_literal=True,
                         timestamps_preserved=[4, 8], source_wire_grammar=["RADIUS", "RADIUS"]),
                  B=dict(path=str(paths["B_isolated_RRQ"]), SHA256=sha(b), frames=8,
                         unchanged_complete_records=[1, 2, 4, 5, 6, 8], RRQ_changes=changes,
                         source_wire_grammar=names, IPv4_header_checksums_verified=True,
                         all_UDP_checksums_verified=True),
                  original_source_facts=details,
                  expected_native_labels_not_yet_runtime_proven=["stun", "coap", "tftp", "radius"] * 2,
                  plan_freeze_requires_closed_old_A_and_B_producers=True,
                  known_old_original8_TFTP_Radius_collision_is_not_an_oracle=True)
    (args.out / "summary.json").write_text(json.dumps(result, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(dict(summary=str(args.out / "summary.json"), SHA256=sha((args.out / "summary.json").read_bytes()),
                          A_SHA256=sha(a), B_SHA256=sha(b))))

if __name__ == "__main__":
    main()
