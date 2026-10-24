"""Full packet CSV plus semantic comparisons for fresh SSDP/TLS traffic.
Usage: compare.py PCAP OUTPUT_DIR TSHARK YAK EVENTS KEYLOG_OR_DASH ssdp_or_tls
"""
import base64,collections,csv,hashlib,io,json,subprocess,sys
from pathlib import Path
csv.field_size_limit(8000000)
cap,out,tshark,yak,audit=map(Path,sys.argv[1:6]);key=sys.argv[6];mode=sys.argv[7];out.mkdir(exist_ok=True)
def run(args,name):
    p=subprocess.run([str(x) for x in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
    (out/name).write_bytes(p.stdout);(out/(name+'.stderr')).write_bytes(p.stderr)
    assert p.returncode==0,(name,p.returncode,p.stderr.decode(errors='replace')[:1000])
    return p.stdout.decode('utf-8-sig')
fields=['frame.number','frame.protocols','_ws.col.Protocol','_ws.col.Info','tcp.stream','tls.handshake.ciphersuite','tls.record.length','http.request.method','http.request.uri','http.response.code','http.host','http.location','http.server','http.cache_control','http.file_data','_ws.expert.message']
args=[tshark,'-2','-n','-r',cap,'-o','tcp.reassemble_out_of_order:TRUE']
if key!='-':args+=['-o','tls.keylog_file:'+key]
if mode=='ssdp' and cap.stem=='loopback':
    port=subprocess.check_output([str(tshark),'-n','-r',str(cap),'-T','fields','-e','udp.dstport']).decode().splitlines()[0];args+=['-d',f'udp.port=={port},ssdp']
if mode=='tls':
    manifest=json.loads((cap.parent/'manifest.json').read_text());
    for port in manifest['ports']:args+=['-d',f'tcp.port=={port},tls']
args+=['-T','fields','-E','header=y','-E','occurrence=a']
for field in fields:args+=['-e',field]
ws=list(csv.DictReader(io.StringIO(run(args,'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE))
yargs=[yak,'shark','-r',cap,'--json'];eargs=[audit,'replay',cap]
if key!='-':yargs+=['--tls-keylog',key];eargs+=[key]
ys=[json.loads(x) for x in run(yargs,'yak.jsonl').splitlines()];es=[json.loads(x) for x in run(eargs,'events.jsonl').splitlines()]
assert len(ws)==len(ys)
byframe=collections.defaultdict(list)
for e in es:
    for r in e['source_bytes'].get('PacketRefs') or []:byframe[r['Number']].append(e)
with (out/'frame-comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:
    w=csv.writer(f);w.writerow(['frame','ws_protocol','yak_protocol','yak_connection_stack','ws_info','yak_info','events'])
    for a,b in zip(ws,ys):
        assert int(a['frame.number'])==b['number'];w.writerow([b['number'],a['_ws.col.protocol'],b['protocol'],' > '.join(b.get('connection_stack',[])),a['_ws.col.info'],b['info'],';'.join(e['protocol']+'/'+e['status'] for e in byframe[b['number']])])
summary=dict(frames=len(ws),native=dict(collections.Counter(e['protocol']+'/'+e['status'] for e in es)),ws_protocols=dict(collections.Counter(w['_ws.col.protocol'] for w in ws)),yak_protocols=dict(collections.Counter(y['protocol'] for y in ys)))
sha=lambda b:hashlib.sha256(b).hexdigest()
if mode=='ssdp':
    checked=0;warnings=[]
    for e in es:
        if e['protocol']!='ssdp' or e['status']!='decoded':continue
        refs=e['source_bytes']['PacketRefs'];assert len(refs)==1
        row=ws[refs[0]['Number']-1];assert row['_ws.col.protocol']=='SSDP'
        s=e['session']
        assert s['Method']==row['http.request.method']
        for key,field in [('Location','http.location'),('Server','http.server'),('HOST','http.host')]:
            if key in s:assert s[key]==row[field],(key,s[key],row[field])
        assert s['Identity Verified'] is False
        if s.get('Conformance Warnings'):warnings.append(dict(frame=refs[0]['Number'],warnings=s['Conformance Warnings']))
        checked+=1
    summary['semantic_packets_checked']=checked;summary['conformance_warnings']=warnings
    if cap.stem=='physical':assert checked==len(ws),'inspect each real device variant, never hide rejected packets'
    else:assert checked==5 and summary['native'].get('ssdp/malformed')==1
else:
    oracle=collections.Counter(o['body_sha256'] for o in manifest['operations']);native=[];ws_hashes=[]
    for e in es:
        if e['protocol']=='http' and e['status']=='decoded' and base64.b64decode(e['raw'] or '').startswith(b'HTTP/1.1 200 '):
            w=base64.b64decode(e['raw']);head,body=w.split(b'\r\n\r\n',1);assert e['source_bytes']['Kind']=='decrypted';native.append(sha(body))
    for w in ws:
        for data in w['http.file_data'].split(','):
            if data:ws_hashes.append(sha(bytes.fromhex(data.replace(':',''))))
    assert collections.Counter(native)==oracle,('native',collections.Counter(native),oracle)
    assert collections.Counter(ws_hashes)==oracle,('wireshark',collections.Counter(ws_hashes),oracle)
    assert not [e for e in es if e['status']!='decoded'],summary['native']
    summary['application_http_bodies_verified']=len(native);summary['wireshark_http_bodies_verified']=len(ws_hashes)
(out/'summary.json').write_text(json.dumps(summary,indent=2),encoding='utf8');print(json.dumps(summary,indent=2))
