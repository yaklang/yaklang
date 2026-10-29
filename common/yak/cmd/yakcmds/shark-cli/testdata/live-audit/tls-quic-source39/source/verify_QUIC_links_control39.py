"""Close source STREAM-anchor isolation and remaining H3 MAX_PUSH_ID scalars.

Request PDU is checked as a literal first authenticated client STREAM anchor,
not falsely described as the packet completing an HTTP request header. Full
request semantics were independently established by the source QPACK witness.
"""
import argparse,base64,collections,hashlib,importlib.util,json,sys
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def rows(p):return[json.loads(line)for line in Path(p).read_text('utf-8-sig').splitlines()]
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('sources','phase','audit','recovery','actual','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();sys.path.insert(0,str(a.audit));root=a.sources;src=root/'QUIC-source01-audit01'
    assert file_sha(root/'SDK-d677-source39-audit01/summary.json')=='f75f6b9b62a936e1aa3033eb1eca8c5a4cd25ba4b7b3177a2f64d8b851ae158a'
    proof=read(root/'SDK-d677-source39-audit01/summary.json');assert proof['results']['QUIC']['failed_events_or_app_frames']==0
    source=rows(src/'packet-source.jsonl');conns={r['connection']:r for r in read(src/'connection-source.json')};events=rows(a.actual/'QUIC/events.jsonl');ep=read(a.actual/'QUIC/events-producer.json');assert file_sha(a.actual/'QUIC/events.jsonl')==ep['export_SHA256']
    source_bykey={(r['frame'],r['packet_SHA256']):r for r in source};native_bykey={};native_to_source={};flow_owner={}
    for e in events:
        refs=e['source_bytes']['PacketRefs']or[]
        if len(refs)!=1:continue
        key=refs[0]['Number'],sha(base64.b64decode(e['raw']or''))
        if key not in source_bykey:continue
        assert key not in native_bykey;native_bykey[key]=e;r=source_bykey[key];native_to_source[e['id']]=r
        if r['authenticated']:flow_owner[e['flow']]=r['connection']
    anchors={};source_anchors=[]
    for r in source:
        if not r['authenticated']or r['direction']!=0:continue
        for fr in r['frames']:
            if fr['name']!='STREAM'or fr['stream']&2:continue
            key=r['connection'],fr['stream']
            if key not in anchors:
                e=native_bykey[r['frame'],r['packet_SHA256']];anchors[key]=e['id'];source_anchors.append(dict(connection=key[0],stream=key[1],source_frame=r['frame'],source_PN=r['PN'],first_observed_stream_offset=fr['offset'],native_PDU=e['id'],packet_SHA256=r['packet_SHA256']))
    assert len(anchors)==18;counts=collections.Counter();failures=[]
    for e in events:
        r=native_to_source.get(e['id'])
        if not r:continue
        if not r['authenticated']:
            assert len(r['candidate_connections'])==1 and flow_owner[e['flow']]==r['candidate_connections'][0];counts['bad_tag_confirmed_CID_owner_no_stream_inheritance']+=1;continue
        first=0
        for fr,want in zip((e['session']or{}).get('Frames',[]),r['frames']):
            if want['name']!='STREAM'or want['stream']&2:continue
            anchor=anchors[r['connection'],want['stream']]
            if fr.get('Request PDU ID')!=anchor or fr.get('Transaction ID')!=anchor:failures.append(dict(PDU=e['id'],stream=want['stream'],error='stream anchor crosses source connection/SID or wrong source first observation'))
            if not first:first=anchor
            counts['bidirectional_STREAM_anchor_checks']+=1
        if e['transaction']!=first or e['response_to']!=(first if r['direction']==1 else 0):failures.append(dict(PDU=e['id'],error='packet transaction/response does not follow its first observed source STREAM anchor',expected_transaction=first))
        counts['packet_transaction_response_checks']+=1
    q=load('QUIC_extra_source',root/'quic_source39.py');crypto=load('QUIC_extra_KDF',a.recovery);wire=load('QUIC_extra_physical',a.audit/'capture_clock_wire_audit.py');physical=wire.physical(a.phase/'fresh-QUIC-epochs39-01/live.pcap');keys=crypto.keys(a.phase/'fresh-QUIC-epochs39-01/session.keys');cells=collections.defaultdict(dict)
    for r in source:
        if not r['authenticated']or not any(f['name']=='STREAM'and f['stream']&2 for f in r['frames']):continue
        c=conns[r['connection']];cipher=next(k for k,v in q.SUITES.items()if v==c['suite']);random=next(rnd for label,rnd in keys if label=='CLIENT_TRAFFIC_SECRET_0'and sha(rnd)==c['client_random_SHA256']);secret=keys[('SERVER'if r['direction']else'CLIENT')+'_TRAFFIC_SECRET_0',random];raw=physical[r['frame']-1]['wire'][r['packet_range'][0]:r['packet_range'][1]]
        opened=q.open_packet(raw,1+len(bytes.fromhex(r['destination_CID'])),False,secret,cipher,r['epoch'],r['PN']-1,crypto.hkdf);assert opened is not None and sha(opened['plaintext'])==r['plaintext_SHA256']
        for f in q.frames(opened['plaintext']):
            if f['name']=='STREAM'and f['stream']&2:q.cells_add(cells[r['connection'],r['direction'],f['stream']],f['offset'],f['data'],r['frame'])
    controls={}
    for key,c in cells.items():
        data=q.prefix(c);typ,at=q.vi(data,0)
        if typ!=0:continue
        while at<len(data):
            start=at;typ,p=q.vi(data,at);n,p=q.vi(data,p);end=p+n;assert end<=len(data)
            if typ==13:v,stop=q.vi(data,p);assert stop==end;controls[(*key,start)]=v
            at=end
    seen=collections.Counter()
    for e in events:
        r=native_to_source.get(e['id'])
        if not r or not r['authenticated']:continue
        for fr in(e['session']or{}).get('Frames',[]):
            if fr.get('Frame Type')!='STREAM':continue
            for hf in fr.get('HTTP3 Frames')or[]:
                if hf['Frame Type Value']!=13:continue
                key=r['connection'],r['direction'],hf['Stream ID'],hf['Stream Offset'];assert key in controls and hf['Frame Type']=='MAX_PUSH_ID'and hf['Push ID']==controls[key];seen[key]+=1;counts['source_MAX_PUSH_ID_scalar_checks']+=1
    assert seen==collections.Counter({k:1 for k in controls})and len(controls)==3
    result=dict(scope=__doc__,source_manifest_SHA256=file_sha(root/'source-manifest01.json'),SDK_core_proof_SHA256=file_sha(root/'SDK-d677-source39-audit01/summary.json'),SDK_export_SHA256=file_sha(a.actual/'QUIC/events.jsonl'),counts=dict(counts),failed_checks=len(failures),failures=failures,source_stream_anchors=source_anchors,source_anchors_starting_after_offset0=sum(r['first_observed_stream_offset']>0 for r in source_anchors),request_anchor_not_always_header_completion=True,source_expected_values_from_original_authenticated_capture=True,whole_event_JSON_equality_claim=False,checker_SHA256=file_sha(__file__))
    with a.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=file_sha(a.out),failures=len(failures),checks=dict(counts))),flush=True)
    if failures:raise SystemExit(1)
if __name__=='__main__':main()
