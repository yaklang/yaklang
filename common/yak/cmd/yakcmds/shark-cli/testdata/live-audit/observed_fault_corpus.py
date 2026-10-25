"""Derive explicitly labelled offline RTMP fault captures from the Wiki sample.

Usage: observed_fault_corpus.py rtmp_sample.cap EXTERNAL_OUTPUT_DIRECTORY
Does not send traffic. Captures, manifests and derived bytes stay outside Git.
Frame operations intentionally target this 26-frame simple-handshake fixture;
they are not a generic packet editor and do not claim new independent traffic.
"""
import hashlib, json, struct, sys
from pathlib import Path

source, out = map(Path, sys.argv[1:3])
for path in (source, out):
    if any((p / ".git").exists() for p in [path.resolve(), *path.resolve().parents]):
        raise SystemExit("Captures and output must remain outside Git")
wire = source.read_bytes()
if wire[:4] != b"\xd4\xc3\xb2\xa1" or struct.unpack_from("<I", wire, 20)[0] != 1:
    raise SystemExit("Expected the classic little-endian Ethernet Wiki RTMP capture")
records, pos = [], 24
while pos < len(wire):
    if len(wire) - pos < 16: raise SystemExit("Truncated capture record header")
    sec, usec, n, original = struct.unpack_from("<IIII", wire, pos)
    if n > len(wire) - pos - 16: raise SystemExit("Truncated capture record")
    records.append((wire[pos+16:pos+16+n], original)); pos += 16+n
if len(records) != 26: raise SystemExit("Expected exactly 26 original frames")

def transport(w):
    if len(w) < 54 or w[12:14] != b"\x08\x00" or w[14] >> 4 != 4 or w[23] != 6:
        raise ValueError("Expected original unfragmented IPv4 TCP")
    ip = (w[14] & 15) * 4
    off = 14 + ip
    tcp = (w[off+12] >> 4) * 4
    if ip < 20 or tcp < 20 or off + tcp > len(w): raise ValueError("Bad observed header")
    return off, tcp

def checksum(w):
    if len(w) & 1: w += b"\0"
    n = sum(struct.unpack("!" + "H" * (len(w)//2), w))
    while n >> 16: n = (n & 65535) + (n >> 16)
    return (~n) & 65535

def rewrite(w, delta=0, reset=False):
    off, tcp = transport(w); w = bytearray(w)
    if reset:
        w = w[:off+tcp]; w[off+13] = 0x14
        struct.pack_into("!H", w, 16, len(w)-14)
    seq, ack = struct.unpack_from("!II", w, off+4)
    struct.pack_into("!I", w, off+4, (seq+delta)&0xffffffff)
    if w[off+13] & 16: struct.pack_into("!I", w, off+8, (ack+delta)&0xffffffff)
    w[24:26] = b"\0\0"; struct.pack_into("!H", w, 24, checksum(bytes(w[14:off])))
    w[off+16:off+18] = b"\0\0"
    total = struct.unpack_from("!H", w, 16)[0]
    body = bytes(w[off:14+total]); pseudo = bytes(w[26:34])+bytes([0,6])+struct.pack("!H",len(body))
    struct.pack_into("!H", w, off+16, checksum(pseudo+body))
    return bytes(w), len(w)

# Refuse a similarly sized unrelated file before applying fixture frame numbers.
for index in (0, 1, 12, 13, 25): transport(records[index][0])
if 1935 not in struct.unpack_from("!HH", records[0][0], transport(records[0][0])[0]):
    raise SystemExit("Expected the Wiki RTMP endpoints")
if records[3][0][transport(records[3][0])[0]+transport(records[3][0])[1]] != 3:
    raise SystemExit("Expected the observed version3 initial handshake")

def capture(rows):
    header = wire[:24]; sec, usec = struct.unpack_from("<II",wire,24); t = sec*1000000+usec
    for i,(packet,original) in enumerate(rows):
        now=t+i*1000
        header += struct.pack("<IIII",now//1000000,now%1000000,len(packet),original)+packet
    return header

swapped = list(records); swapped[12], swapped[13] = swapped[13], swapped[12]
duplicate = records[:14]+[records[13]]+records[14:]
truncated = list(records); off,tcp=transport(records[13][0]); truncated[13]=(records[13][0][:off+tcp+80],records[13][1])
reuse = records+[rewrite(records[-1][0],reset=True)]+[rewrite(w,delta=1000000) for w,_ in records]
cases = {
    "reordered-echo": (swapped,"Original frames 13 and 14 arrive in reverse order; TCP must recover identical complete payloads"),
    "duplicate-data": (duplicate,"Duplicate original frame14 with unchanged TCP sequence; no duplicate logical command"),
    "lost-echo": (records[:12]+records[13:],"Drop original frame13; missing handshake bytes must not be invented"),
    "truncated-connect": (truncated,"Snaplen truncation of original frame14; retain original on-wire length"),
    "same-tuple-new-generation": (reuse,"Observed RST then same endpoints/ports and new ISNs; checksums regenerated; no command association across generations"),
}
out.mkdir(parents=True,exist_ok=True)
manifest={"source_sha256":hashlib.sha256(wire).hexdigest(),"source_frames":len(records),"kind":"offline derived fault fixtures; not new live captures","cases":[]}
for name,(rows,why) in cases.items():
    b=capture(rows); path=out/(name+".pcap"); path.write_bytes(b)
    manifest["cases"].append({"name":name,"file":path.name,"sha256":hashlib.sha256(b).hexdigest(),"frames":len(rows),"operation":why})
(out/"manifest.json").write_text(json.dumps(manifest,indent=2)+"\n",encoding="utf-8")
print(json.dumps({"source_frames":len(records),"derived_cases":len(cases)}))
