"""Review every original 94/39 field difference without normalizing it away.

Inputs, DNS oracle JSON and detailed proofs must stay outside Git. The final
review JSON contains only original frame numbers, lengths, hashes and reasons.
TLS byte identities are constrained by the native direction/flow/domain and
the actual contributing packets; three delayed Wireshark views additionally
require a spanning sibling record, not merely another equal-length record.
"""
import argparse, base64, collections, hashlib, json, subprocess
from pathlib import Path

p=argparse.ArgumentParser(description=__doc__)
p.add_argument("--baseline",type=Path,required=True)
p.add_argument("--weber-events",type=Path,required=True)
p.add_argument("--sharon-events",type=Path,required=True)
p.add_argument("--weber-capture",type=Path,required=True)
p.add_argument("--sharon-capture",type=Path,required=True)
p.add_argument("--tshark",type=Path,required=True)
p.add_argument("--out",type=Path,required=True)
a=p.parse_args()
for path in [a.baseline,a.weber_events,a.sharon_events,a.weber_capture,a.sharon_capture,a.out]:
    if any((d/".git").exists() for d in [path.resolve(),*path.resolve().parents]):raise SystemExit("Raw audit data must stay outside Git")
a.out.mkdir(parents=True,exist_ok=True)
sha=lambda w:hashlib.sha256(w).hexdigest()
def read(path):return json.loads(path.read_text(encoding="utf-8-sig"))
def digest(path):
    h=hashlib.sha256()
    with path.open('rb') as f:
        for b in iter(lambda:f.read(1<<20),b''):h.update(b)
    return h.hexdigest()
def refs(e):return [r["Number"] for r in e["source_bytes"].get("PacketRefs") or []]
def scope(e):return (e["flow"],e["direction"],json.dumps(e["domain"],sort_keys=True))
def raw_rows(v):return [v] if v and isinstance(v[0],str) else v or []
def tls_records(v):
    if isinstance(v,dict):
        for k,x in v.items():
            if k=="tls.record_raw":
                for r in raw_rows(x):
                    w=bytes.fromhex(r[0])
                    if len(w)>=5 and int.from_bytes(w[3:5],'big')==len(w)-5:yield w
            elif not k.endswith('_raw'):yield from tls_records(x)
    elif isinstance(v,list):
        for x in v:yield from tls_records(x)
def expanded_name(w,at):
    out=bytearray();seen=set();length=1
    while True:
        if at in seen or not 0<=at<len(w):raise ValueError('DNS name pointer loop/boundary')
        seen.add(at);n=w[at];at+=1
        if n==0:return bytes(out+b'\0')
        if n&0xc0==0xc0:
            if at>=len(w):raise ValueError('truncated DNS pointer')
            at=((n&63)<<8)|w[at];continue
        if n>63 or at+n>len(w):raise ValueError('DNS label boundary')
        length+=1+n
        if length>255:raise ValueError('DNS expanded name over 255 octets')
        out.append(n);out.extend(w[at:at+n]);at+=n

oracle=read(a.baseline/'tls-final'/'tls-records-oracle.json')
ws_by_frame=collections.defaultdict(list);ws_by_hash=collections.defaultdict(set)
for packet in oracle:
    layers=packet['_source']['layers'];frame=int(layers['frame']['frame.number'])
    for w in tls_records(layers):
        item={'sha256':sha(w),'length':len(w)-5};ws_by_frame[frame].append(item);ws_by_hash[item['sha256']].add(frame)
del oracle
derived_frames=collections.defaultdict(set)
provenance={v['derived_frame']:v['original_frame'] for v in read(a.baseline/'rdp-final'/'derived-provenance.json')}
derived_summary=read(a.baseline/'rdp-final'/'summary.json')
assert not derived_summary['missing'] and not derived_summary['extra']
assert digest(a.baseline/'rdp-final'/'DERIVED-rdpudp-ordered-tls.pcap')==derived_summary['derived_capture_sha256']
for packet in read(a.baseline/'rdp-final'/'derived-tls.json'):
    l=packet['_source']['layers'];original=provenance[int(l['frame']['frame.number'])]
    for w in tls_records(l):derived_frames[sha(w)].add(original)
reviews={}
for name,path,capture in [('weber',a.weber_events,a.weber_capture),('sharon',a.sharon_events,a.sharon_capture)]:
    baseline=read(a.baseline/(name+'-final')/'field-mismatches.json')
    wanted={d['frame'] for d in baseline};events=[];tls=[];frame_events=collections.defaultdict(list);frame_scopes=collections.defaultdict(set);by_hash=collections.defaultdict(list)
    for line in path.open(encoding='utf-8-sig'):
        if not line.startswith('{'):continue
        e=json.loads(line)
        if e['protocol']=='tls' and e['status']=='decoded':
            w=base64.b64decode(e['raw']);e['_hash']=sha(w);e['_length']=len(w)-5
            assert int.from_bytes(w[3:5],'big')==e['_length']==e['session']['Record Length']
            assert [v['value'] for v in e['field_values']['values'] if v['name']=='tls.record.length']==[e['_length']]
            if refs(e):
                assert e['source_bytes'].get('CompletedIn',{}).get('Number')==max(refs(e)), 'new explicit completion evidence required'
            tls.append(e);by_hash[e['_hash']].append(e)
            for frame in refs(e):frame_scopes[frame].add(scope(e))
        if any(n in wanted for n in refs(e)):events.append(e)
    for e in events:
        for n in refs(e):frame_events[n].append(e)
    # Weber's complete JSON oracle preserves original frame numbering. Sharon
    # has a different ordering: export its complete TLS views independently.
    if name=='sharon':
        oracle_path=a.out/'sharon-tls-oracle.json'
        with oracle_path.open('wb') as out,(a.out/'sharon-tls-oracle.stderr').open('wb') as err:
            subprocess.run([str(a.tshark),'-2','-n','-r',str(capture),'-o','tcp.reassemble_out_of_order:TRUE','-Y','tls.record','-T','json','-x','--no-duplicate-keys'],stdout=out,stderr=err,check=True,timeout=600)
        ws_by_frame=collections.defaultdict(list);ws_by_hash=collections.defaultdict(set)
        for packet in read(oracle_path):
            l=packet['_source']['layers'];frame=int(l['frame']['frame.number'])
            for w in tls_records(l):ws_by_frame[frame].append({'sha256':sha(w),'length':len(w)-5});ws_by_hash[sha(w)].add(frame)
    def record_proof(e):
        candidates=sorted(f for f in ws_by_hash[e['_hash']] if scope(e) in frame_scopes[f])
        direct=[f for f in candidates if f in refs(e)]
        if direct:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'wireshark_frames':direct,'proof':'byte-identical complete record in same flow/direction/domain'}
        if name=='weber' and any(v in ('RDPUDP','RDPUDP2') for v in e.get('carrier_layers') or []):
            candidates=sorted(f for f in derived_frames[e['_hash']] if f in refs(e) and scope(e) in frame_scopes[f])
            if candidates:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'derived_oracle_original_frames':candidates,'derived_capture_sha256':derived_summary['derived_capture_sha256'],'proof':'independent derived RDPUDP TLS record; original oracle carrier lost TLS dissection'}
        # A delayed batch view must have a native sibling which both shares
        # the completed record's packet and spans to that exact oracle frame.
        for frame in candidates:
            hashes={r['sha256'] for r in ws_by_frame[frame]}
            siblings=[v for v in tls if scope(v)==scope(e) and v['_hash'] in hashes and refs(v) and max(refs(v))==frame and set(refs(v))&set(refs(e))]
            if siblings:return {'pdu':e['id'],'sha256':e['_hash'],'length':e['_length'],'contributors':refs(e),'byte_complete_frame':max(refs(e)),'wireshark_frames':[frame],'spanning_sibling_pdus':[v['id'] for v in siblings],'proof':'Wireshark batch deferral, proved by byte-identical spanning sibling in same flow/direction/domain'}
        raise AssertionError(('no constrained TLS byte proof',name,e['id'],refs(e),e['_hash']))
    dns_frames=[d['frame'] for d in baseline if d['field']=='dns.qry.name'];dns_layers={}
    if dns_frames:
        out=a.out/(name+'-dns-oracle.json')
        with out.open('wb') as f,(a.out/(name+'-dns-oracle.stderr')).open('wb') as er:
            subprocess.run([str(a.tshark),'-n','-r',str(capture),'-c',str(max(dns_frames)),'-Y','frame.number in {'+','.join(map(str,dns_frames))+'}','-T','json','-x','--no-duplicate-keys'],stdout=f,stderr=er,check=True,timeout=600)
        for packet in read(out):dns_layers[int(packet['_source']['layers']['frame']['frame.number'])]=packet['_source']['layers']
    result=[]
    for d in baseline:
        frame=d['frame'];row={'frame':frame,'field':d['field']}
        if d['field']=='tls.record.length':
            associated=[e for e in frame_events[frame] if e['protocol']=='tls' and e['status']=='decoded']
            proofs=[record_proof(e) for e in associated]
            missing=set(d['wireshark'])-set(e['_length'] for e in associated)
            for v in ws_by_frame[frame]:
                if v['length'] not in missing:continue
                choices=[e for e in by_hash[v['sha256']] if scope(e) in frame_scopes[frame]]
                assert len(choices)==1,(frame,v,'ambiguous omitted native record')
                proofs.append(record_proof(choices[0]));missing.discard(v['length'])
            assert not missing,(name,frame,missing)
            kind='contributor/frame attribution with exact record bytes'
            if any('derived_oracle_original_frames' in r for r in proofs):kind='contributor attribution plus original RDPUDP oracle limitation; independent derived byte proof'
            row.update({'classification':kind,'original_wireshark_lengths':d['wireshark'],'original_contributor_union_lengths':d['yak'],'records':proofs,'parser_value_error':False})
        else:
            ns=[e for e in frame_events[frame] if e['protocol'] in ('dns','mdns','llmnr') and e['status']=='decoded'];assert len(ns)==1
            e=ns[0];w=base64.b64decode(e['raw']);w=w[2:] if e['transport']=='tcp' else w
            assert w in bytes.fromhex(dns_layers[frame]['frame_raw'][0]),'native DNS bytes absent from original physical frame'
            q=e['session']['DNS']['Questions'][0];actual=expanded_name(w,12);assert actual==base64.b64decode(q['Name Wire'])
            text=[v['value'] for v in e['field_values']['values'] if v['name']=='dns.qry.name'];escaped=[v['value'] for v in e['field_values']['values'] if v['name']=='dns.qry.name.escaped']
            assert escaped==[q['Name']]
            fixed=set(v.lower().rstrip('.') for v in text)==set(d['wireshark'])
            row.update({'classification':'readable DNS name display fixed' if fixed else 'invalid UTF-8 retained losslessly; Wireshark replacement text differs','wire_name_sha256':sha(actual),'native_message_sha256':sha(w),'display_matches_wireshark':fixed,'escaped_name_preserved':True,'transaction_identity_uses_wire':True,'parser_value_error':False})
        result.append(row)
    assert len(result)==len(baseline)
    reviews[name]={'original_difference_count':len(baseline),'events_sha256':digest(path),'capture_sha256':digest(capture),'classification_counts':dict(collections.Counter(r['classification'] for r in result)),'unreviewed':0,'rows':result}
    print(json.dumps({'capture':name,'reviewed':len(result),'classes':reviews[name]['classification_counts']}),flush=True)
(a.out/'field-differences-reviewed.json').write_text(json.dumps(reviews,indent=2)+'\n',encoding='utf8')
