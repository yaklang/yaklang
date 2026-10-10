"""Build the offline PCAPDB fixture from the repository's pinned traffic corpus.

Run explicitly with Python 3. All input archives remain unchanged; packet bytes,
lengths and timestamp precision are retained in independently scoped sections.
"""

import gzip
import hashlib
import json
from pathlib import Path
import struct
import zipfile

REPOSITORY = Path(__file__).resolve().parents[4]
BATCH_ROOT = REPOSITORY / "common/bin-parser/testdata/corpus-batches"
SELECTED = ['ndpi-dns', 'ndpi-http', 'ndpi-ssh', 'ndpi-mqtt', 'ndpi-mysql', 'ndpi-modbus', 'ndpi-s7comm', 'ndpi-enip-cip', 'ndpi-opcua', 'ndpi-smtp', 'ndpi-ftp', 'ndpi-snmp', 'ndpi-dcerpc', 'gen-dhcpv6', 'gen-icmpv6', 'gen-llmnr-mdns', 'gen-ieee8021q', 'gen-geneve', 'gen-jsonrpc', 'ndpi-rtp', 'ndpi-coap', 'ndpi-sip', 'ndpi-zabbix-agent', 'ndpi-syslog']


def sha256(data):
    return hashlib.sha256(data).hexdigest()


def block(kind, body):
    body += b"\x00" * (-len(body) % 4)
    size = len(body) + 12
    return struct.pack("<II", kind, size) + body + struct.pack("<I", size)


def option(kind, body):
    return struct.pack("<HH", kind, len(body)) + body + b"\x00" * (-len(body) % 4)


def classic_to_ng(data, name):
    formats = {
        b"\xd4\xc3\xb2\xa1": ("<", 6),
        b"\xa1\xb2\xc3\xd4": (">", 6),
        b"\x4d\x3c\xb2\xa1": ("<", 9),
        b"\xa1\xb2\x3c\x4d": (">", 9),
    }
    endian, resolution = formats[data[:4]]
    major, minor, _, _, snaplen, link = struct.unpack(endian + "HHiIII", data[4:24])
    assert (major, minor) == (2, 4) and link < 65536
    output = bytearray(block(0x0A0D0D0A, struct.pack("<IHHq", 0x1A2B3C4D, 1, 0, -1)))
    options = option(2, name.encode()) + option(9, bytes([resolution])) + option(0, b"")
    output += block(1, struct.pack("<HHI", link, 0, snaplen) + options)
    offset = 24
    while offset < len(data):
        seconds, fraction, captured, wire = struct.unpack_from(endian + "IIII", data, offset)
        packet = data[offset + 16:offset + 16 + captured]
        assert len(packet) == captured and captured <= wire
        timestamp = seconds * (10 ** resolution) + fraction
        output += block(6, struct.pack("<IIIII", 0, timestamp >> 32, timestamp & 0xFFFFFFFF, captured, wire) + packet)
        offset += 16 + captured
    assert offset == len(data)
    return bytes(output)


def inspect_ng(data):
    offset, sections, interfaces, packets, packet_bytes = 0, 0, 0, 0, 0
    endian = None
    links = set()
    payload_hash = hashlib.sha256()
    while offset < len(data):
        if data[offset:offset + 4] == b"\x0a\x0d\x0d\x0a":
            endian = "<" if data[offset + 8:offset + 12] == b"\x4d\x3c\x2b\x1a" else ">"
            sections += 1
        kind, size = struct.unpack_from(endian + "II", data, offset)
        assert size >= 12 and size % 4 == 0 and offset + size <= len(data)
        assert struct.unpack_from(endian + "I", data, offset + size - 4)[0] == size
        if kind == 1:
            interfaces += 1
            links.add(struct.unpack_from(endian + "H", data, offset + 8)[0])
        if kind == 6:
            captured, wire = struct.unpack_from(endian + "II", data, offset + 20)
            packet = data[offset + 28:offset + 28 + captured]
            assert captured <= wire and len(packet) == captured
            payload_hash.update(struct.pack("<II", captured, wire))
            payload_hash.update(packet)
            packets += 1
            packet_bytes += captured
        elif kind in (2, 3):
            raise ValueError("Fixture inputs must use enhanced packet blocks")
        offset += size
    return dict(sections=sections, interfaces=interfaces, packets=packets, link_types=sorted(links),
                packet_payload_sha256=payload_hash.hexdigest(), packet_bytes=packet_bytes)


def main():
    index = json.loads((BATCH_ROOT / "index.json").read_text())
    batch = next(b for b in index["batches"] if b["file"] == "baseline-2fb090d177c3.zip")
    archive = BATCH_ROOT / batch["file"]
    assert sha256(archive.read_bytes()) == batch["sha256"]
    output = bytearray()
    sources = []
    with zipfile.ZipFile(archive) as source_zip:
        password = batch["password"].encode()
        provenance = json.loads(source_zip.read("PROVENANCE.json", pwd=password))
        cases = {c["original_path"]: c for c in provenance["cases"]}
        available = [f for f in source_zip.infolist() if f.filename.startswith("captures/")
                     and f.filename.endswith((".pcap", ".pcapng", ".cap"))]
        for name in SELECTED:
            choices = [f for f in available if Path(f.filename).stem == name]
            assert len(choices) == 1, name
            member_name = choices[0].filename
            raw = source_zip.read(member_name, pwd=password)
            member = batch["members"][member_name]
            assert len(raw) == member["bytes"] and sha256(raw) == member["sha256"]
            original = member_name.removeprefix("captures/")
            converted = raw if raw[:4] == b"\x0a\x0d\x0d\x0a" else classic_to_ng(raw, name)
            stats = inspect_ng(converted)
            case = cases[original]
            records = [m["record"] for m in case["metadata"] if "protocol" in m.get("record", {})]
            sources.append(dict(source=original, member=member_name, sha256=member["sha256"],
                                bytes=len(raw), first_packet_id=sum(s["packets"] for s in sources) + 1,
                                **stats, protocols=sorted({r["protocol"] for r in records}),
                                evidence_kinds=sorted({r.get("evidence_kind", "") for r in records}),
                                license_evidence=case.get("license_evidence", [])))
            output += converted
    compressed = gzip.compress(bytes(output), compresslevel=9, mtime=0)
    manifest = dict(
        format="pcapng", source_batch=batch["file"], source_batch_sha256=batch["sha256"],
        source_commit=provenance["source_sha"],
        conversion="Each complete source is a separate PCAPNG section. Classic PCAP records become enhanced packet blocks with original link type, timestamp precision, captured bytes and wire length. Existing PCAPNG sections are copied byte for byte; packet order is unchanged. No padding or repeated packets added to reach the compressed-size target.",
        compressed_bytes=len(compressed), expanded_bytes=len(output),
        compressed_sha256=sha256(compressed), expanded_sha256=sha256(output),
        **inspect_ng(output), sources=sources,
    )
    assert 190 << 10 <= len(compressed) <= 215 << 10
    folder = Path(__file__).resolve().parent
    (folder / "comprehensive.pcapng.gz").write_bytes(compressed)
    (folder / "comprehensive.json").write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n")
    print(json.dumps({k: v for k, v in manifest.items() if k not in ("sources", "conversion")}, indent=2))
    print("sources", len(sources))


if __name__ == "__main__":
    main()
