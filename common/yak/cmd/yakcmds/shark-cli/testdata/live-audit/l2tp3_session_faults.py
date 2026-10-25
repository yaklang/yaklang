"""Declared offline control/session faults from Wireshark issue7711 lns-eth.

Usage: l2tp3_session_faults.py ALL_FRAME_ORACLE OUT TSHARK SDK
No network packets are transmitted. Keep every generated pcap/raw export
outside Git. Original packet bytes are retained except explicit ID/bootstrap
mutations; reference dissection counts are reported, never normalized.
"""
import base64
import collections
import hashlib
import json
from pathlib import Path
import struct
import subprocess
import sys

oracle, out, tshark, sdk = map(Path, sys.argv[1:5])
for p in (oracle, out):
    if any((q/'.git').exists() for q in (p.resolve(), *p.resolve().parents)):
        raise ValueError('raw capture evidence must remain outside Git')


def sha(w): return hashlib.sha256(w).hexdigest()
packets = json.loads(oracle.read_text(encoding='utf-8-sig'))
# Complete original prefix includes both the first retired session and its
# later replacement. Omitting those intervening controls would create a gap.
rows = [(int(p['_source']['layers']['frame']['frame.number']), bytes.fromhex(p['_source']['layers']['frame_raw'][0])) for p in packets]
base = [(n, w, len(w)) for n, w in rows if n <= 32]
assert len(base) == 32


def transform(w, fresh_ids=False, conflict=False):
    b = bytearray(w)
    if len(b) < 42 or b[23] != 17:
        return bytes(b)
    iplen = (b[14] & 15)*4
    u = 14+iplen
    at = u+8
    if b[at+1] & 15 != 3:
        return bytes(b)
    control = bool(b[at] & 128)
    def replace(pos):
        v = int.from_bytes(b[pos:pos+4], 'big')
        if v:
            b[pos:pos+4] = ((v+0x11111111) & 0xffffffff or 1).to_bytes(4, 'big')
    if fresh_ids:
        replace(at+4)  # receiver CCID or receiver SID, both 32 bits
    if control:
        end = at+int.from_bytes(b[at+2:at+4], 'big')
        pos = at+12
        while pos < end:
            length = int.from_bytes(b[pos:pos+2], 'big') & 1023
            assert length >= 6 and pos+length <= end
            typ = int.from_bytes(b[pos+4:pos+6], 'big')
            if fresh_ids and typ in (61,63,64):
                replace(pos+6)
            if conflict and typ == 7:
                b[pos+6] ^= 1
            pos += length
    # IPv4 permits a zero UDP checksum. No length or IP header is changed.
    b[u+6:u+8] = b'\0\0'
    return bytes(b)


rq = next(r for r in base if r[0] == 3)
cn = next(r for r in base if r[0] == 5)
data = base[-1]
without_cn = [r for r in base if r[0] != 5]
reordered = list(base)
reordered[2], reordered[3] = reordered[3], reordered[2]
short = (cn[0], cn[1][:-4], cn[2])
cases = {
    'baseline': (base, 1, 'unchanged original prefix through first data on replacement session'),
    'reply-before-request': (reordered, 1, 'SCCRP before SCCRQ; responder ephemeral port retained'),
    'duplicate-setup': (base[:5]+[cn,cn]+base[5:], 1, 'same SCCCN repeated; no state growth or sequence advancement'),
    'missing-control': (without_cn, 0, 'SCCCN missing; later Ns and Nr must not invent its parameters'),
    'repaired-control-gap': ([r for r in base[:-1] if r[0] != 5]+[cn,data], 1, 'SCCCN restored after pending session exchanges'),
    'data-before-control-repair': (without_cn+[cn,data], 1, 'unbound data discarded; only subsequent whole data admitted'),
    'snaplen-control': ([short if r[0] == 5 else r for r in base], 0, 'locally cut SCCCN cannot complete setup'),
    'snaplen-control-repaired': ([short if r[0] == 5 else r for r in base[:-1]]+[cn,data], 1, 'complete SCCCN repairs the local cut without invented bytes'),
    'fresh-control-ids-same-UDP-tuple': (base+[(n,transform(w,fresh_ids=True),length) for n,w,length in base], 2, 'declared fresh 32-bit control/session IDs; same outer tuple isolated by logical generation'),
    'ambiguous-reused-control-id': (base+[(n,transform(w,conflict=True),length) for n,w,length in base], 1, 'same receiver control IDs with conflicting bootstrap; later data quarantined'),
}

out.mkdir(parents=True, exist_ok=True)
results = {}
for name, (frames, expected, mutation) in cases.items():
    d = out/name
    d.mkdir(exist_ok=True)
    cap = d/'fault.pcap'
    wire = bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
    for i, (original, w, length) in enumerate(frames):
        wire += struct.pack('<IIII',1700000000,i*1000,len(w),length)+w
    cap.write_bytes(wire)
    jobs = [([sdk,'replay',cap,'-','--ignore-embedded-secrets'],'events.jsonl'), ([tshark,'-2','-n','-r',cap,'-d','udp.port==49898,l2tp','-T','json','-x','--no-duplicate-keys'],'oracle.json')]
    for args, target in jobs:
        with (d/target).open('wb') as stdout, (d/(target+'.stderr')).open('wb') as stderr:
            subprocess.run(list(map(str,args)),stdout=stdout,stderr=stderr,check=True,timeout=120)
    es = [json.loads(s) for s in (d/'events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
    admitted = [e for e in es if e['protocol'] == 'l2tp' and (e.get('session') or {}).get('Payload Decoded')]
    assert len(admitted) == expected, (name, expected, len(admitted))
    checks = 1
    for e in admitted:
        assert len(e['session']['Context Proof']) == 6
        assert e['session']['Control Setup Complete'] and e['session']['Session Setup Complete']
        assert sha(base64.b64decode(e['session']['Ethernet Bytes'])) == 'bc5630c2bb6027f89cad33f75b2bd16f70537b5fd9c5395c2a4ca088dbbd8b35'
        checks += 4
    children = [e for e in es if 'L2TPv3' in (e.get('carrier_layers') or [])]
    assert len(children) == expected, (name,'inner observation count',len(children),expected)
    if expected == 2:
        assert len({e['domain']['Encapsulation'] for e in children}) == 2
        assert len({e['flow'] for e in admitted}) == 2
        checks += 2
    reference = json.loads((d/'oracle.json').read_text(encoding='utf-8-sig'))
    # Keep the reference's actual inner dissection count even when it accepts
    # data after an unobserved setup exchange that native observation refuses.
    reference_inner = collections.Counter()
    for p in reference:
        ls = p['_source']['layers']
        if 'l2tp' in ls:
            for k in ('icmpv6','icmp','arp'):
                if k in ls:
                    reference_inner[k] += 1
    results[name] = {'mutation':mutation,'capture_sha256':sha(wire),'physical_frames':len(frames),'original_frame_map':[n for n,_,_ in frames],'expected_admitted_data':expected,'native_admitted_data':len(admitted),'native_inner_protocols':dict(collections.Counter(e['protocol'] for e in children)),'reference_inner_protocols':dict(reference_inner),'native_status':dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),'checks':checks}
    print(json.dumps({'case':name,'native_admitted':len(admitted),'reference_inner':dict(reference_inner)}),flush=True)
(out/'fault-summary.json').write_text(json.dumps({'scope':'explicit offline derivatives of original real lns-eth prefix; no new network traffic, authentication or endpoint delivery claim','source_oracle_sha256':sha(oracle.read_bytes()),'cases':results},indent=2)+'\n',encoding='utf-8')
