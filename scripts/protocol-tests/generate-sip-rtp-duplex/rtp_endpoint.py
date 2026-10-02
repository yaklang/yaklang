#!/usr/bin/env python3
"""Send or echo a fixed number of test-only PCMU RTP datagrams over UDP."""

import argparse
import socket
import struct
import time


def packet(sequence: int, timestamp: int, ssrc: int) -> bytes:
    return struct.pack("!BBHII", 0x80, 0, sequence & 0xFFFF,
                       timestamp & 0xFFFFFFFF, ssrc) + (b"\xff" * 160)


def run_echo(local_port: int, peer_port: int, count: int) -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("127.0.0.1", local_port))
    sock.settimeout(0.1)
    received = 0
    deadline = time.monotonic() + 8.0
    while received < count and time.monotonic() < deadline:
        try:
            data, address = sock.recvfrom(2048)
        except TimeoutError:
            continue
        if address[1] != peer_port or len(data) != 172 or data[0] != 0x80 or data[1] != 0:
            continue
        sequence = 5000 + received
        timestamp = 800000 + received * 160
        sock.sendto(packet(sequence, timestamp, 0xCAFE0222), address)
        received += 1
    sock.close()
    print(f"mode=echo local_port={local_port} tx={received} rx={received}")
    return 0 if received == count else 1


def run_send(local_port: int, peer_port: int, count: int, interval: float) -> int:
    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind(("127.0.0.1", local_port))
    sock.settimeout(0.5)
    received = 0
    for index in range(count):
        sock.sendto(packet(5000 + index, 800000 + index * 160, 0xCAFE0111),
                    ("127.0.0.1", peer_port))
        try:
            data, address = sock.recvfrom(2048)
        except TimeoutError:
            print(f"mode=send local_port={local_port} tx={index + 1} rx={received}")
            sock.close()
            return 1
        if address[1] != peer_port or len(data) != 172 or data[0] != 0x80 or data[1] != 0:
            print(f"mode=send local_port={local_port} tx={index + 1} rx={received}")
            sock.close()
            return 1
        received += 1
        time.sleep(interval)
    sock.close()
    print(f"mode=send local_port={local_port} tx={count} rx={received}")
    return 0 if received == count else 1


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("mode", choices=("send", "echo"))
    parser.add_argument("local_port", type=int)
    parser.add_argument("peer_port", type=int)
    parser.add_argument("count", type=int, default=63)
    parser.add_argument("--interval", type=float, default=0.02)
    args = parser.parse_args()
    if not (0 < args.local_port < 65536 and 0 < args.peer_port < 65536):
        parser.error("ports must be in 1..65535")
    if not (1 <= args.count <= 512) or not (0 <= args.interval <= 1):
        parser.error("count must be 1..512 and interval must be 0..1 seconds")
    if args.mode == "echo":
        return run_echo(args.local_port, args.peer_port, args.count)
    return run_send(args.local_port, args.peer_port, args.count, args.interval)


if __name__ == "__main__":
    raise SystemExit(main())
