#!/usr/bin/env python3
"""Build a deterministic Ethernet/IPv4/TCP DCE/RPC v5 exchange PCAP.

The packet and RPC wire bytes are constructed here with only Python's standard
library; this does not replay or transform an existing capture.
"""

from __future__ import annotations

import hashlib
import ipaddress
import struct
from pathlib import Path


ROOT = Path(__file__).resolve().parents[3]
OUTPUT = ROOT / "common/pcapx/pcaputil/testdata/protocol-sessions/dcerpc-v5-bindack-request-response.pcap"

CLIENT_IP = ipaddress.IPv4Address("192.0.2.10").packed
SERVER_IP = ipaddress.IPv4Address("192.0.2.20").packed
CLIENT_MAC = bytes.fromhex("020000000010")
SERVER_MAC = bytes.fromhex("020000000020")
CLIENT_PORT = 49152
SERVER_PORT = 135
CLIENT_ISN = 100000
SERVER_ISN = 500000

EPM_UUID = bytes.fromhex("0883afe11f5dc91191a408002b14a0fa")
NDR32_UUID = bytes.fromhex("045d888aeb1cc9119fe808002b104860")


def internet_checksum(data: bytes) -> int:
    if len(data) & 1:
        data += b"\x00"
    words = struct.unpack(f"!{len(data) // 2}H", data)
    total = sum(words)
    while total >> 16:
        total = (total & 0xFFFF) + (total >> 16)
    return (~total) & 0xFFFF


def pdu(ptype: int, call_id: int, body: bytes, flags: int = 0x03) -> bytes:
    header = bytearray(16)
    header[0:4] = bytes((5, 0, ptype, flags))
    header[4:8] = bytes((0x10, 0, 0, 0))
    struct.pack_into("<HHI", header, 8, 16 + len(body), 0, call_id)
    return bytes(header) + body


def bind() -> bytes:
    body = bytearray(56)
    struct.pack_into("<HHI", body, 0, 5840, 5840, 0)
    body[8] = 1
    struct.pack_into("<HBB", body, 12, 0, 1, 0)
    body[16:32] = EPM_UUID
    struct.pack_into("<I", body, 32, 3)
    body[36:52] = NDR32_UUID
    struct.pack_into("<I", body, 52, 2)
    return pdu(11, 1, body)


def bind_ack() -> bytes:
    body = bytearray(40)
    struct.pack_into("<HHI", body, 0, 5840, 5840, 1)
    struct.pack_into("<H", body, 8, 0)  # Empty secondary address.
    body[12] = 1  # One presentation-context result.
    struct.pack_into("<HH", body, 16, 0, 0)  # Acceptance, reason 0.
    body[20:36] = NDR32_UUID
    struct.pack_into("<I", body, 36, 2)
    return pdu(12, 1, body)


def request() -> bytes:
    body = struct.pack("<IHH", 4, 0, 99) + bytes.fromhex("deadbeef")
    return pdu(0, 2, body)


def response() -> bytes:
    body = struct.pack("<I HBB", 4, 0, 0, 0) + bytes.fromhex("cafebabe")
    return pdu(2, 2, body)


def tcp_packet(src: int, dst: int, seq: int, ack: int, flags: int, payload: bytes, ident: int) -> bytes:
    src_ip, dst_ip = (CLIENT_IP, SERVER_IP) if src == CLIENT_PORT else (SERVER_IP, CLIENT_IP)
    src_mac, dst_mac = (CLIENT_MAC, SERVER_MAC) if src == CLIENT_PORT else (SERVER_MAC, CLIENT_MAC)
    tcp = bytearray(struct.pack("!HHIIHHHH", src, dst, seq, ack, (5 << 12) | flags, 64240, 0, 0))
    tcp_bytes = bytes(tcp) + payload
    pseudo = src_ip + dst_ip + bytes((0, 6)) + struct.pack("!H", len(tcp_bytes))
    struct.pack_into("!H", tcp, 16, internet_checksum(pseudo + tcp_bytes))
    tcp_bytes = bytes(tcp) + payload

    total_length = 20 + len(tcp_bytes)
    ip = bytearray(struct.pack("!BBHHHBBH4s4s", 0x45, 0, total_length, ident, 0x4000, 64, 6, 0, src_ip, dst_ip))
    struct.pack_into("!H", ip, 10, internet_checksum(bytes(ip)))
    ethernet = dst_mac + src_mac + b"\x08\x00"
    return ethernet + bytes(ip) + tcp_bytes


def packets() -> list[bytes]:
    bind_pdu = bind()
    ack_pdu = bind_ack()
    request_pdu = request()
    response_pdu = response()
    cseq = CLIENT_ISN
    sseq = SERVER_ISN
    cnext = cseq + 1
    snext = sseq + 1
    result = [
        tcp_packet(CLIENT_PORT, SERVER_PORT, cseq, 0, 0x02, b"", 1),
        tcp_packet(SERVER_PORT, CLIENT_PORT, sseq, cnext, 0x12, b"", 2),
        tcp_packet(CLIENT_PORT, SERVER_PORT, cnext, snext, 0x10, b"", 3),
    ]
    result.append(tcp_packet(CLIENT_PORT, SERVER_PORT, cnext, snext, 0x18, bind_pdu, 4))
    cnext += len(bind_pdu)
    result.append(tcp_packet(SERVER_PORT, CLIENT_PORT, snext, cnext, 0x18, ack_pdu, 5))
    snext += len(ack_pdu)
    result.append(tcp_packet(CLIENT_PORT, SERVER_PORT, cnext, snext, 0x18, request_pdu, 6))
    cnext += len(request_pdu)
    result.append(tcp_packet(SERVER_PORT, CLIENT_PORT, snext, cnext, 0x18, response_pdu, 7))
    return result


def main() -> None:
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    frames = packets()
    with OUTPUT.open("wb") as out:
        out.write(struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 65535, 1))
        for index, frame in enumerate(frames):
            out.write(struct.pack("<IIII", 1_700_000_000, index * 100_000, len(frame), len(frame)))
            out.write(frame)
    digest = hashlib.sha256(OUTPUT.read_bytes()).hexdigest()
    print(f"{OUTPUT.relative_to(ROOT)}\t{len(frames)} packets\tsha256={digest}")


if __name__ == "__main__":
    main()
