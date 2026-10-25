"""Explicit offline IPv6 fragment faults from the real Wireshark PIM sample.

Usage: pim_fragment_faults.py SOURCE_CAP DIRECTORY TSHARK EVENTS
Requires the original complete little-endian loopback PCAP. Network traffic is
never replayed. Original PIM bytes and IPv6 endpoints stay unchanged; only the
declared fragment envelopes, snaplen and order differ. Outputs stay outside Git.
"""
import base64,hashlib,json,struct,subprocess,sys
from pathlib import Path

source,out,tshark,events=map(Path,sys.argv[1:5])
for p in [source,out]:
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):raise ValueError('Captures/output must remain outside Git')
def sha(w):return hashlib.sha256(w).hexdigest()
data=source.read_bytes()
if data[:4]!=bytes.fromhex('d4c3b2a1') or int.from_bytes(data[20:24],'little')!=0:raise ValueError('Expected original little-endian DLT_NULL capture')
records=[];at=24
while at<len(data):
    if at+16>len(data):raise ValueError('truncated PCAP record header')
    sec,usec,cap,wire=struct.unpack_from('<IIII',data,at);at+=16
    if cap>len(data)-at:raise ValueError('truncated PCAP packet')
    records.append({'frame':len(records)+1,'stamp':sec*1000000+usec,'wire':wire,'data':data[at:at+cap]});at+=cap
candidates=[r for r in records if len(r['data'])>=4+40+96 and r['wire']==len(r['data']) and r['data'][4]>>4==6 and r['data'][10]==103 and r['data'][44]==0x21 and not r['data'][48]&0x40]
if not candidates:raise ValueError('No complete non-null IPv6 Register in source')
selected=max(candidates,key=lambda r:len(r['data']));w=selected['data'];prefix=w[:4];ip=w[4:44];payload=w[44:]
if len(payload)!=int.from_bytes(ip[4:6],'big'):raise ValueError('Source IPv6 payload is not complete')
expected_sha=sha(payload)
def fragment(start,end,more):
    header=bytearray(ip);header[6]=44
    frag=bytes([103,0])+struct.pack('!HI',start|int(more),0x50494d31)+payload[start:end]
    header[4:6]=len(frag).to_bytes(2,'big')
    b=prefix+header+frag
    return {'data':b,'wire':len(b)}
parts=[fragment(0,16,True),fragment(16,64,True),fragment(64,len(payload),False)]
short={**parts[1],'data':parts[1]['data'][:-8]}
cases={
 'baseline':([{'data':w,'wire':len(w)}],1,'one unchanged original real Register'),
 'fragmented':(parts,1,'three IPv6 fragments with unchanged application bytes'),
 'reordered':([parts[2],parts[0],parts[1]],1,'last fragment arrives first'),
 'duplicate':([parts[0],parts[1],parts[1],parts[2]],1,'identical middle fragment duplicate'),
 'missing-middle':([parts[0],parts[2]],0,'missing middle fragment must leave a real hole'),
 'snaplen':([parts[0],short,parts[2]],0,'middle captured length shorter than original wire length'),
 'snaplen-repaired':([parts[0],short,parts[1],parts[2]],1,'complete retransmission repairs captured middle fragment'),
}
out.mkdir(parents=True,exist_ok=True)
manifest={'kind':'explicit offline faults from real Wireshark sample; no new live capture','source_sha256':sha(data),'source_frame':selected['frame'],'original_pim_sha256':expected_sha,'fragment_offsets':[0,16,64],'cases':{}}
for name,(rows,count,operation) in cases.items():
    b=bytearray(data[:24])
    for i,r in enumerate(rows):
        stamp=selected['stamp']+i
        b+=struct.pack('<IIII',stamp//1000000,stamp%1000000,len(r['data']),r['wire'])+r['data']
    cap=out/(name+'.cap');cap.write_bytes(b)
    manifest['cases'][name]={'capture_sha256':sha(b),'operation':operation,'frames':len(rows),'expected_complete_PIM_observations':count}
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
results={}
for name,case in manifest['cases'].items():
    d=out/name;d.mkdir(exist_ok=True);cap=out/(name+'.cap')
    for args,file in [([tshark,'-2','-n','-r',cap,'-Y','pim','-T','json','-x','--no-duplicate-keys'],'oracle.json'),([events,'replay',cap,'-','--ignore-embedded-secrets'],'events.jsonl')]:
        with (d/file).open('wb') as stdout,(d/(file+'.stderr')).open('wb') as stderr:subprocess.run(list(map(str,args)),stdout=stdout,stderr=stderr,timeout=120,check=True)
    oracle=json.loads((d/'oracle.json').read_text());es=[json.loads(l) for l in (d/'events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
    ps=[e for e in es if e['protocol']=='pim' and e['status']=='decoded'];want=case['expected_complete_PIM_observations']
    assert len(oracle)==len(ps)==want,(name,len(oracle),len(ps),want)
    assert all(sha(base64.b64decode(e['raw']))==expected_sha for e in ps),(name,'invented PIM bytes')
    comparison=None
    if want:
        comparison=d/'comparison.json'
        with (d/'audit.log').open('wb') as stdout,(d/'audit.stderr').open('wb') as stderr:subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('pim_control_audit.py')),str(d/'oracle.json'),str(d/'events.jsonl'),str(comparison)],stdout=stdout,stderr=stderr,check=True)
    results[name]={**case,'native_complete':len(ps),'oracle_complete':len(oracle),'native_status':dict(__import__('collections').Counter(e['protocol']+'/'+e['status'] for e in es)),
        'checks':json.loads(comparison.read_text())['checks_total'] if comparison else 0,'pdu_packet_refs':[[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []] for e in ps]}
    print(json.dumps({'case':name,'native_complete':len(ps),'oracle_complete':len(oracle)}),flush=True)
(out/'fault-summary.json').write_text(json.dumps({'manifest':manifest,'cases':results},indent=2)+'\n',encoding='utf8')
