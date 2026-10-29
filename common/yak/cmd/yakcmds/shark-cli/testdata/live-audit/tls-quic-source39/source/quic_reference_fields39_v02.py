"""Compare isolated Wireshark fields to the already-frozen source AEAD graph.

Every authenticated packet/frame is compared before HTTP/3 views. Repeated H3
views have explicit connection/direction/stream/QPACK-payload source anchors;
they never create new source operations. Missing complete DATA views remain
reference omissions, not invented packet loss or native correctness claims.
"""
import argparse,collections,hashlib,importlib.util,json,sys,traceback,xml.etree.ElementTree as ET
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def fields(node,name):return [f for f in node.iter('field')if f.get('name')==name]
def number(field):
    s=field.get('show');return s=='True'if s in('True','False')else int(s,0)
def load(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m

def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('source','capture-root','audit','reference','crypto-reference','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir()
    summary=read(a.source/'summary.json');assert file_sha(a.source/'summary.json')=='6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859'
    assert file_sha(a.source/'packet-source.jsonl')==summary['packet_source_SHA256']and file_sha(a.source/'HTTP3-source.json')==summary['HTTP3_source_SHA256']
    sys.path.insert(0,str(a.audit));wire=load('reference_source_physical',a.audit/'capture_clock_wire_audit.py');quic=load('reference_source_quic_header',Path(__file__).parent/'quic_source39.py')
    cap=a.capture_root/'live.pcap';physical=wire.physical(cap);rows=[json.loads(l)for l in(a.source/'packet-source.jsonl').read_text('utf8').splitlines()];by_range={(r['frame'],tuple(r['packet_range'])):r for r in rows};by_frame=collections.defaultdict(list)
    for r in rows:by_frame[r['frame']].append(r)
    headers={(r['connection'],r['direction'],r['stream']):r for r in read(a.source/'HTTP3-source.json')}
    source_h3=read(a.source/'HTTP3-frame-source.json');head_blocks={(r['connection'],r['direction'],r['stream']):r for r in source_h3 if r['type']==1}
    bound={str(p):file_sha(p)for p in[Path(__file__),Path(__file__).parent/'quic_source39.py',a.source/'summary.json',a.source/'packet-source.jsonl',a.source/'HTTP3-source.json',a.source/'HTTP3-frame-source.json',cap,a.reference/'reference.pdml',a.reference/'producer.json']}
    cp=read(a.crypto_reference/'producer.json');assert file_sha(a.crypto_reference/'producer.json')=='133426d6744e495edf54cbd768143505c7332e63729ed2b725189fd3ba90e45a'
    assert cp['all_source_offset_length_and_raw_digest_occurrences_equal']is True and cp['source_summary_SHA256']==file_sha(a.source/'summary.json')
    assert file_sha(a.crypto_reference/'reference.json')==cp['export_SHA256']
    bound[str(a.crypto_reference/'producer.json')]=file_sha(a.crypto_reference/'producer.json');bound[str(a.crypto_reference/'reference.json')]=cp['export_SHA256']
    counts=collections.Counter();failures=[];head_views=collections.Counter();data_views=collections.Counter();WS_to_source={};source_to_WS={};matched=set();tail_views=0
    def check(actual,expected,owner,field):
        counts['field_checks']+=1
        if type(actual)is not type(expected)or actual!=expected:failures.append(dict(owner=owner,field=field,actual=actual,expected=expected))
    def scalar(node,alias,expected,owner):
        got=fields(node,alias);check(len(got),1,owner,alias+' cardinality')
        if len(got)==1:check(number(got[0]),expected,owner,alias)
    def blob(node,alias,expected,owner):
        got=fields(node,alias);check(len(got),1,owner,alias+' cardinality')
        if len(got)==1:
            raw=bytes.fromhex(got[0].get('value',''));check(len(raw),expected['bytes'],owner,alias+' bytes');check(sha(raw),expected['SHA256'],owner,alias+' SHA256')
    try:
        tree=ET.parse(a.reference/'reference.pdml').getroot();assert len(tree)==len(physical)
        for frame,p in enumerate(tree,1):
            scalar(p,'frame.number',frame,dict(frame=frame));scalar(p,'frame.cap_len',len(physical[frame-1]['wire']),dict(frame=frame));scalar(p,'frame.len',len(physical[frame-1]['wire']),dict(frame=frame))
            for q in p.findall('proto'):
                if q.get('name')!='quic':continue
                start=int(q.get('pos'));extent=[start,start+int(q.get('size'))];r=by_range.get((frame,tuple(extent)))
                if r is None:
                    tail=[t for t in summary['unprotected_unparsed_tails']if t['frame']==frame and t['range']==extent]
                    assert len(tail)==1;tail_views+=1;continue
                owner=dict(frame=frame,packet_SHA256=r['packet_SHA256']);matched.add((frame,tuple(extent)))
                if not r['authenticated']:counts['unopened_packets_not_assigned_fields']+=1;continue
                counts['authenticated_packets']+=1;scalar(q,'quic.header_form',int(r['long_header']),owner);scalar(q,'quic.packet_number',r['PN'],owner);scalar(q,'quic.packet_number_length',r['PN_bytes']-1,owner)
                raw=physical[frame-1]['wire'][start:extent[1]];cid=bytes.fromhex(r['destination_CID']);blob(q,'quic.dcid',dict(bytes=len(cid),SHA256=sha(cid)),owner)
                if r['long_header']:
                    h=quic.packet_header(raw);scalar(q,'quic.version',1,owner);scalar(q,'quic.long.packet_type',0 if r['space']=='initial'else 2,owner);scalar(q,'quic.dcil',len(h['dcid']),owner);scalar(q,'quic.scil',len(h['scid']),owner);blob(q,'quic.scid',dict(bytes=len(h['scid']),SHA256=sha(h['scid'])),owner)
                else:scalar(q,'quic.key_phase',bool(r['key_phase']),owner)
                ids=fields(p,'quic.connection.number')
                if ids:
                    other={number(f)for f in ids};assert len(other)==1;other=next(iter(other));cid=r['connection']
                    assert other not in WS_to_source or WS_to_source[other]==cid;assert cid not in source_to_WS or source_to_WS[cid]==other;WS_to_source[other]=cid;source_to_WS[cid]=other
                frs=[f for f in q.findall('field')if f.get('name')=='quic.frame'];check(len(frs),len(r['frames']),owner,'frame occurrence count')
                for index,(source,f)in enumerate(zip(r['frames'],frs)):
                    owner2=dict(owner,frame_index=index);t=source['type'];scalar(f,'quic.frame_type',t,owner2);check(int(f.get('size')),source['plaintext_range'][1]-source['plaintext_range'][0],owner2,'plaintext frame extent bytes');counts['authenticated_frame_occurrences']+=1
                    if t==0:scalar(f,'quic.padding_length',source['length'],owner2)
                    elif t==2:
                        for alias,key in [('largest_acknowledged','largest'),('ack_delay','delay')]:scalar(f,'quic.ack.'+alias,source[key],owner2)
                        scalar(f,'quic.ack.ack_range_count',len(source['ranges'])-1,owner2);scalar(f,'quic.ack.first_ack_range',source['ranges'][0][1],owner2)
                        for alias,expected in [('quic.ack.gap',[v[0]for v in source['ranges'][1:]]),('quic.ack.ack_range',[v[1]for v in source['ranges'][1:]])]:check([number(v)for v in fields(f,alias)],expected,owner2,alias+' ordered occurrences')
                    elif t==6:
                        scalar(f,'quic.crypto.offset',source['offset'],owner2);scalar(f,'quic.crypto.length',source['length'],owner2);check(len(fields(f,'quic.crypto.crypto_data')),1,owner2,'CRYPTO FT_NONE presence');check(int(fields(f,'quic.crypto.crypto_data')[0].get('size')),source['data']['bytes'],owner2,'CRYPTO FT_NONE source extent');counts['CRYPTO_raw_digest_closed_by_bound_raw_JSON']+=1
                    elif 8<=t<=15:
                        scalar(f,'quic.stream.stream_id',source['stream'],owner2);scalar(f,'quic.stream.fin',source['FIN'],owner2);scalar(f,'quic.stream.off',bool(t&4),owner2);scalar(f,'quic.stream.len',bool(t&2),owner2)
                        if t&4:scalar(f,'quic.stream.offset',source['offset'],owner2)
                        else:check(fields(f,'quic.stream.offset'),[],owner2,'no invented wire offset field')
                        if t&2:scalar(f,'quic.stream.length',source['length'],owner2)
                        blob(f,'quic.stream_data',source['data'],owner2)
                    elif t==24:
                        scalar(f,'quic.nci.sequence',source['sequence'],owner2);scalar(f,'quic.nci.retire_prior_to',source['retire_prior_to'],owner2);scalar(f,'quic.nci.connection_id.length',source['CID']['bytes'],owner2);blob(f,'quic.nci.connection_id',source['CID'],owner2);blob(f,'quic.nci.stateless_reset_token',source['reset_token'],owner2)
                    elif t==25:scalar(f,'quic.rci.sequence',source['sequence'],owner2)
                    elif t in(26,27):blob(f,'quic.path_challenge.data'if t==26 else'quic.path_response.data',source['data'],owner2)
                    elif t==29:
                        scalar(f,'quic.cc.error_code.app',source['error'],owner2);check([v.get('show')for v in fields(f,'quic.cc.reason_phrase')],[source['reason']],owner2,'close reason')
                    elif t not in(1,30):failures.append(dict(owner=owner2,field='uncovered source frame scalar contract',type=t))
            owners={(r['connection'],r['direction'])for r in by_frame[frame]if r['authenticated']}
            for f in p.iter('field'):
                if f.get('name')!='http3.frame':continue
                typ=fields(f,'http3.frame_type');sid=fields(f,'http3.frame_streamid')
                if len(typ)!=1 or len(sid)!=1:continue
                typ,sid=number(typ[0]),number(sid[0])
                if sid&2:continue
                assert len(owners)==1;cid,d=next(iter(owners));key=(cid,d,sid);assert key in headers
                expected=headers[key];owner=dict(frame=frame,connection=cid,direction=d,stream=sid)
                if typ==1:
                    block=head_blocks[key];payload=fields(f,'http3.frame_payload');assert len(payload)==1
                    raw=bytes.fromhex(payload[0].get('value',''));check(sha(raw),block['payload_SHA256'],owner,'QPACK block literal source SHA256');check(len(raw),block['length'],owner,'QPACK block source length')
                    mapping={':method':'method',':scheme':'scheme',':authority':'authority',':path':'path',':status':'status','content-type':'content_type'}
                    for name,value in expected['headers']:check([v.get('show')for v in fields(f,'http3.headers.'+mapping[name])],[value],owner,'HTTP3 '+name)
                    head_views[key]+=1
                elif typ==0:
                    for data in fields(f,'http3.data'):
                        raw=bytes.fromhex(data.get('value',''));check(sha(raw),expected['body_SHA256'],owner,'complete HTTP3 DATA peer SHA256');check(len(raw),expected['body_bytes'],owner,'complete HTTP3 DATA peer bytes');data_views[key]+=1
        assert matched==set(by_range),'all literal source packet ranges have reference views'
        assert len(source_to_WS)==len(WS_to_source)==3
        missing_heads=[dict(connection=k[0],direction=k[1],stream=k[2])for k in headers if not head_views[k]]
        missing_data=[dict(connection=k[0],direction=k[1],stream=k[2],source_body_bytes=v['body_bytes'],source_body_SHA256=v['body_SHA256'])for k,v in headers.items()if not data_views[k]]
        duplicates=[dict(connection=k[0],direction=k[1],stream=k[2],reference_views=n,source_QPACK_block_SHA256=head_blocks[k]['payload_SHA256'])for k,n in head_views.items()if n>1]
        after={p:file_sha(p)for p in bound};assert bound==after
        result=dict(scope=__doc__,source_summary_SHA256=file_sha(a.source/'summary.json'),counts=dict(counts),authenticated_packet_and_frame_scalar_failures=len(failures),failures=failures,WS_to_source_connection_bijection=WS_to_source,explicit_unprotected_tail_views=tail_views,unique_source_HTTP3_headers=len(headers),reference_HTTP3_header_views=sum(head_views.values()),unique_reference_HTTP3_header_anchors=len(head_views),reference_repeated_header_views=duplicates,missing_header_anchors=missing_heads,complete_reference_DATA_views=sum(data_views.values()),missing_complete_reference_DATA_anchors=missing_data,missing_DATA_means_source_packet_loss=False,full_WS_field_or_native_parity_claim=False,no_native_or_Yak_output_read=True,inputs=bound,inputs_unchanged=bound==after)
        write(a.out/'summary.json',result);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'),scalar_checks=counts['field_checks'],scalar_failures=len(failures),missing_heads=len(missing_heads),missing_DATA=len(missing_data),repeated_header_views=sum(n-1 for n in head_views.values()))),flush=True)
        if failures:raise SystemExit(1)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
