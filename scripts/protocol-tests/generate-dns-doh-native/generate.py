"""Capture pinned CoreDNS and dnspython traffic on a local loopback interface.

Requires Docker, dumpcap/tshark, OpenSSL and the pinned Python packages below.
No Yaklang parser is used to generate the captures or independent oracles.
"""
import argparse
import hashlib
import importlib.metadata
import json
import os
from pathlib import Path
import shutil
import signal
import subprocess
import time

IMAGE = "coredns/coredns@sha256:40384aa1f5ea6bfdc77997d243aec73da05f27aed0c5e9d65bfa98933c519d97"


def run(args, **kwargs):
    return subprocess.run(args, check=True, **kwargs)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("out", type=Path)
    args = parser.parse_args()
    for package, expected in {"dnspython": "2.7.0", "httpx": "0.28.1"}.items():
        actual = importlib.metadata.version(package)
        if actual != expected:
            parser.error(f"{package} must be {expected}, got {actual}")
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=True)
    if any(out.iterdir()):
        parser.error("capture output must be empty to avoid mixing independent sessions")
    source = Path(__file__).resolve().parent
    for name in ["Corefile", "db.native.test"]:
        shutil.copyfile(source / name, out / name)
    run(["openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "3650",
         "-keyout", str(out / "server-key.pem"), "-out", str(out / "server.pem"),
         "-subj", "/CN=localhost native protocol fixture",
         "-addext", "subjectAltName=IP:127.0.0.1,DNS:localhost"], capture_output=True)
    name = f"pr5013-dns-{os.getpid()}"
    container = None
    dumpcap = shutil.which("dumpcap") or "/Applications/Wireshark.app/Contents/MacOS/dumpcap"
    tshark = shutil.which("tshark") or "/Applications/Wireshark.app/Contents/MacOS/tshark"
    try:
        container = run(["docker", "run", "-d", "--name", name,
                         "-p", "127.0.0.1:19553:19553/udp", "-p", "127.0.0.1:19553:19553/tcp",
                         "-p", "127.0.0.1:19544:19544/tcp", "-v", f"{out}:/fixtures:ro",
                         IMAGE, "-conf", "/fixtures/Corefile"], capture_output=True, text=True).stdout.strip()
        time.sleep(2)
        version = run(["docker", "exec", name, "/coredns", "-version"], capture_output=True, text=True).stdout
        for mode in ["dns", "h1", "h2"]:
            with (out / f"{mode}-dumpcap.log").open("w") as log:
                capture = subprocess.Popen([dumpcap, "-i", "lo0", "-f",
                                            f"port {19553 if mode == 'dns' else 19544}",
                                            "-w", str(out / f"{mode}-native.pcapng")], stdout=log, stderr=log)
                try:
                    time.sleep(1)
                    run([os.sys.executable, str(source / "client.py"), mode, str(out)])
                    time.sleep(1)
                finally:
                    capture.send_signal(signal.SIGINT)
                    capture.wait(timeout=10)
            fields = ["frame.number", "tcp.stream", "http2.streamid", "dns.id", "dns.flags.response",
                      "dns.flags.rcode", "dns.qry.name", "dns.qry.type", "dns.qry.class", "dns.a",
                      "dns.aaaa", "dns.cname", "dns.txt", "http2.headers.method", "http2.headers.status",
                      "http.request.method", "http.response.code"]
            command = [tshark, "-n", "-r", str(out / f"{mode}-native.pcapng"), "-d", "udp.port==19553,dns",
                       "-d", "tcp.port==19553,dns", "-d", "tcp.port==19544,tls"]
            if mode != "dns":
                command += ["-o", f"tls.keylog_file:{out / (mode + '.keys')}"]
            command += ["-T", "fields"]
            for field in fields:
                command += ["-e", field]
            with (out / f"{mode}-tshark.tsv").open("w") as log:
                run(command, stdout=log)
        (out / "server-oracle.log").write_text(run(["docker", "logs", name], capture_output=True, text=True).stdout)
        (out / "versions.json").write_text(json.dumps({
            "server": version.strip(), "docker_image": IMAGE, "client": "dnspython 2.7.0 / httpx 0.28.1",
            "python": os.sys.version, "tshark": run([tshark, "-v"], capture_output=True, text=True).stdout.splitlines()[0],
            "ssl": run([os.sys.executable, "-c", "import ssl;print(ssl.OPENSSL_VERSION)"], capture_output=True, text=True).stdout.strip(),
            "license": {"coredns": "Apache-2.0", "dnspython": "ISC", "httpx": "BSD-3-Clause"},
            "sources": ["https://github.com/coredns/coredns/tree/v1.12.0", "https://github.com/rthalley/dnspython/tree/v2.7.0"],
            "representation": "native_capture", "capture_interface": "lo0", "keylog_origin": "locally generated client TLS test sessions",
        }, indent=2) + "\n")
        # Server private key is not needed for replay; client key logs are the
        # explicit decryption source, and the public certificate is provenance.
        (out / "server-key.pem").unlink()
        entries = []
        for path in sorted(out.iterdir()):
            if path.is_file() and path.name != "manifest.json":
                raw = path.read_bytes()
                entries.append({"file": path.name, "bytes": len(raw), "sha256": hashlib.sha256(raw).hexdigest()})
        (out / "manifest.json").write_text(json.dumps({"representation": "native_capture", "files": entries}, indent=2) + "\n")
    finally:
        if container:
            run(["docker", "rm", "-f", name], capture_output=True)


if __name__ == "__main__":
    main()
