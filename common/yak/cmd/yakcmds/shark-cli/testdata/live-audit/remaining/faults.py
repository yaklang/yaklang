"""Derive faults from a fresh remaining-protocol run, with provenance.

Usage: faults.py CAPTURE_DIR OUTPUT_DIR TSHARK YAK EVENTS
Checks missing evidence can remove messages, never invent a decoded PDU. TCP
duplicates and snaplen repaired by a later copy must preserve decoded messages.
"""
import collections, concurrent.futures, copy, hashlib, json, shutil, struct, subprocess, sys
from pathlib import Path

cap,out,tshark,yak,audit=map(Path,sys.argv[1:6]);out.mkdir(exist_ok=True)
m=json.loads((cap/'manifest.json').read_text());raw=(cap/'live.pcap').read_bytes()
assert raw[:4] in (bytes.fromhex('4d3cb2a1'),bytes.fromhex('d4c3b2a1')) and struct.unpack_from('<I',raw,20)[0]==0
frames=[];at=24
while at<len(raw):
    sec,frac,n,wire=struct.unpack_from('<IIII',raw,at);at+=16
    frames.append(dict(original=len(frames)+1,sec=sec,frac=frac,wire=wire,data=raw[at:at+n]));at+=n
def packet(f):
    b=f['data'];ip=4
    if b[ip]>>4!=4:return None
    tr=ip+4*(b[ip]&15);proto=b[ip+9]
    if proto not in (6,17):return None
    sp,dp=struct.unpack_from('!HH',b,tr)
    off=tr+(4*(b[tr+12]>>4) if proto==6 else 8)
    return dict(proto=proto,sp=sp,dp=dp,off=off,payload=b[off:],flags=b[tr+13] if proto==6 else 0,tuple=tuple(sorted([sp,dp])))
def signature(e): return e['protocol'],e['entry'],hashlib.sha256(__import__('base64').b64decode(e['raw'] or '')).hexdigest()
baseline=[json.loads(x) for x in (cap/'comparison/events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
good=collections.Counter(signature(e) for e in baseline if e['status']=='decoded' and e['protocol']!='ftp-data')
ftp=collections.defaultdict(bytearray)
for e in baseline:
    if e['protocol']=='ftp-data' and e['status']=='decoded':ftp[e['flow']]+=__import__('base64').b64decode(e['raw'])
variants=[]
def save(name,fs,fault):
    d=out/name;d.mkdir(exist_ok=True);data=bytearray(raw[:24])
    for f in fs:data+=struct.pack('<IIII',f['sec'],f['frac'],len(f['data']),f['wire'])+f['data']
    (d/'live.pcap').write_bytes(data)
    for file in ('manifest.json','session.keys'):shutil.copyfile(cap/file,d/file)
    (d/'provenance.json').write_text(json.dumps(dict(source_sha256=hashlib.sha256(raw).hexdigest(),fault=fault,original_frames=[f['original'] for f in fs]),indent=2))
    variants.append((name,d))
for service,port in m['Ports'].items():
    if '_data_' in service:continue
    indices=[i for i,f in enumerate(frames) if (p:=packet(f)) and port in (p['sp'],p['dp']) and len(p['payload'])>20]
    if not indices:continue
    # Prefer a large body/message after admission rather than a tiny ACK/header.
    target=max(indices[1:] or indices,key=lambda i:len(packet(frames[i])['payload']))
    p=packet(frames[target]);flow=p['tuple']
    save(service+'-loss',[f for i,f in enumerate(frames) if i!=target],dict(dropped=target+1))
    fs=copy.deepcopy(frames);fs[target]['data']=fs[target]['data'][:p['off']+3]
    save(service+'-snaplen',fs,dict(truncated=target+1,captured_payload=3,original_length='preserved'))
    if p['proto']==6:
        fs=copy.deepcopy(frames);fs.insert(target+1,copy.deepcopy(frames[target]))
        save(service+'-duplicate',fs,dict(duplicated=target+1))
        fs=copy.deepcopy(frames);fs[target]['data']=fs[target]['data'][:p['off']+3];fs.insert(target+1,copy.deepcopy(frames[target]))
        save(service+'-snaplen-repaired',fs,dict(truncated=target+1,repair='immediate complete TCP retransmission'))
        fs=[f for i,f in enumerate(frames) if i>=target or not ((q:=packet(f)) and q['tuple']==flow)]
        save(service+'-midstream',fs,dict(removed_flow_prefix_before=target+1,tuple=flow))
        # Swap consecutive same-direction data segments only; do not manufacture
        # a response preceding its request in an otherwise ordered conversation.
        for a,b in zip(indices,indices[1:]):
            pa,pb=packet(frames[a]),packet(frames[b])
            between=[packet(f) for f in frames[a+1:b] if packet(f) and packet(f)['tuple']==pa['tuple'] and packet(f)['payload']]
            if pa['sp']==pb['sp'] and pa['tuple']==pb['tuple'] and not between:
                fs=copy.deepcopy(frames);fs[a],fs[b]=fs[b],fs[a]
                save(service+'-reorder',fs,dict(swapped=[a+1,b+1]));break
def run(item):
    name,d=item
    try:
        p=subprocess.run([sys.executable,str(Path(__file__).with_name('compare.py')),str(d),str(tshark),str(yak),str(audit)],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
        (d/'compare.stdout').write_bytes(p.stdout);(d/'compare.stderr').write_bytes(p.stderr)
        assert p.returncode==0,p.stderr.decode(errors='replace')[:800]
        es=[json.loads(x) for x in (d/'comparison/events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
        seen=collections.Counter(signature(e) for e in es if e['status']=='decoded' and e['protocol']!='ftp-data')
        extra=seen-good
        assert not extra,('new decoded PDU',list(extra.items())[:6])
        for e in es:
            if e['protocol']=='ftp-data' and e['status']=='decoded':assert any(__import__('base64').b64decode(e['raw']) in data for data in ftp.values()),'invented FTP bytes'
        if name.endswith(('-duplicate','-snaplen-repaired')):assert seen==good,('lost messages despite retransmission',list((good-seen).items())[:6])
        return dict(case=name,frames=json.loads((d/'comparison/summary.json').read_text())['frames'],events=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),passed=True)
    except Exception as error:return dict(case=name,passed=False,error=str(error))
with concurrent.futures.ThreadPoolExecutor(max_workers=3) as pool:results=list(pool.map(run,variants))
(out/'fault-summary.json').write_text(json.dumps(results,indent=2),encoding='utf8')
print(json.dumps(dict(cases=len(results),failed=[x for x in results if not x['passed']]),indent=2))
assert all(x['passed'] for x in results),'see fault-summary.json'
