"""Explicit offline IPv4 faults from original real L2TPv3 IP115 bytes.

Usage: l2tp3_fragment_faults.py ORIGINAL_ORACLE_JSON DIRECTORY TSHARK EVENTS
No packet is sent on a network. Original carrier peers and complete L2TP bytes
are retained. Fragment envelopes/order/snaplen are the declared mutations.
"""
import base64,collections,hashlib,ipaddress,json,struct,subprocess,sys
from pathlib import Path

oracle,out,tshark,events=map(Path,sys.argv[1:5])
for p in [oracle,out]:
    if any((a/'.git').exists() for a in [p.resolve(),*p.resolve().parents]):raise ValueError('Raw evidence remains outside Git')
def sha(b):return hashlib.sha256(b).hexdigest()
def desc(x):return ([x] if isinstance(x[0],str) else x) if x else []
def vals(x,k):
    if isinstance(x,dict):
        if k in x:yield from x[k] if isinstance(x[k],list) else [x[k]]
        for key,v in x.items():
            if not key.endswith('_raw'):yield from vals(v,k)
    elif isinstance(x,list):
        for v in x:yield from vals(v,k)
def checksum(b):
    if len(b)%2:b+=b'\0'
    v=sum(int.from_bytes(b[i:i+2],'big') for i in range(0,len(b),2))
    while v>>16:v=(v&65535)+(v>>16)
    return (~v)&65535

selected=None
for p in json.loads(oracle.read_text(encoding='utf-8-sig')):
    ls=p['_source']['layers']
    for ir in desc(ls.get('ip_raw')):
        ip=bytes.fromhex(ir[0]);at=int(ir[1])
        if len(ip)!=20 or ip[0]!=0x45 or ip[9]!=115 or int.from_bytes(ip[6:8],'big')&0x3fff:continue
        w=bytes.fromhex(ls['frame_raw'][0]);end=at+int.from_bytes(ip[2:4],'big')
        body=w[at+20:end]
        if end<=len(w) and len(body)>=80 and int.from_bytes(body[:4],'big')!=0:
            selected=(ls,w,at,ip,body);break
    if selected:break
if selected is None:raise ValueError('Original complete real IP115 data frame is required')
ls,original,at,ip,body=selected;prefix=original[:at];sid=int.from_bytes(body[:4],'big')
source_peer,destination_peer=str(ipaddress.ip_address(ip[12:16])),str(ipaddress.ip_address(ip[16:20]))
def frag(start,end,more):
    h=bytearray(ip);h[2:4]=(20+end-start).to_bytes(2,'big');h[6:8]=((start//8)|int(more)*0x2000).to_bytes(2,'big');h[10:12]=b'\0\0';h[10:12]=checksum(bytes(h)).to_bytes(2,'big')
    w=prefix+h+body[start:end]
    return {'data':w,'length':len(w)}
parts=[frag(0,16,True),frag(16,48,True),frag(48,len(body),False)]
short={**parts[1],'data':parts[1]['data'][:-8]}
cases={
 'baseline':([{'data':original,'length':len(original)}],1,'unchanged original real router frame'),
 'fragmented':(parts,1,'three IPv4 fragments with original logical bytes'),
 'reordered':([parts[2],parts[0],parts[1]],1,'last fragment first'),
 'duplicate':([parts[0],parts[1],parts[1],parts[2]],1,'identical middle fragment twice'),
 'missing-middle':([parts[0],parts[2]],0,'unrepaired hole must not create a PDU'),
 'snaplen':([parts[0],short,parts[2]],0,'snaplen cut must not invent missing bytes'),
 'snaplen-repaired':([parts[0],short,parts[1],parts[2]],1,'complete retransmission repairs the cut fragment'),
}
out.mkdir(parents=True,exist_ok=True)
manifest={'scope':'explicit offline IP fragment mutations, not new live traffic or L2TP session delivery','oracle_sha256':sha(oracle.read_bytes()),'original_frame':int(ls['frame']['frame.number']),'original_wire_sha256':sha(original),'logical_l2tp_sha256':sha(body),'session_id':sid,'source':source_peer,'destination':destination_peer,'cases':{}}
results={}
for name,(rows,want,operation) in cases.items():
    b=bytearray(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1))
    for i,r in enumerate(rows):b+=struct.pack('<IIII',1700000000,i*1000,len(r['data']),r['length'])+r['data']
    cap=out/(name+'.pcap');cap.write_bytes(b);d=out/name;d.mkdir(exist_ok=True)
    manifest['cases'][name]={'capture_sha256':sha(b),'operation':operation,'frames':len(rows),'expected_complete_envelopes':want}
    for args,file in [([tshark,'-2','-n','-r',cap,'-o','ip.defragment:TRUE','-Y','l2tp','-T','json','-x','--no-duplicate-keys'],'oracle.json'),([events,'replay',cap,'-','--ignore-embedded-secrets'],'events.jsonl')]:
        with (d/file).open('wb') as stdout,(d/(file+'.stderr')).open('wb') as stderr:
            subprocess.run(list(map(str,args)),stdout=stdout,stderr=stderr,timeout=120,check=True)
    ws=json.loads((d/'oracle.json').read_text());es=[json.loads(l) for l in (d/'events.jsonl').read_text(encoding='utf-8-sig').splitlines()]
    ps=[e for e in es if e['protocol']=='l2tp' and e['status']=='decoded']
    ws_sids=[int(x,0) for p in ws for x in vals(p['_source']['layers'].get('l2tp'),'l2tp.sid')]
    assert len(ps)==len(ws_sids)==want,(name,len(ps),len(ws_sids),want)
    checks=0
    for e in ps:
        assert sha(base64.b64decode(e['raw']))==sha(body),(name,'invented logical PDU bytes');checks+=1
        assert e['session']['Session ID']==sid and ws_sids==[sid],(name,'session ID mismatch');checks+=1
        assert e['session']['Payload Decoded'] is False and e['flow']==0,(name,'invented pseudowire/session');checks+=1
        assert e['source']==source_peer and e['destination']==destination_peer,(name,'changed carrier endpoints');checks+=1
    results[name]={**manifest['cases'][name],'native_complete':len(ps),'wireshark_complete':len(ws_sids),'checks':checks,'native_status':dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),'packet_refs':[[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []] for e in ps]}
    print(json.dumps({'case':name,'native_complete':len(ps),'wireshark_complete':len(ws_sids)}),flush=True)
(out/'fault-summary.json').write_text(json.dumps({'manifest':manifest,'cases':results},indent=2)+'\n',encoding='utf8')
