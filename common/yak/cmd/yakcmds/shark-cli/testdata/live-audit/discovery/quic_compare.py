"""Compare every real aioquic capture frame, logical HTTP/3 DATA, and PN evidence.
Usage: quic_compare.py CAPTURE_DIR OUTPUT_DIR TSHARK YAK EVENTS
"""
import base64,collections,csv,hashlib,io,json,subprocess,sys
from pathlib import Path
csv.field_size_limit(8000000)
cap,out,tshark,yak,audit=map(Path,sys.argv[1:6]);out.mkdir(exist_ok=True)
manifest=json.loads((cap/'manifest.json').read_text());keys=cap/'session.keys'
def run(args,name):
    p=subprocess.run([str(a) for a in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
    (out/name).write_bytes(p.stdout);(out/(name+'.stderr')).write_bytes(p.stderr)
    assert p.returncode==0,(name,p.stderr.decode(errors='replace')[:1000])
    return p.stdout.decode('utf-8-sig')
fields=['frame.number','frame.protocols','_ws.col.Protocol','_ws.col.Info','udp.srcport','udp.dstport','quic.connection.number','quic.packet_number','quic.dcid','quic.key_phase','quic.stream.stream_id','quic.stream.offset','http3.headers.path','http3.data','_ws.expert.message']
args=[tshark,'-2','-n','-r',cap/'live.pcap','-o','tls.keylog_file:'+str(keys),'-d',f"udp.port=={manifest['server_port']},quic",'-T','fields','-E','header=y','-E','occurrence=a']
for field in fields:args+=['-e',field]
ws=list(csv.DictReader(io.StringIO(run(args,'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE))
ys=[json.loads(s) for s in run([yak,'shark','-r',cap/'live.pcap','--json','--tls-keylog',keys],'yak.jsonl').splitlines()]
es=[json.loads(s) for s in run([audit,'replay',cap/'live.pcap',keys],'events.jsonl').splitlines()]
tokens=lambda s:[p for p in s.split(',') if p]
sha=lambda b:hashlib.sha256(b).hexdigest()
oracle=collections.Counter(o[key] for o in manifest['operations'] for key in ['request_sha256','response_sha256'])
native=collections.Counter(sha(base64.b64decode(h['Payload'])) for e in es if e['protocol']=='http3' for f in e['session'].get('Frames',[]) for h in f.get('HTTP3 Frames',[]) if h['Frame Type']=='DATA' and 'Payload' in h)
ws_data=collections.Counter();seen=set()
for w in ws:
    for data in tokens(w['http3.data']):
        key=(w['quic.connection.number'],w['quic.stream.stream_id'],w['quic.stream.offset'],data)
        if key in seen:continue
        seen.add(key);ws_data[sha(bytes.fromhex(data.replace(':','')))]+=1
assert native==oracle,('Yak DATA',native-oracle,oracle-native)
assert not ws_data-oracle,('Wireshark invented DATA',ws_data-oracle)
# Wireshark's HTTP/3 PDU reassembly can omit a complete DATA frame after
# reordered retransmission. Independently assemble its authenticated QUIC
# STREAM fields as a second comparison; keep this distinction in the report.
json_args=args[:args.index('-T')]+['-T','json','--no-duplicate-keys']
wire_rows=json.loads(run(json_args,'tshark-streams.json'))
streams={}
for row in wire_rows:
    layers=row['_source']['layers'];packets=layers.get('quic',[])
    if not isinstance(packets,list):packets=[packets]
    direction=int(layers['udp']['udp.srcport']==str(manifest['server_port']))
    for packet in packets:
        connection=packet.get('QUIC Connection information',{}).get('quic.connection.number')
        frames=packet.get('quic.frame',[])
        if not isinstance(frames,list):frames=[frames]
        for frame in frames:
            if 'quic.stream_data' not in frame:continue
            sid=int(frame['quic.stream.stream_id'])
            if sid%4:continue
            off=int(frame.get('quic.stream.offset','0'));data=bytes.fromhex(frame['quic.stream_data'].replace(':',''));end=off+len(data)
            assert end<4000000
            buf,present=streams.setdefault((connection,direction,sid),(bytearray(),bytearray()))
            if end>len(buf):buf.extend(bytes(end-len(buf)));present.extend(bytes(end-len(present)))
            for i,b in enumerate(data,off):
                assert not present[i] or buf[i]==b,'conflicting Wireshark stream bytes'
                buf[i]=b;present[i]=1
def vint(w,at):
    n=1<<(w[at]>>6);assert at+n<=len(w)
    return int.from_bytes(w[at:at+n],'big')&((1<<(8*n-2))-1),at+n
ws_stream_data=collections.Counter()
for key,(buf,present) in streams.items():
    assert all(present),(key,'unfilled stream gap')
    at=0
    while at<len(buf):
        typ,at=vint(buf,at);n,at=vint(buf,at);assert at+n<=len(buf)
        if typ==0:ws_stream_data[sha(buf[at:at+n])]+=1
        at+=n
assert ws_stream_data==oracle,('Wireshark QUIC STREAM DATA',ws_stream_data-oracle,oracle-ws_stream_data)
paths=sorted(o['path'] for o in manifest['operations'])
native_paths=sorted(h['Value'] for e in es if e['protocol']=='http3' for h in e['session'].get('Headers',[]) if h['Name']==':path')
assert native_paths==paths
assert sorted({p for w in ws for p in tokens(w['http3.headers.path'])})==paths
assert len(ws)==len(ys)
by_frame=collections.defaultdict(list);pn_checked=0
for e in es:
    for ref in e['source_bytes'].get('PacketRefs') or []:by_frame[ref['Number']].append(e)
    s=e.get('session') or {}
    if s.get('Authentication Verified') and 'Packet Number' in s:
        ref=e['source_bytes']['PacketRefs'];assert len(ref)==1
        w=ws[ref[0]['Number']-1]
        assert str(s['Packet Number']) in tokens(w['quic.packet_number']),(e['id'],s['Packet Number'],w)
        pn_checked+=1
bad=[e for e in es if e['status']!='decoded']
assert len(bad)==manifest['network']['corrupted']
assert all(e['protocol']=='quic' and e.get('expert')=='AuthenticationFailed' and (e.get('session') or {}).get('Authentication Verified') is False for e in bad),[(e['protocol'],e.get('expert')) for e in bad]
flows={e['flow'] for e in es if e['flow']};assert len(flows)==3
for flow in flows:
    events=[e for e in es if e['flow']==flow and e['status']=='decoded']
    assert len({e['source'] for e in events if e['direction']==0})==2,'NAT rebinding must retain the same connection'
    for direction in (0,1):
        ordered=sorted([(e['session']['Packet Number'],e['session']['Key Phase']) for e in events if e['direction']==direction and 'Key Phase' in e['session']])
        phases=[]
        for _,p in ordered:
            if not phases or phases[-1]!=p:phases.append(p)
        assert phases==[0,1,0],(flow,direction,phases)
with (out/'frame-comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:
    writer=csv.writer(f);writer.writerow(['frame','ws_protocol','yak_protocol','yak_connection_stack','ws_info','yak_info','events'])
    for w,y in zip(ws,ys):
        assert int(w['frame.number'])==y['number']
        writer.writerow([y['number'],w['_ws.col.protocol'],y['protocol'],' > '.join(y.get('connection_stack',[])),w['_ws.col.info'],y['info'],';'.join(f"{e['flow']}:{e['protocol']}/{e['status']}" for e in by_frame[y['number']])])
summary=dict(frames=len(ws),network=manifest['network'],operations=len(manifest['operations']),data_bodies_verified=sum(native.values()),wireshark_http3_pdu_bodies=sum(ws_data.values()),wireshark_stream_bodies_verified=sum(ws_stream_data.values()),wireshark_http3_reassembly_omitted_hashes=list((oracle-ws_data).elements()),packet_numbers_verified=pn_checked,connections=len(flows),authenticated_retired_cid_events=sum((e.get('session') or {}).get('Retired Destination CID') is True for e in es),events=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),yak_protocols=dict(collections.Counter(y['protocol'] for y in ys)),wireshark_protocols=dict(collections.Counter(w['_ws.col.protocol'] for w in ws)),pcap_sha256=sha((cap/'live.pcap').read_bytes()))
(out/'summary.json').write_text(json.dumps(summary,indent=2),encoding='utf8');print(json.dumps(summary,indent=2))
