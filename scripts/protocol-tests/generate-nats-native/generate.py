#!/usr/bin/env python3
"""Generate native NATS fixtures with pinned official binaries/APIs and passive TCP capture.

Requires Python 3, Go 1.22+, dumpcap (capture permission), tshark, and Internet
access. The Go client is built in a temporary module; the repository go.mod is
never used or changed. Output must be a new directory. Linux and macOS are
supported; the committed capture was generated on macOS arm64/lo0.
"""
import argparse
import csv
import hashlib
import io
import json
import os
from pathlib import Path
import platform
import shutil
import signal
import subprocess
import tarfile
import tempfile
import time
import urllib.request

HERE = Path(__file__).resolve().parent
SERVER = "2.10.26"
CLIENT = "1.39.1"
ADR_COMMIT = "685743395fb8e2c912f1cbf2405db1ed5802c7ac"
FIELDS = ["frame.number", "tcp.stream", "ip.src", "tcp.srcport", "ip.dst", "tcp.dstport", "tcp.seq", "tcp.len", "tcp.payload"]


def sha(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def tool(name):
    found = shutil.which(name)
    if not found:
        candidate = Path("/Applications/Wireshark.app/Contents/MacOS") / name
        if candidate.exists():
            found = str(candidate)
    if not found:
        raise RuntimeError(f"{name} is required")
    return found


def fetch(url, path):
    urllib.request.urlretrieve(url, path)


def main():
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--output", type=Path, required=True)
    ap.add_argument("--interface", default="lo0" if platform.system() == "Darwin" else "lo")
    ap.add_argument("--port", type=int, default=44222)
    ap.add_argument("--monitor-port", type=int, default=44223)
    args = ap.parse_args()
    out = args.output.resolve()
    out.mkdir(parents=True, exist_ok=False)
    dumpcap, tshark = tool("dumpcap"), tool("tshark")
    osname = platform.system().lower()
    arch = {"aarch64": "arm64", "arm64": "arm64", "x86_64": "amd64"}[platform.machine()]
    archive = f"nats-server-v{SERVER}-{osname}-{arch}.tar.gz"
    release = f"https://github.com/nats-io/nats-server/releases/download/v{SERVER}/{archive}"
    checksums = {line.split()[1]: line.split()[0] for line in (HERE / "server-SHA256SUMS").read_text().splitlines()}
    with tempfile.TemporaryDirectory(prefix="nats-native-") as work:
        work = Path(work)
        packed = work / archive
        fetch(release, packed)
        assert sha(packed) == checksums[archive], "release archive SHA256 mismatch"
        with tarfile.open(packed) as tar:
            # Only extract the pinned executable, never arbitrary archive paths.
            member = tar.getmember(f"nats-server-v{SERVER}-{osname}-{arch}/nats-server")
            server = work / "nats-server"
            server.write_bytes(tar.extractfile(member).read())
            server.chmod(0o755)
        module = work / "client"
        module.mkdir()
        for source, target in [("client.go.txt", "main.go"), ("client.mod", "go.mod"), ("client.sum", "go.sum")]:
            shutil.copyfile(HERE / source, module / target)
        with (out / "client-build.log").open("w") as log:
            subprocess.run(["go", "build", "-mod=readonly", "-o", str(work / "native-client"), "."], cwd=module, stdout=log, stderr=subprocess.STDOUT, check=True)
        (out / "client-build-info.txt").write_text(subprocess.check_output(["go", "version", "-m", str(work / "native-client")], text=True))
        version = subprocess.check_output([str(server), "--version"], text=True).strip()
        cmd = [str(server), "-a", "127.0.0.1", "-p", str(args.port), "-m", str(args.monitor_port), "-n", "nats-native-fixture", "-js", "-sd", str(work / "jetstream"), "-DV"]
        cap = None
        with (out / "server.log").open("w") as serverlog, (out / "dumpcap.log").open("w") as caplog:
            srv = subprocess.Popen(cmd, stdout=serverlog, stderr=subprocess.STDOUT)
            try:
                for _ in range(60):
                    if srv.poll() is not None:
                        raise RuntimeError("server exited before becoming ready")
                    try:
                        with urllib.request.urlopen(f"http://127.0.0.1:{args.monitor_port}/healthz", timeout=.2) as r:
                            assert r.status == 200
                        break
                    except OSError:
                        time.sleep(.1)
                else:
                    raise RuntimeError("server readiness timeout")
                cap = subprocess.Popen([dumpcap, "-i", args.interface, "-f", f"tcp port {args.port}", "-w", str(out / "native-headers.pcapng")], stdout=caplog, stderr=subprocess.STDOUT)
                time.sleep(.5)
                if cap.poll() is not None:
                    raise RuntimeError("dumpcap failed; check capture permissions")
                subprocess.run([str(work / "native-client"), f"nats://127.0.0.1:{args.port}", str(out)], check=True)
                with urllib.request.urlopen(f"http://127.0.0.1:{args.monitor_port}/varz") as r:
                    (out / "server-varz.json").write_bytes(r.read())
                time.sleep(.3)
            finally:
                if cap and cap.poll() is None:
                    cap.send_signal(signal.SIGINT)
                    cap.wait(timeout=5)
                srv.terminate()
                srv.wait(timeout=5)
        sources = []
        for name, repo, tag in [("server", "nats-server", f"v{SERVER}"), ("client", "nats.go", f"v{CLIENT}")]:
            url = f"https://raw.githubusercontent.com/nats-io/{repo}/{tag}/LICENSE"
            fetch(url, out / f"{name}-LICENSE.txt")
            sources.append({"name": name, "version": tag, "source": f"https://github.com/nats-io/{repo}/tree/{tag}", "license": "Apache-2.0", "license_source": url})
        fields_cmd = [tshark, "-r", str(out / "native-headers.pcapng"), "-Y", "tcp.len > 0", "-T", "fields", "-E", "header=y", "-E", "separator=,", "-E", "quote=d"]
        for field in FIELDS:
            fields_cmd.extend(["-e", field])
        csv_text = subprocess.check_output(fields_cmd, text=True)
        (out / "tshark-tcp.csv").write_text(csv_text)
        streams = {}
        for row in csv.DictReader(io.StringIO(csv_text)):
            direction = "server" if int(row["tcp.srcport"]) == args.port else "client"
            key = (int(row["tcp.stream"]), direction)
            data = bytes.fromhex(row["tcp.payload"])
            assert len(data) == int(row["tcp.len"])
            raw = streams.setdefault(key, bytearray())
            start = int(row["tcp.seq"]) - 1
            assert start <= len(raw), "TCP capture has a gap"
            overlap = min(len(raw) - start, len(data))
            assert raw[start:start + overlap] == data[:overlap], "conflicting retransmission"
            raw.extend(data[overlap:])
        tcp_oracle = []
        assert len(streams) == 4, "expected two complete bidirectional official client connections"
        for (stream, direction), raw in sorted(streams.items()):
            file = f"connection-{stream + 1}-{direction}.bin"
            assert raw == (out / file).read_bytes(), "tshark reassembly differs from socket bytes"
            tcp_oracle.append({"stream": stream, "direction": direction, "file": file, "bytes": len(raw), "sha256": sha(out / file)})
        protocols = subprocess.check_output([tshark, "-G", "protocols"], text=True)
        dissectors = [line for line in protocols.splitlines() if "nats" in line.lower()]
        manifest = {
            "kind": "native", "capture_method": "passive dumpcap on loopback, unmodified packets; official nats.go API against official nats-server release",
            "capture_license": "Generated locally for this repository; no third-party packet capture copied",
            "sources": sources,
            "header_spec": f"https://github.com/nats-io/nats-architecture-and-design/blob/{ADR_COMMIT}/adr/ADR-4.md",
            "server_release": {"url": release, "archive_sha256": sha(packed), "binary_sha256": sha(server), "version_output": version},
            "environment": {"os": platform.platform(), "machine": platform.machine(), "interface": args.interface, "go": subprocess.check_output(["go", "version"], text=True).strip()},
            "tshark": {"version": subprocess.check_output([tshark, "--version"], text=True).splitlines()[0], "protocol_dissectors_matching_nats": dissectors, "available_fields_used": FIELDS, "limitation": "TCP byte oracle only; no NATS dissector in the capture environment"},
            "tcp_oracle": tcp_oracle,
            "native_variants": ["multi-value and case-distinct keys", "ASCII NUL/SOH/DEL values", "empty header section", "reply subject and zero body", "server-generated 503", "server-generated 404 with description", "server-generated 100 heartbeat with fields"],
            "synthetic": "Malformed, future-version, folding and budget fixtures are created explicitly in protocol_session_nats_headers_test.go; none are represented as native captures",
            "generator_files": {f.name: sha(f) for f in sorted(HERE.iterdir()) if f.is_file()},
            "files": {f.name: sha(f) for f in sorted(out.iterdir()) if f.is_file()},
        }
        (out / "manifest.json").write_text(json.dumps(manifest, indent=2) + "\n")
        print(f"Verified {len(streams)} socket directions against tshark TCP bytes; wrote {out}")


if __name__ == "__main__":
    main()
