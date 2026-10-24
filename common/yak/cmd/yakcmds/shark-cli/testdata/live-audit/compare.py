import base64,collections,csv,hashlib,io,json,os,struct,subprocess,sys
from pathlib import Path
csv.field_size_limit(8000000)
if len(sys.argv)<5:
    raise SystemExit('usage: compare.py OUTPUT_ROOT TSHARK YAK AUDIT [CAPTURE_DIRECTORY_NAME]')
root=Path(sys.argv[1]).resolve()
capture=root/(sys.argv[5] if len(sys.argv)>5 else 'capture-01')
manifest=json.loads((capture/'manifest.json').read_text(encoding='utf8'))
tshark,yak,audit=map(Path,sys.argv[2:5])
fields=['frame.number','frame.time_epoch','frame.cap_len','frame.len','frame.protocols','_ws.col.Protocol','_ws.col.Info',
        'tcp.stream','tcp.srcport','tcp.dstport','tcp.seq_raw','tcp.len','tcp.flags','tcp.analysis.lost_segment','tcp.analysis.out_of_order','tcp.analysis.retransmission',
        'tls.record.length','tls.record.version','tls.handshake.type','tls.handshake.ciphersuite','tls.handshake.extensions.supported_version',
        'http.request.method','http.request.uri','http.response.code','http2.type','http2.streamid','http2.headers.method','http2.headers.path','http2.headers.status',
        'grpc.message_length','grpc.message_data','udp.srcport','quic.connection.number','quic.packet_number','quic.dcid','quic.scid','quic.stream.stream_id','quic.stream.offset',
        'http3.frame_type','http3.headers.method','http3.headers.path','http3.headers.status','http3.data','dns.id','dns.qry.name','_ws.expert.message']
def run(args,path):
    r=subprocess.run([str(a) for a in args],stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=120)
    path.write_bytes(r.stdout);path.with_suffix(path.suffix+'.stderr').write_bytes(r.stderr)
    if r.returncode:raise RuntimeError((path,r.returncode,r.stderr.decode(errors='replace')[:1000]))
    return r.stdout.decode('utf-8-sig')
available={s.split('\t')[2] for s in subprocess.check_output([str(tshark),'-G','fields']).decode(errors='replace').splitlines() if s.startswith('F\t') and len(s.split('\t'))>2}
missing=set(fields)-available-{'_ws.col.Info','_ws.col.Protocol'}
assert not missing,missing
def tshark_args(cap):
    args=[tshark,'-2','-n','-r',cap,'-o','tls.keylog_file:'+str(capture/'session.keys'),'-o','tcp.desegment_tcp_streams:TRUE','-o','tcp.reassemble_out_of_order:TRUE','-o','tls.desegment_ssl_records:TRUE','-o','tls.desegment_ssl_application_data:TRUE']
    for key,p in manifest['Ports'].items():
        proto=None;carrier='tcp'
        if key.startswith('tls') or key=='grpc':proto='tls'
        if key=='http':proto='http'
        if key=='dns_tcp':proto='dns'
        if key=='dns_udp':proto='dns';carrier='udp'
        if key=='quic_client':proto='quic';carrier='udp'
        if proto:args+=['-d',f'{carrier}.port=={p},{proto}']
    args+=['-T','fields','-E','header=y','-E','occurrence=a']
    for f in fields:args+=['-e',f]
    return args
def tokens(s):return [v for v in s.split(',') if v]
def analyze(cap,out):
    out.mkdir(exist_ok=True)
    ws=list(csv.DictReader(io.StringIO(run(tshark_args(cap),out/'tshark.tsv')),delimiter='\t',quoting=csv.QUOTE_NONE))
    ys=[json.loads(s) for s in run([yak,'shark','-r',cap,'--json','--tls-keylog',capture/'session.keys'],out/'yak.jsonl').splitlines()]
    events=[json.loads(s) for s in run([audit,'replay',cap,capture/'session.keys'],out/'events.jsonl').splitlines()]
    grpc=[m for e in events for m in (e.get('session') or {}).get('GRPC Messages',[])]
    grpc_y=sorted(hashlib.sha256(base64.b64decode(m['Payload'])).hexdigest() for m in grpc)
    grpc_w=sorted(hashlib.sha256(bytes.fromhex(p.replace(':',''))).hexdigest() for w in ws for p in tokens(w['grpc.message_data']))
    h3_y=sorted(h['Value'] for e in events if e['protocol']=='http3' and e['status'] in ('decoded','deferred') for h in (e.get('session') or {}).get('Headers',[]) if h['Name']==':path')
    h3_seen=set();h3_w=[]
    for w in ws:
        for p in tokens(w['http3.headers.path']):
            key=(w['quic.connection.number'],w['udp.srcport'],w['quic.stream.stream_id'],p)
            if key not in h3_seen:h3_seen.add(key);h3_w.append(p)
    h3_w.sort()
    h3_data_y=sorted(hashlib.sha256(base64.b64decode(h['Payload'])).hexdigest() for e in events for fr in (e.get('session') or {}).get('Frames',[]) for h in fr.get('HTTP3 Frames',[]) if h.get('Frame Type')=='DATA' and 'Payload' in h)
    # Wireshark displays DATA again on a retransmitted QUIC STREAM range. Count
    # logical frame delivery once, preserving connection, direction and offset.
    seen=set();h3_data_w=[];h3_retransmitted=0
    for w in ws:
        for p in tokens(w['http3.data']):
            key=(w['quic.connection.number'],w['udp.srcport'],w['quic.stream.stream_id'],w['quic.stream.offset'] or '0',p)
            if key in seen:h3_retransmitted+=1;continue
            seen.add(key);h3_data_w.append(hashlib.sha256(bytes.fromhex(p.replace(':',''))).hexdigest())
    h3_data_w.sort()
    h3_oracle=collections.Counter(o['ValueHash'] for o in manifest['Operations'] if o.get('Protocol')=='http3')
    http_y=[e['summary'] for e in events if e['protocol']=='http' and e['status']=='decoded']
    errors=[{'id':e['id'],'protocol':e['protocol'],'status':e['status'],'expert':e.get('expert'),'error':e.get('error'),'frames':[r['Number'] for r in e['source_bytes'].get('PacketRefs') or []]} for e in events if e['status'] not in ('decoded','deferred')]
    summary={'frames':[len(ws),len(ys)],'rows_yak':dict(collections.Counter(r['protocol'] for r in ys)),'rows_ws':dict(collections.Counter(r['_ws.col.protocol'] for r in ws)),
             'events':dict(collections.Counter(e['protocol']+'/'+e['status'] for e in events)),
             'grpc':[len(grpc_w),len(grpc_y)],'grpc_bytes_equal':grpc_w==grpc_y,'grpc_ws_hashes':grpc_w,'grpc_yak_hashes':grpc_y,
             'h3_paths_equal':h3_w==h3_y,'h3_ws_paths':h3_w,'h3_yak_paths':h3_y,
             'h3_data':[len(h3_data_w),len(h3_data_y)],'h3_data_equal':h3_data_w==h3_data_y,'h3_ws_hashes':h3_data_w,'h3_yak_hashes':h3_data_y,
             'h3_wireshark_retransmitted_data_observations':h3_retransmitted,
             'h3_yak_application_replies_verified':bool(h3_oracle) and not h3_oracle-collections.Counter(h3_data_y),
             'h3_ws_application_replies_verified':bool(h3_oracle) and not h3_oracle-collections.Counter(h3_data_w),
             'tls_records':[sum(len(tokens(w['tls.record.length'])) for w in ws),sum(e['protocol']=='tls' for e in events)],
             'http_summaries':http_y,'errors':errors,
             'authenticated_plaintext_without_verified_tag':[e['id'] for e in events if (e.get('session') or {}).get('Content Visibility')=='authenticated-plaintext' and (e.get('session') or {}).get('Authentication Verified') is not True]}
    # One comparison row for every captured frame. Event contributor references
    # remain explicit: reassembled messages may legitimately span several rows.
    by_frame=collections.defaultdict(list)
    for e in events:
        for ref in e['source_bytes'].get('PacketRefs') or []:
            by_frame[ref['Number']].append(e)
    mapping=list(range(1,len(ws)+1))
    if (out/'provenance.json').exists():mapping=json.loads((out/'provenance.json').read_text())['original_frames']
    with (out/'frame-comparison.csv').open('w',encoding='utf-8-sig',newline='') as f:
        writer=csv.writer(f)
        writer.writerow(['frame','original_frame','wireshark_protocol','yak_packet_protocol','yak_connection_stack','wireshark_info','yak_info','contributing_events','diagnostics'])
        for i,(w,y) in enumerate(zip(ws,ys)):
            ev=by_frame[i+1]
            writer.writerow([i+1,mapping[i],w['_ws.col.protocol'],y['protocol'],' > '.join(y.get('connection_stack',[])),w['_ws.col.info'],y['info'],'; '.join(f"{e['id']}:{e['protocol']}/{e['status']}" for e in ev),'; '.join(e.get('error','') for e in ev if e.get('error'))])
    (out/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf8')
    return summary,ws,ys,events

if __name__=='__main__':
    s,_,_,_=analyze(capture/'live.pcap',root/'baseline')
    print(json.dumps({k:v for k,v in s.items() if 'hashes' not in k and k not in ('http_summaries',)},ensure_ascii=False,indent=2))
