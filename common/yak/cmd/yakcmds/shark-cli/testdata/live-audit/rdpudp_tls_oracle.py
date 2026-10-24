"""Supplemental RDPUDP2/TLS oracle, independent of Yak's carrier receiver.

Usage: rdpudp_tls_oracle.py CAPTURE OUT TSHARK EVENTS_JSONL
Original capture comparisons remain separate. This writes a DERIVED TCP PCAP
outside Git so Wireshark's TLS dissector can verify the ordered RDPUDP2 bytes
without its RDPUDP2 retransmission/desegmentation behavior. Synthetic endpoints,
TCP headers, timestamps and sequence numbers are not original-capture evidence.
"""
import base64
import collections
import csv
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import sys

capture, out, tshark, events = map(Path, sys.argv[1:5])
for p in (capture, out):
    if any((q / '.git').exists() for q in [p.resolve(), *p.resolve().parents]):
        raise SystemExit('Raw capture and outputs must be outside Git')
out.mkdir(parents=True, exist_ok=True)
fields = ['frame.number', 'frame.interface_id', 'ipv6.src', 'ipv6.dst',
          'udp.srcport', 'udp.dstport', 'udp.payload',
          'rdpudp.data.channelseqnumber', 'rdpudp.data.channelfullseqnumber']
args = [str(tshark), '-2', '-n', '-r', str(capture), '-Y', 'rdpudp',
        '-T', 'fields', '-E', 'header=y']
for name in fields:
    args += ['-e', name]
with (out / 'carrier.tsv').open('wb') as f:
    subprocess.run(args, stdout=f, check=True, timeout=300)

def unwrap(w):
    b = bytearray(w)
    b[0], b[7] = b[7], b[0]
    assert b[0] & 1 == 0
    if (b[0] >> 1) & 15 == 8:
        return None
    flags = int.from_bytes(b[1:3], 'little') & 4095
    if not flags & 4:
        return None
    at = 3
    if flags & 1:
        at += 7 + (b[at + 6] & 15)
    at += bool(flags & 64) + 3 * bool(flags & 256) + 2 * bool(flags & 16)
    at += 2  # transport DataHeader sequence changes on retransmission
    if flags & 8:
        size = b[at + 2]
        at += 3 + (4 if size & 128 else 0) + (size & 127)
    channel = int.from_bytes(b[at:at+2], 'little')
    return channel, bytes(b[at+2:])

chunks = collections.defaultdict(dict)
duplicates = []
with (out / 'carrier.tsv').open(encoding='utf8') as f:
    for row in csv.DictReader(f, delimiter='\t'):
        if not row['rdpudp.data.channelseqnumber']:
            continue
        parsed = unwrap(bytes.fromhex(row['udp.payload']))
        assert parsed is not None
        seq, data = parsed
        assert seq == int(row['rdpudp.data.channelseqnumber'], 0)
        key = (row['frame.interface_id'], row['ipv6.src'], row['udp.srcport'],
               row['ipv6.dst'], row['udp.dstport'])
        full = int(row['rdpudp.data.channelfullseqnumber'], 0)
        frame = int(row['frame.number'])
        if full in chunks[key]:
            old = chunks[key][full]
            assert old[1] == data, 'conflicting source data is not deduplicated'
            duplicates.append({'frame': frame, 'original': old[0], 'channel': full})
        else:
            chunks[key][full] = frame, data

def checksum(b):
    if len(b) % 2: b += b'\0'
    n = sum(struct.unpack('!%dH' % (len(b)//2), b))
    while n >> 16: n = (n & 65535) + (n >> 16)
    return (~n) & 65535

derived = out / 'DERIVED-rdpudp-ordered-tls.pcap'
provenance = []
keys = list(chunks)
assert len(keys) == 2, 'This supplemental oracle requires one observed two-way generation'
with derived.open('wb') as f:
    f.write(struct.pack('<IHHIIII', 0xa1b2c3d4, 2, 4, 0, 0, 65535, 1))
    number = 0
    def packet(direction, sequence, data, frame=0):
        global number
        number += 1
        src, dst = (b'\xc6\x12\x00\x01', b'\xc6\x12\x00\x02')
        sp, dp = 50000, 443
        if direction: src, dst, sp, dp = dst, src, dp, sp
        tcp = struct.pack('!HHIIBBHHH', sp, dp, sequence, 1, 0x50, 0x18, 65535, 0, 0)
        pseudo = src+dst+struct.pack('!BBH',0,6,len(tcp)+len(data))
        tcp = tcp[:16]+struct.pack('!H',checksum(pseudo+tcp+data))+tcp[18:]+data
        ip = struct.pack('!BBHHHBBH4s4s',0x45,0,20+len(tcp),number&65535,0,64,6,0,src,dst)
        ip = ip[:10]+struct.pack('!H',checksum(ip))+ip[12:]
        wire = b'\x00'*12+b'\x08\x00'+ip+tcp
        f.write(struct.pack('<IIII',1,number,len(wire),len(wire))+wire)
        provenance.append({'derived_frame':number,'original_frame':frame})
    # Preserve original inter-direction order after per-direction sequencing.
    ordered = []
    for d, key in enumerate(keys):
        seqs = sorted(chunks[key])
        assert seqs == list(range(seqs[0],seqs[-1]+1)), 'missing source sequence'
        seq = 1
        for s in seqs:
            frame, data = chunks[key][s]
            ordered.append((frame,d,seq,data));seq += len(data)
    for frame,d,seq,data in sorted(ordered): packet(d,seq,data,frame)
(out/'derived-provenance.json').write_text(json.dumps(provenance,indent=2))
args = [str(tshark),'-2','-n','-r',str(derived),'-o','tcp.reassemble_out_of_order:TRUE',
        '-d','tcp.port==443,tls','-Y','tls.record','-T','json','-x','--no-duplicate-keys']
with (out/'derived-tls.json').open('wb') as f:
    subprocess.run(args,stdout=f,check=True,timeout=300)
def records(tree):
    if isinstance(tree,dict):
        for k,v in tree.items():
            if k=='tls.record_raw':
                if v and isinstance(v[0],str): yield bytes.fromhex(v[0])
                else:
                    for item in v: yield bytes.fromhex(item[0])
            elif not k.endswith('_raw'): yield from records(v)
    elif isinstance(tree,list):
        for item in tree: yield from records(item)
ws = collections.Counter()
for p in json.loads((out/'derived-tls.json').read_bytes()):
    for b in records(p):
        assert len(b)>=5 and int.from_bytes(b[3:5],'big')==len(b)-5
        ws[hashlib.sha256(b).hexdigest()] += 1
native = collections.Counter()
for line in events.open(encoding='utf-8-sig'):
    if not line.startswith('{'):continue
    e=json.loads(line)
    if e['protocol']=='tls' and e.get('carrier_layers')==['RDPUDP2']:
        b=base64.b64decode(e['raw'] or '')
        if len(b)>=5 and int.from_bytes(b[3:5],'big')==len(b)-5:
            native[hashlib.sha256(b).hexdigest()] += 1
summary = {'scope':'Supplemental DERIVED TCP TLS dissection; original-capture oracle retained separately',
           'derived_wireshark_records':sum(ws.values()),'native_records':sum(native.values()),
           'matching_occurrences':sum((ws&native).values()),'missing':sum((ws-native).values()),
           'extra':sum((native-ws).values()),'deduplicated_carrier_retransmissions':duplicates,
           'direction_chunk_counts':[len(chunks[k]) for k in keys],
           'derived_capture_sha256':hashlib.sha256(derived.read_bytes()).hexdigest()}
(out/'summary.json').write_text(json.dumps(summary,indent=2))
print(json.dumps({**{k:v for k,v in summary.items() if k!='deduplicated_carrier_retransmissions'},
                  'deduplicated_carrier_retransmissions':len(duplicates)},indent=2))
assert ws==native, 'supplemental TLS record mismatch'
