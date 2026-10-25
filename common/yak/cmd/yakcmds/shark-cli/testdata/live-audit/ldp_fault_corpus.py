"""Explicit offline LDP faults from a real, single-interface TCP export.

Usage: ldp_fault_corpus.py SOURCE_PCAPNG OUTPUT_DIRECTORY
Requires Ethernet/IPv4 TCP and an observed SYN. Application bytes stay original;
split/reuse controls recompute changed headers/checksums. No network replay.
"""
import hashlib,json,struct,sys
from pathlib import Path

source,out=map(Path,sys.argv[1:3])
for p in (source,out):
    if any((x/'.git').exists() for x in [p.resolve(),*p.resolve().parents]):raise SystemExit('Capture/output must remain outside Git')
raw=source.read_bytes();records=[];pos=0;endian=None;interfaces=[]
while pos<len(raw):
    if raw[pos:pos+4]==bytes.fromhex('0a0d0d0a'):
        magic=raw[pos+8:pos+12];endian='<' if magic==bytes.fromhex('4d3c2b1a') else '>' if magic==bytes.fromhex('1a2b3c4d') else None
        if not endian:raise ValueError('bad section byte order')
        interfaces=[]
    if endian is None or len(raw)-pos<12:raise ValueError('missing/truncated PCAPNG block')
    typ,n=struct.unpack_from(endian+'II',raw,pos)
    if n<12 or n%4 or pos+n>len(raw) or struct.unpack_from(endian+'I',raw,pos+n-4)[0]!=n:raise ValueError('bad block bounds')
    block=raw[pos+8:pos+n-4]
    if typ==1:
        link=struct.unpack_from(endian+'H',block)[0];resolution=1000000;offset=0;at=8
        while at+4<=len(block):
            code,size=struct.unpack_from(endian+'HH',block,at);at+=4
            if code==0:break
            if at+size>len(block):raise ValueError('bad interface option')
            if code==9 and size==1:resolution=2**(block[at]&127) if block[at]&128 else 10**block[at]
            if code==14 and size==8:offset=struct.unpack_from(endian+'q',block,at)[0]
            at+=(size+3)&~3
        interfaces.append((link,resolution,offset))
    elif typ==6:
        iface,hi,lo,cap,wire=struct.unpack_from(endian+'IIIII',block)
        if iface>=len(interfaces) or cap>len(block)-20:raise ValueError('bad enhanced packet')
        link,resolution,offset=interfaces[iface]
        if link!=1:raise ValueError('fault source must contain only Ethernet packets')
        stamp=(((hi<<32)|lo)*1000000000)//resolution+offset*1000000000
        records.append({'data':block[20:20+cap],'wire':wire,'stamp':stamp,'original':len(records)+1})
    elif typ in (2,3):raise ValueError('unsupported packet block; do not silently drop packets')
    pos+=n

def info(row):
    w=row['data']
    if len(w)<54 or w[12:14]!=b'\x08\x00' or w[14]>>4!=4 or w[23]!=6:raise ValueError('expected original IPv4 TCP')
    if int.from_bytes(w[20:22],'big')&0x3fff:raise ValueError('fault source must be unfragmented IPv4')
    ip=4*(w[14]&15);off=14+ip;tcp=4*(w[off+12]>>4);total=14+int.from_bytes(w[16:18],'big')
    if ip<20 or tcp<20 or off+tcp>total or total>len(w):raise ValueError('invalid or truncated source TCP')
    ports=struct.unpack_from('!HH',w,off)
    a=(w[26:30],ports[0]);b=(w[30:34],ports[1])
    return {'off':off,'tcp':tcp,'total':total,'payload':w[off+tcp:total],'tuple':tuple(sorted([a,b])),'flags':w[off+13]}
def checksum(w):
    if len(w)&1:w+=b'\0'
    n=sum(struct.unpack('!'+'H'*(len(w)//2),w))
    while n>>16:n=(n&65535)+(n>>16)
    return (~n)&65535
def rewrite(row,payload=None,seqadd=0,shift=False,reset=False):
    p=info(row);off,tcp=p['off'],p['tcp'];w=bytearray(row['data'][:off+tcp])
    if payload is None:payload=p['payload']
    if reset:payload=b'';w[off+13]=0x14;seqadd+=len(p['payload'])
    w+=payload
    seq,ack=struct.unpack_from('!II',w,off+4)
    struct.pack_into('!I',w,off+4,(seq+seqadd)&0xffffffff)
    if shift and w[off+13]&16:struct.pack_into('!I',w,off+8,(ack+seqadd)&0xffffffff)
    struct.pack_into('!H',w,16,len(w)-14)
    w[24:26]=b'\0\0';struct.pack_into('!H',w,24,checksum(bytes(w[14:off])))
    w[off+16:off+18]=b'\0\0';body=bytes(w[off:]);pseudo=bytes(w[26:34])+bytes([0,6])+struct.pack('!H',len(body))
    struct.pack_into('!H',w,off+16,checksum(pseudo+body))
    return {**row,'data':bytes(w),'wire':len(w)}

# Select one actually observed new TCP generation, keeping its complete peer
# exchange. A caller may not pass an unrelated packet collection as a fixture.
syn=next((i for i,r in enumerate(records) if info(r)['flags']&0x12==2),None)
if syn is None:raise ValueError('no captured SYN; cannot construct generation controls')
key=info(records[syn])['tuple']
baseline=[r for r in records[syn:] if info(r)['tuple']==key]
if 646 not in [v[1] for v in key]:raise ValueError('not the LDP service tuple')
target=max(range(len(baseline)),key=lambda i:len(info(baseline[i])['payload']))
payload=info(baseline[target])['payload']
if len(payload)<100 or payload[:2]!=b'\0\1' or int.from_bytes(payload[2:4],'big')+4!=len(payload):raise ValueError('expected one complete large original LDP PDU')
parts=[rewrite(baseline[target],payload[:10]),rewrite(baseline[target],payload[10:70],seqadd=10),rewrite(baseline[target],payload[70:],seqadd=70)]
before,after=baseline[:target],baseline[target+1:]
truncated={**baseline[target],'data':baseline[target]['data'][:info(baseline[target])['off']+info(baseline[target])['tcp']+17]}
reset=rewrite(baseline[-1],reset=True)
reset['stamp']=baseline[-1]['stamp']+1000
second=[rewrite(r,seqadd=1000000,shift=True) for r in baseline]
second_epoch=reset['stamp']+1000000-baseline[0]['stamp']
second=[{**r,'stamp':r['stamp']+second_epoch} for r in second]
cases={
 'baseline':(baseline,'single observed original TCP generation; no application-byte changes'),
 'split-reordered':(before+[parts[0],parts[2],parts[1]]+after,'split largest original PDU, deliver last sequence range before middle'),
 'split-duplicate':(before+[parts[0],parts[1],parts[2],parts[1]]+after,'duplicate unchanged middle TCP sequence range'),
 'split-loss':(before+[parts[0],parts[2]]+after,'drop middle range; preserve the missing sequence gap'),
 'snaplen':(before+[truncated]+after,'capture only 17 payload bytes; retain original wire length'),
 'snaplen-retransmitted':(before+[truncated,baseline[target]]+after,'original full TCP retransmission repairs captured truncation'),
 'same-tuple-new-generation':(baseline+[reset]+second,'constructed observed RST, same tuple and application bytes/IDs with changed TCP ISNs'),
}
out.mkdir(parents=True,exist_ok=True)
manifest={'source_sha256':hashlib.sha256(raw).hexdigest(),'kind':'offline fault controls derived from original real LDP packets; not new live traffic','source_frames':len(records),'selected_connection_frames':len(baseline),'target_source_frame':baseline[target]['original'],'clock_policy':'source clock plus serialization nanoseconds; constructed RST 1 us after original connection, copied generation begins 1 ms after RST','cases':[]}
header=bytes.fromhex('4d3cb2a1')+struct.pack('<HHIIII',2,4,0,0,65535,1)
for name,(rs,why) in cases.items():
    data=bytearray(header)
    for i,r in enumerate(rs):
        # Repeated/split frames get a nanosecond serialization tick. The copied
        # generation has its explicitly derived, later epoch (no time rewind).
        stamp=r['stamp']+i
        data+=struct.pack('<IIII',stamp//1000000000,stamp%1000000000,len(r['data']),r['wire'])+r['data']
    path=out/(name+'.pcap');path.write_bytes(data)
    manifest['cases'].append({'name':name,'file':path.name,'sha256':hashlib.sha256(data).hexdigest(),'frames':len(rs),'operation':why,'source_frames':[r['original'] for r in rs]})
(out/'manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
print(json.dumps({'source_frames':len(records),'connection_frames':len(baseline),'derived_cases':len(cases),'split_payload_bytes':len(payload)}))
