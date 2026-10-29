#!/usr/bin/env python3
"""Authored SSH wire controls. No product decoder or runtime output is imported."""
import argparse
import base64
import hashlib
import ipaddress
import json
from pathlib import Path
import struct

PRIMARY = Path(__file__).resolve().parent
MARKERS = {
    "ext-info-c", "ext-info-s",
    "kex-strict-c-v00@openssh.com", "kex-strict-s-v00@openssh.com",
}
LIBSSH = "curve25519-sha256@libssh.org"
CURVE = "curve25519-sha256"
DH = "diffie-hellman-group14-sha256"
GEX = "diffie-hellman-group-exchange-sha256"
NIST = "ecdh-sha2-nistp256"
ALGORITHMS = [
    ("diffie-hellman-group1-sha1", "dh", "rfc4253.txt", "8", ["diffie-hellman-group1-sha1"]),
    ("diffie-hellman-group14-sha1", "dh", "rfc4253.txt", "8", ["diffie-hellman-group14-sha1"]),
    ("diffie-hellman-group14-sha256", "dh", "rfc8268.txt", "3", ["diffie-hellman-group14-sha256"]),
    ("diffie-hellman-group15-sha512", "dh", "rfc8268.txt", "3", ["diffie-hellman-group15-sha512"]),
    ("diffie-hellman-group16-sha512", "dh", "rfc8268.txt", "3", ["diffie-hellman-group16-sha512"]),
    ("diffie-hellman-group17-sha512", "dh", "rfc8268.txt", "3", ["diffie-hellman-group17-sha512"]),
    ("diffie-hellman-group18-sha512", "dh", "rfc8268.txt", "3", ["diffie-hellman-group18-sha512"]),
    ("diffie-hellman-group-exchange-sha1", "dh-gex", "rfc4419.txt", "4", ["diffie-hellman-group-exchange-sha1"]),
    ("diffie-hellman-group-exchange-sha256", "dh-gex", "rfc4419.txt", "4", ["diffie-hellman-group-exchange-sha256"]),
    (NIST, "ecdh-nistp256", "rfc5656.txt", "4, 6.1", ["ECDH_INIT", "nistp256"]),
    (CURVE, "ecdh-curve25519", "rfc8731.txt", "3", [CURVE]),
    (LIBSSH, "ecdh-curve25519", "rfc8731.txt", "1, 4", [LIBSSH]),
]
HOST_PUBLIC = bytes.fromhex("d75a980182b10ab7d54bfed3c964073a0ee172f3daa62325af021a68f707511a")
CLIENT_PUBLIC = bytes(range(1, 33))
SERVER_PUBLIC = bytes(range(33, 65))
P256 = bytes.fromhex(
    "046b17d1f2e12c4247f8bce6e563a440f277037d812deb33a0f4a13945d898c296"
    "4fe342e2fe1a7f9b8ee7eb4a7c0f9e162bce33576b315ececbb6406837bf51f5")
BANNER_C = b"SSH-2.0-SourceControlClient49\r\n"
BANNER_S = b"SSH-2.0-SourceControlServer49\r\n"

def sha(b):
    return hashlib.sha256(b).hexdigest()

def jbytes(v):
    return (json.dumps(v, ensure_ascii=False, sort_keys=True, indent=2) + "\n").encode()

def dump(path, v):
    path.write_bytes(jbytes(v))

def string(b):
    if isinstance(b, str):
        b = b.encode("ascii")
    return struct.pack("!I", len(b)) + b

def mpint(n):
    b = n.to_bytes(max(1, (n.bit_length()+7)//8), "big") if n else b""
    if b and b[0] & 0x80:
        b = b"\0" + b
    return string(b)

HOST = string("ssh-ed25519") + string(HOST_PUBLIC)
SIGNATURE = string("ssh-ed25519") + string(bytes(64))
# These are public structural fixtures; neither signature nor key exchange is verified.
FINGERPRINT = "SHA256:" + base64.b64encode(hashlib.sha256(HOST).digest()).decode().rstrip("=")

def frame(payload):
    pad = 4
    while (4 + 1 + len(payload) + pad) % 8:
        pad += 1
    return struct.pack("!I", 1 + len(payload) + pad) + bytes([pad]) + payload + bytes([0xA5])*pad

def kex_payload(names, follows=False):
    lists = [
        ",".join(names), "ssh-ed25519",
        "aes128-ctr", "aes128-ctr",
        "hmac-sha2-256", "hmac-sha2-256", "none", "none", "", "",
    ]
    return b"\x14" + bytes(range(16)) + b"".join(string(x) for x in lists) + bytes([bool(follows)]) + bytes(4)

def real_names(names):
    return [n for n in names if n not in MARKERS]

def select(client, server):
    offered = set(real_names(server))
    return next((n for n in real_names(client) if n in offered), None)

def body_fields(kind, payload):
    """Independent literal spans are offsets inside the complete binary SSH PDU."""
    rows = []
    pos = 6
    def atom(name, width, value):
        nonlocal pos
        rows.append({"name": name, "span": [pos, pos+width], "value": value})
        pos += width
    def sfield(name, expected):
        nonlocal pos
        atom(name + " Length", 4, len(expected))
        rows.append({"name": name, "span": [pos, pos+len(expected)],
                     "bytes_SHA256": sha(expected), "byte_length": len(expected)})
        pos += len(expected)
    if kind == "kexinit":
        atom("Cookie", 16, list(range(16)))
        names = ["Kex Algorithms", "Host Key Algorithms", "Encryption C2S", "Encryption S2C",
                 "MAC C2S", "MAC S2C", "Compression C2S", "Compression S2C", "Languages C2S", "Languages S2C"]
        p = 17
        for name in names:
            n = struct.unpack("!I", payload[p:p+4])[0]
            b = payload[p+4:p+4+n]
            sfield(name, b)
            rows[-1]["ascii"] = b.decode("ascii")
            p += 4+n
        atom("First Kex Packet Follows", 1, bool(payload[p]))
        atom("Reserved", 4, struct.unpack("!I", payload[p+1:p+5])[0])
    elif kind in ("curve-init", "nist-init"):
        b = CLIENT_PUBLIC if kind == "curve-init" else P256
        sfield("Client Public Value", b)
    elif kind in ("curve-reply", "nist-reply", "dh-reply"):
        sfield("Host Key Blob", HOST)
        b = SERVER_PUBLIC if kind == "curve-reply" else P256 if kind == "nist-reply" else b"\x03"
        sfield("Server Public Value" if kind != "dh-reply" else "Server Exchange Value", b)
        sfield("Signature Blob", SIGNATURE)
    elif kind == "dh-init":
        sfield("Client Exchange Value", b"\x02")
    elif kind == "gex-request":
        for name, value in [("Minimum Bits", 1024), ("Preferred Bits", 2048), ("Maximum Bits", 4096)]:
            atom(name, 4, value)
    elif kind == "gex-group":
        # Deliberately structural only. These small integers are not an acceptable DH group.
        sfield("Prime", b"\x17")
        sfield("Generator", b"\x02")
    elif kind == "opaque":
        b = payload[1:]
        rows.append({"name": "Opaque Method Payload", "span": [6, 6+len(b)],
                     "bytes_SHA256": sha(b), "byte_length": len(b),
                     "semantic_layout_selected": False})
    elif kind == "newkeys":
        assert payload == b"\x15"
    else:
        raise ValueError(kind)
    return rows

def internet_checksum(raw):
    if len(raw) % 2:
        raw += b"\0"
    n = sum(struct.unpack("!" + "H"*(len(raw)//2), raw))
    while n >> 16:
        n = (n & 65535) + (n >> 16)
    return (~n) & 65535

def ethernet_tcp(src, dst, sport, dport, seq, ack, flags, payload, ident):
    srcb, dstb = ipaddress.ip_address(src).packed, ipaddress.ip_address(dst).packed
    tcp = struct.pack("!HHIIBBHHH", sport, dport, seq, ack, 5 << 4, flags, 32768, 0, 0)
    tcp_sum = internet_checksum(srcb+dstb+b"\0\x06"+struct.pack("!H", len(tcp)+len(payload))+tcp+payload)
    tcp = tcp[:16]+struct.pack("!H", tcp_sum)+tcp[18:]
    ip = struct.pack("!BBHHHBBH4s4s", 0x45, 0, 40+len(payload), ident, 0x4000, 64, 6, 0, srcb, dstb)
    ip = ip[:10]+struct.pack("!H", internet_checksum(ip))+ip[12:]
    ether = bytes.fromhex("0200000000020200000000010800")
    return ether+ip+tcp+payload

def make_cases():
    cases = []
    def add(name, client, server=None, profile=None, sequence=None, **kw):
        if server is None:
            server = list(client)
        cases.append({"sourcecase": name, "client_names": client, "server_names": server,
                      "profile": profile, "sequence": sequence or [], **kw})
    def msg(role, msgnum, body, kind, **kw):
        return {"role": role, "message": msgnum, "payload": bytes([msgnum])+body, "kind": kind, **kw}
    def curve_seq():
        return [msg("client", 30, string(CLIENT_PUBLIC), "curve-init"),
                msg("server", 31, string(HOST)+string(SERVER_PUBLIC)+string(SIGNATURE), "curve-reply")]
    for name, private in [
        ("01-ext-info-private-prefix", "ext-info-private@example.test"),
        ("02-kex-strict-private-prefix", "kex-strict-private@example.test"),
    ]:
        # Known-looking data does not select the unknown private method's semantics.
        seq = curve_seq()
        for x in seq:
            x["source_wire_shape_only"] = x["kind"]
            x["kind"] = "opaque"
        add(name, [private, CURVE], profile=None, sequence=seq,
            issue="A name sharing a marker prefix is a selectable literal private algorithm, not a marker.")
    add("03-dh-private-prefix", ["diffie-hellman-group-private@example.test"], profile=None, sequence=[
        msg("client", 30, mpint(2), "opaque"),
        msg("server", 31, string(HOST)+mpint(3)+string(SIGNATURE), "opaque")],
        issue="An unknown private name beginning with diffie-hellman-group does not select the standard DH layout.")
    add("04-gex-private-prefix", ["diffie-hellman-group-exchange-private@example.test"], profile=None, sequence=[
        msg("client", 34, struct.pack("!III", 1024, 2048, 4096), "opaque"),
        msg("server", 31, mpint(23)+mpint(2), "opaque")],
        issue="Unknown private group-exchange-prefixed method stays opaque; small group integers are not asserted cryptographically valid.")
    add("05-curve-substring-private", ["private-curve25519-method@example.test"], profile=None, sequence=[
        msg("client", 30, string(CLIENT_PUBLIC), "opaque"),
        msg("server", 31, string(b"not-a-host-key"), "opaque")],
        issue="A curve25519 substring is not method-family evidence.")
    add("06-unknown-reply-no-hostkey", ["private@example.test"], profile=None, sequence=[
        msg("client", 30, string(b"method-specific-client-value"), "opaque"),
        msg("server", 31, string(b"not-a-host-key"), "opaque")],
        issue="The first SSH string of an unselected method reply is not proven to be a host key.")
    seq7 = curve_seq()+[
        msg("server", 21, b"", "newkeys"),
        {"role": "server", "opaque_after_newkeys": b"SSH-2.0-NotANewBanner49\r\n",
         "kind": "protected-phase-sentinel"},
        msg("client", 21, b"", "newkeys"),
        {"role": "client", "opaque_after_newkeys": frame(b"\x14"+bytes(24)),
         "kind": "protected-phase-sentinel"},
    ]
    add("07-standard-curve-markers-and-reuse", ["ext-info-c", CURVE, "kex-strict-c-v00@openssh.com"],
        ["ext-info-s", CURVE, "kex-strict-s-v00@openssh.com"], "ecdh-curve25519", seq7,
        reuse_tuple_of="06-unknown-reply-no-hostkey",
        issue="Exact markers excluded only as markers; a new SYN generation cannot inherit the preceding private method. Per-direction NEWKEYS freezes clear parsing only in that direction.")
    add("08-libssh-correct-guess-server-first", [LIBSSH], profile="ecdh-curve25519",
        sequence=curve_seq(), first_follows=True, server_first=True,
        issue="Explicit RFC8731 vendor alias and correct guessed packet retain the selected Curve25519 layout.")
    add("09-nistp256-reverse-endpoints", [NIST], profile="ecdh-nistp256", sequence=[
        msg("client", 30, string(P256), "nist-init"),
        msg("server", 31, string(HOST)+string(P256)+string(SIGNATURE), "nist-reply")],
        reverse_endpoints=True, issue="Client role comes from SYN evidence even when the client address is numerically greater.")
    add("10-dh14-sha256", [DH], profile="dh", sequence=[
        msg("client", 30, mpint(2), "dh-init"),
        msg("server", 31, string(HOST)+mpint(3)+string(SIGNATURE), "dh-reply")],
        issue="Exact RFC8268 method keeps DH syntax. No DH group/peer/signature verification is asserted.")
    add("11-gex-sha256-request", [GEX], profile="dh-gex", sequence=[
        msg("client", 34, struct.pack("!III", 1024, 2048, 4096), "gex-request")],
        issue="Exact RFC4419 GEX request uses its method-specific message34 and three uint32 fields.")
    add("12-first-guess-wrong-then-correct", [DH, CURVE], [CURVE], "ecdh-curve25519", [
        msg("client", 30, mpint(2), "opaque", ignored_guess=True),
        *curve_seq()],
        first_follows=True,
        issue="After both KEXINITs, the wrong guessed packet is ignored without changing the chosen method; the following Curve25519 packet is selected normally.")
    return cases

def witness(file, terms):
    p = PRIMARY/"primary01"/file
    lines = p.read_text(encoding="utf-8").splitlines()
    return {"path": str(p), "SHA256": sha(p.read_bytes()),
            "matches": [{"term": term, "line_numbers": [i+1 for i, line in enumerate(lines) if term in line]}
                        for term in terms]}

def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", required=True, type=Path)
    args = ap.parse_args()
    out = args.out.resolve()
    out.mkdir(parents=True, exist_ok=False)
    (out/"raw").mkdir()
    script_sha = sha(Path(__file__).read_bytes())
    cases = make_cases()
    facts = []
    records = []
    frame_no = 0
    usec = 0
    source_paths = []
    tuples = {}
    all_probe = []
    for ci, case in enumerate(cases, 1):
        name = case["sourcecase"]
        default = ("192.0.2.1", "192.0.2.2", 51000+ci, 2222)
        if case.get("reverse_endpoints"):
            default = ("192.0.2.2", "192.0.2.1", 51000+ci, 2222)
        clientip, serverip, clientport, serverport = tuples.get(case.get("reuse_tuple_of"), default)
        tuples[name] = (clientip, serverip, clientport, serverport)
        seq = {"client": ci*100000+1000, "server": ci*100000+50000}
        peer = {"client": "server", "server": "client"}
        endpoints = {"client": (clientip, clientport), "server": (serverip, serverport)}
        emitted = []
        def emit(role, payload=b"", flags=0x18, step_kind="tcp-control", detail=None):
            nonlocal frame_no, usec
            other = peer[role]
            src, sport = endpoints[role]
            dst, dport = endpoints[other]
            ack = 0 if flags == 0x02 else seq[other]
            packet = ethernet_tcp(src, dst, sport, dport, seq[role], ack, flags, payload, frame_no+1)
            frame_no += 1
            sec = 1700000049 + usec//1000000
            sub = usec % 1000000
            usec += 2000
            records.append(struct.pack("<IIII", sec, sub, len(packet), len(packet))+packet)
            item = {"frame": frame_no, "role": role, "TCP Sequence": seq[role], "TCP Acknowledgment": ack,
                    "TCP Flags": flags, "captured_length": len(packet), "original_length": len(packet),
                    "IPv4 Declared Length": len(packet)-14, "IPv4 Header Length": 20,
                    "physical_TCP_payload_span": [54, len(packet)], "TCP_payload_length": len(payload),
                    "wire_SHA256": sha(packet), "payload_SHA256": sha(payload), "kind": step_kind}
            if payload:
                rawpath = out/"raw"/(name+"-"+str(frame_no)+".bin")
                rawpath.write_bytes(payload)
                item["input_path"] = str(rawpath)
                source_paths.append(rawpath)
            if detail:
                item.update(detail)
            emitted.append(item)
            seq[role] += len(payload) + bool(flags & 0x02) + bool(flags & 0x01)
            return item
        emit("client", flags=0x02)
        emit("server", flags=0x12)
        emit("client", flags=0x10)
        roles = ["server", "client"] if case.get("server_first") else ["client", "server"]
        for role in roles:
            emit(role, BANNER_C if role == "client" else BANNER_S, step_kind="identification")
        selected = select(case["client_names"], case["server_names"])
        if selected is None:
            raise ValueError("fixture has no method")
        for role in roles:
            names = case["client_names"] if role == "client" else case["server_names"]
            follows = bool(case.get("first_follows")) if role == "client" else False
            payload = kex_payload(names, follows)
            pdu = frame(payload)
            emit(role, pdu, step_kind="KEXINIT", detail={
                "SSH_message": 20, "source_fields": body_fields("kexinit", payload),
                "offers": names, "first_kex_packet_follows": follows,
                "binary_packet_length": len(pdu), "framing": {"Packet Length": [0,4], "Padding Length": [4,5], "Message Number": [5,6]}})
        newkeys = {"client": False, "server": False}
        for step in case["sequence"]:
            role = step["role"]
            if "opaque_after_newkeys" in step:
                assert newkeys[role]
                emit(role, step["opaque_after_newkeys"], step_kind=step["kind"], detail={
                    "source_visibility": "opaque after this direction sent NEWKEYS",
                    "claim": "synthetic post-NEWKEYS sentinel; no encryption or AEAD/MAC conformance asserted",
                    "must_not_create_plaintext_KEX_or_banner": True})
                continue
            assert not newkeys[role]
            payload = step["payload"]
            pdu = frame(payload)
            kind = step["kind"]
            detail = {
                "SSH_message": step["message"], "selected_literal_KEX": selected,
                "selected_method_profile": case["profile"],
                "source_fields": body_fields(kind, payload),
                "binary_packet_length": len(pdu),
                "semantic_selection": "ignored guessed packet" if step.get("ignored_guess") else
                    "selected known wire layout" if case["profile"] else "unselected private method; opaque",
                "ignored_guess": bool(step.get("ignored_guess")),
                "framing": {"Packet Length": [0,4], "Padding Length": [4,5], "Message Number": [5,6]},
            }
            if case["profile"] is None or step.get("ignored_guess"):
                detail["forbidden_standard_method_claims"] = [
                    "Client Exchange Value", "Client Public Value", "Server Exchange Value", "Server Public Value",
                    "Host Key Format", "Host Key Fingerprint", "Host Key SHA256", "Signature Blob",
                    "Minimum Bits", "Preferred Bits", "Maximum Bits", "Prime", "Generator"]
            elif kind.endswith("reply"):
                detail["structural_hostkey_blob_SHA256"] = sha(HOST)
                detail["structural_hostkey_fingerprint"] = FINGERPRINT
                detail["hostkey_signature_and_exchange_math_verified"] = False
            emit(role, pdu, step_kind=kind, detail=detail)
            if step["message"] == 21:
                newkeys[role] = True
        emit("client", flags=0x11)
        emit("server", flags=0x11)
        emit("client", flags=0x10)
        facts.append({
            "sourcecase": name, "scope": "authored transport/field-admission control, not a completed authenticated SSH session",
            "tuple": {"client": f"{clientip}:{clientport}", "server": f"{serverip}:{serverport}", "transport": "tcp"},
            "TCP_generation_source": {"SYN": emitted[0]["frame"], "SYNACK": emitted[1]["frame"],
                                      "client_ISN": ci*100000+1000, "server_ISN": ci*100000+50000},
            "literal_client_names": case["client_names"], "literal_server_names": case["server_names"],
            "selected_literal_KEX": selected, "selected_method_profile": case["profile"],
            "expected_human_family": ("ECDH" if case["profile"] and case["profile"].startswith("ecdh") else
                                      "Diffie-Hellman" if case["profile"] in ("dh", "dh-gex") else "unconfirmed"),
            "issue": case["issue"], "steps": emitted,
            "tuple_reuse_of": case.get("reuse_tuple_of"),
            "identity": "SSH is proven by two identification banners and complete bounded KEXINIT framing; unknown method semantics are not proven",
            "not_claimed": ["host authentication", "signature verification", "key exchange mathematics", "ciphertext decryption", "SFTP", "user authentication"],
        })
        for item in emitted:
            if item["kind"] != "tcp-control":
                all_probe.append({"sourcecase": name, "frame": item["frame"], "direction": item["role"],
                                  "input_path": item["input_path"], "byte_offset": 0,
                                  "byte_length": item["TCP_payload_length"], "raw_SHA256": item["payload_SHA256"],
                                  "kind": item["kind"]})
    capture = out/"ssh-source49-authored12.pcap"
    capture.write_bytes(struct.pack("<IHHIIII", 0xA1B2C3D4, 2, 4, 0, 0, 262144, 1)+b"".join(records))
    source_paths.append(capture)
    allowlist = {
        "scope": "Exact names selecting currently implemented complete initial clear-transport method-field grammar; not a cryptographic policy recommendation.",
        "marker_names": sorted(MARKERS),
        "marker_evidence": [
            witness("rfc8308.txt", ['"ext-info-s"', '"ext-info-c"', "any position"]),
            {"path": str(PRIMARY/"primary02"/"PROTOCOL"),
             "SHA256": sha((PRIMARY/"primary02"/"PROTOCOL").read_bytes()), "line_numbers": [146,147]},
        ],
        "selected_complete_layout": [
            {"name": n, "profile": p, "primary_section": sec, "evidence": witness(f, terms)}
            for n,p,f,sec,terms in ALGORITHMS],
        "known_standard_but_current_field_grammar_unselected": [
            {"name": n, "reason": why} for n,why in [
                ("ecdh-sha2-nistp384", "Existing selected point grammar is P-256 only; do not claim the P-384 body is supported."),
                ("ecdh-sha2-nistp521", "Existing selected point grammar is P-256 only; do not claim the P-521 body is supported."),
                ("curve448-sha512", "The selected Curve25519 grammar requires 32 bytes, while RFC8731 Curve448 requires 56.")]],
        "exact_matching_policy": "Use case-sensitive exact names. Private @domain names may share any textual prefix; none become a marker or known layout from prefix/substring alone.",
        "private_name_primary": witness("rfc4251.txt", ["name@domainname", "Names are case-sensitive", "30 to 49"]),
        "guess_primary": witness("rfc4253.txt", ["next packet MUST be silently ignored", "guessed packet"]),
    }
    dump(out/"allowlist-and-markers.json", allowlist)
    dump(out/"source-facts.json", {"authored": True, "cases": facts})
    with (out/"probe-input.jsonl").open("wb") as f:
        for row in all_probe:
            f.write(json.dumps(row, sort_keys=True).encode()+b"\n")
    sources = [Path(__file__), *source_paths, out/"allowlist-and-markers.json", out/"source-facts.json", out/"probe-input.jsonl"]
    manifest = {
        "schema": "SSH-source49-frozen-authored-v1", "authored": True,
        "generator": {"path": str(Path(__file__).resolve()), "SHA256": script_sha},
        "no_candidate_runtime_or_product_decoder_imported": True,
        "no_private_key_or_keylog": True, "real_capture_count": 0,
        "scope": "12 complete TCP connection/source-admission controls; binary framing and exact method selection only",
        "primary_manifests": [
            {"path": str(PRIMARY/p/"manifest.json"), "SHA256": sha((PRIMARY/p/"manifest.json").read_bytes())}
            for p in ("primary01","primary02")],
        "capture": {"path": str(capture), "SHA256": sha(capture.read_bytes()), "frames": frame_no, "DLT": 1},
        "files": [{"path": str(p), "bytes": p.stat().st_size, "SHA256": sha(p.read_bytes())} for p in sources],
        "counts": {"connections": len(cases), "negative_method_cases": 6, "positive_cases": 6,
                   "physical_frames": frame_no, "TCP_payload_steps": len(all_probe),
                   "exact_layout_allowlist_names": len(ALGORITHMS), "exact_marker_names": len(MARKERS),
                   "four_tuple_reuse_pairs": 1, "wrong_guess_cases": 1, "post_NEWKEYS_sentinels": 2},
        "freezing_rule": "Do not edit. Corrections use a new directory; old packets/plans and failures remain.",
        "next_action": "Root may run already closed bins-48 on this PCAP to reproduce baseline differences; this plan has not read those outputs.",
    }
    dump(out/"manifest.json", manifest)
    print(json.dumps({"out": str(out), "manifest_SHA256": sha((out/"manifest.json").read_bytes()), "counts": manifest["counts"]}))

if __name__ == "__main__":
    main()

