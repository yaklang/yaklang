"""Independent physical/source labels plus exact SDK-to-CLI private-field bridge.

Source facts were frozen before producer execution. This later auditor uses
source values, the immutable public JSON format, and a separately closed SDK
source certificate. It does not normalize IDs, types, labels or occurrences.
"""
from pathlib import Path
import base64,collections,datetime,hashlib,importlib.util,json,re,sys

root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
actual=root/'fresh-TLS-QUIC-d677-actual39-01'
source=root/'fresh-TLS-QUIC-source39-01'
out=root/sys.argv[1];out.mkdir()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def rows(p):return[json.loads(s)for s in Path(p).read_text('utf-8-sig').splitlines()]
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
assert sha(source/'source-manifest01.json')=='8be15e99bfbd572364a4c0acf5c6b19eb4cd2e2f7c02a0a5c5b9a3fe5482bf72'
sdkproof=source/'SDK-d677-source39-audit01/summary.json'
assert sha(sdkproof)=='f75f6b9b62a936e1aa3033eb1eca8c5a4cd25ba4b7b3177a2f64d8b851ae158a'
reader=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/vxlan_endpoint_projection_audit.py'
sys.path.insert(0,str(reader.parent));spec=importlib.util.spec_from_file_location('source_records',reader);module=importlib.util.module_from_spec(spec);spec.loader.exec_module(module)
def timestamp(text):
    m=re.fullmatch(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d)(?:\.(\d{1,9}))?(Z|[+-]\d\d:\d\d)',text);assert m
    d=datetime.datetime.strptime(m[1],'%Y-%m-%dT%H:%M:%S')
    offset=0 if m[3]=='Z'else(1 if m[3][0]=='+'else-1)*(int(m[3][1:3])*3600+int(m[3][4:6])*60)
    sec=(d-datetime.datetime(1970,1,1)).days*86400+d.hour*3600+d.minute*60+d.second-offset
    return sec*10**9+int((m[2]or'').ljust(9,'0'))
def physical(record):
    raw=record['wire'];assert record['link']==0 and raw[:4]in(b'\x02\x00\x00\x00',b'\x00\x00\x00\x02')
    assert raw[4]>>4==4;ihl=(raw[4]&15)*4;at=4+ihl;typ=raw[13]
    transport={6:'TCP',17:'UDP'}[typ]
    src='.'.join(str(v)for v in raw[16:20]);dst='.'.join(str(v)for v in raw[20:24])
    sport=int.from_bytes(raw[at:at+2],'big');dport=int.from_bytes(raw[at+2:at+4],'big')
    header=(raw[at+12]>>4)*4 if typ==6 else 8
    return dict(source=src,destination=dst,source_port=sport,destination_port=dport,transport=transport,layers=['Loopback','IPv4',transport],payload=raw[at+header:])
def flat(snapshot):
    result={}
    for field in snapshot['values']:result.setdefault(field['name'],[]).append(field['value'])
    return result

inputs=dict(source_manifest=source/'source-manifest01.json',SDK_certificate=sdkproof,record_reader=reader,controller=Path(__file__))
for name,folder in [('TLS','fresh-TLS-resumption39-03'),('QUIC','fresh-QUIC-epochs39-01')]:
    for label,p in [('capture',root/folder/'live.pcap'),('events',actual/name/'events.jsonl'),('CLI',actual/name/'yak.jsonl'),('events_producer',actual/name/'events-producer.json'),('CLI_producer',actual/name/'yak-producer.json')]:inputs[name+'/'+label]=p
before={k:dict(path=str(p),SHA256=sha(p))for k,p in inputs.items()};write(out/'inputs-before.json',before)
tls_records=rows(source/'TLS-source03-audit02/record-source.jsonl');http=read(source/'TLS-source03-audit02/HTTP-source.json');conns=read(source/'TLS-source03-audit02/connection-source.json')
record_at=collections.defaultdict(list);http_at=set();owner={}
for r in tls_records:
    for frame in r['source_frames']:record_at[frame].append(r)
for h in http:
    http_at.update(frame for p in h['authenticated_record_parents']for frame in p['source_frames'])
for cid,c in conns.items():
    for frame in c['frames']:assert frame not in owner;owner[frame]=c
quic=rows(source/'QUIC-source01-audit01/packet-source.jsonl');h3_at=set();bad_at=set()
for p in quic:
    if p['authenticated']:
        if any(8<=f['type']<=15 for f in p['frames']):h3_at.add(p['frame'])
    else:bad_at.add(p['frame'])
checks=[];diffs=[]
for name,folder in [('TLS','fresh-TLS-resumption39-03'),('QUIC','fresh-QUIC-epochs39-01')]:
    capture=root/folder/'live.pcap';packets=list(module.capture_records(capture));cli=rows(actual/name/'yak.jsonl');events=rows(actual/name/'events.jsonl');byid={e['id']:e for e in events};assert len(byid)==len(events)
    for kind in('events','yak'):
        proof=read(actual/name/(kind+'-producer.json'));assert proof['source']=='d6772965c669e288705f7e0d0e25c62972226832'and proof['exit_code']==0
        assert proof['export_SHA256']==sha(actual/name/(kind+'.jsonl'))and proof['inputs']['capture']['SHA256']==sha(capture)
        build=root/'bins-38'/(kind+'-receipt.json');assert sha(build)==proof['build_receipt_SHA256']
        assert sha(root/'bins-38'/(kind+'.exe'))==proof['binary_SHA256']==read(build)['binary_SHA256']
    assert len(cli)==len(packets)
    embedded=collections.Counter();field_occurrences=0;label_counts=collections.Counter();source_label_counts=collections.Counter()
    for frame,(record,row)in enumerate(zip(packets,cli),1):
        p=physical(record)
        assert row['number']==frame and timestamp(row['time'])==record['time_ns']
        assert row['length']==record['original_length']and row['captured_length']==record['captured_length']
        assert row['capture_domain']==dict(Section=record['section'],Interface=record['interface'],Encapsulation='')
        for key in('source','destination','source_port','destination_port','transport','layers'):assert row[key]==p[key],(name,frame,key,row[key],p[key])
        if p['transport']=='UDP'and(name=='TLS'or frame not in{v['frame']for v in quic}):expected='UDP'
        elif name=='QUIC':expected='HTTP/3'if frame in h3_at else'QUIC'
        elif not p['payload']:expected='TCP'
        elif frame in http_at:expected='HTTP/1.1'
        elif any(r['legacy_version']==769 for r in record_at[frame]):expected='TLS'
        else:expected='TLSv1.2'if owner[frame]['profile']=='tls12'else'TLSv1.3'
        source_label_counts[expected]+=1;label_counts[row['protocol']]+=1
        if row['protocol']!=expected:diffs.append(dict(capture=name,frame=frame,source_expected=expected,actual=row['protocol']))
        for pdu in row.get('pdus',[]):
            ident=pdu['id'];assert type(ident)is int and ident>0 and ident in byid
            event=byid[ident];assert set(pdu)=={'id','protocol','status','completeness','response_to','byte_source','fields','field_values'}
            for key in('protocol','status','completeness','response_to'):assert pdu[key]==event[key],(name,frame,ident,key)
            assert pdu['byte_source']==event['source_bytes']and pdu['field_values']==event['field_values']
            assert pdu['fields']==flat(event['field_values'])
            assert any(ref['Number']==frame for ref in pdu['byte_source']['PacketRefs']),(name,frame,ident,'foreign byte owner')
            embedded[ident]+=1;field_occurrences+=len(pdu['field_values']['values'])
        if name=='QUIC'and frame in bad_at:
            assert row['protocol']=='QUIC'and row['protocol_status']=='malformed'
            for pdu in row['pdus']:
                values=pdu['fields'];assert 'quic.packet_number'not in values and 'quic.key_phase'not in values
    assert set(embedded)==set(byid)and all(v==1 for v in embedded.values()),'No missing,duplicate or invented PDU publication'
    checks.append(dict(capture=name,physical_records=len(packets),unique_embedded_PDUs=len(embedded),private_field_occurrences=field_occurrences,actual_labels=dict(label_counts),source_expected_labels=dict(source_label_counts)))
after={k:dict(path=str(p),SHA256=sha(p))for k,p in inputs.items()};assert before==after;write(out/'inputs-after.json',after)
summary=dict(scope=__doc__,source='d6772965c669e288705f7e0d0e25c62972226832',input_hashes_unchanged=True,checks=checks,label_differences=diffs,normalization=False,ID_mapping=False,source_facts_precede_producer=True,CLI_constructor_itself_is_later_than_producer=True,SDK_source_certificate_SHA256=sha(sdkproof),controller_SHA256=sha(__file__))
write(out/'summary.json',summary);print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),label_differences=len(diffs),physical_records=sum(x['physical_records']for x in checks))))
assert not diffs,'Source-derived label disagreements retained in summary'
