"""Close source owner/offset/FIN and literal reference view of three DATA gaps.

No conclusion about unexported Wireshark memory is drawn. Exact body bytes are
looked for under every PDML value owner, including complete reassembled-stream
suffixes, rather than counting one selected alias or assuming a tail frame.
"""
import argparse,collections,hashlib,json,xml.etree.ElementTree as ET
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def fields(n,name):return [f for f in n.iter('field')if f.get('name')==name]
def num(f):return int(f.get('show'),0)
def merge(ranges):
    out=[]
    for lo,hi in sorted(ranges):
        if not out or out[-1][1]<lo:out.append([lo,hi])
        else:out[-1][1]=max(out[-1][1],hi)
    return out
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for name in('source','field-proof','reference','out'):ap.add_argument('--'+name,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists()
    assert file_sha(a.source/'summary.json')=='6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859'
    assert file_sha(a.field_proof/'summary.json')=='07112a08fd973eadb8c51cdf67a76462cd242af40ec84c347f4f020bbd51f917'
    proof=read(a.field_proof/'summary.json');assert proof['authenticated_packet_and_frame_scalar_failures']==0
    files=[Path(__file__),a.source/'summary.json',a.source/'packet-source.jsonl',a.source/'HTTP3-source.json',a.source/'HTTP3-frame-source.json',a.field_proof/'summary.json',a.reference/'reference.pdml'];bound={str(p):file_sha(p)for p in files}
    packets=[json.loads(line)for line in(a.source/'packet-source.jsonl').read_text('utf8').splitlines()];http=read(a.source/'HTTP3-source.json');H3=read(a.source/'HTTP3-frame-source.json');tree=ET.parse(a.reference/'reference.pdml').getroot();by_frame=collections.defaultdict(list)
    for r in packets:by_frame[r['frame']].append(r)
    results=[]
    for miss in proof['missing_complete_reference_DATA_anchors']:
        key=miss['connection'],miss['direction'],miss['stream'];entry=next(r for r in http if(r['connection'],r['direction'],r['stream'])==key);h3=next(r for r in H3 if(r['connection'],r['direction'],r['stream'],r['type'])==(*key,0));contrib=[];allranges=[];FIN=None;first_complete=None
        for r in packets:
            if r.get('connection')!=key[0]or r['direction']!=key[1]:continue
            for f in r.get('frames',[]):
                if f['name']!='STREAM'or f['stream']!=key[2]:continue
                allranges.append([f['offset'],f['offset']+f['length']]);coverage=merge(allranges)
                item=dict(frame=r['frame'],PN=r['PN'],epoch=r['epoch'],source_port=r['source_port'],destination_port=r['destination_port'],destination_CID=r['destination_CID'],offset=f['offset'],length=f['length'],FIN=f['FIN'],data_SHA256=f['data']['SHA256'],packet_SHA256=r['packet_SHA256'],contiguous_prefix_bytes=coverage[0][1]if coverage[0][0]==0 else 0)
                if f['FIN']:FIN=f['offset']+f['length'];assert FIN==entry['final_size']
                if FIN is not None and coverage==[[0,FIN]]and first_complete is None:first_complete=r['frame']
                contrib.append(item)
        assert FIN==entry['final_size']and merge(allranges)==[[0,FIN]]and first_complete==h3['completed_source_frame']
        body_matches=[];views=[];annotations=[];experts=[]
        for frame,p in enumerate(tree,1):
            owners={(r['connection'],r['direction'])for r in by_frame[frame]if r['authenticated']}
            if key[:2]not in owners:continue
            assert len(owners)==1
            for q in p.findall('proto'):
                if q.get('name')!='quic':continue
                for fr in q.findall('field'):
                    if fr.get('name')!='quic.frame':continue
                    sids=fields(fr,'quic.stream.stream_id')
                    if len(sids)==1 and num(sids[0])==key[2]:
                        ann=[num(v)for v in fields(fr,'quic.reassembled_in')]
                        if ann:annotations.append(dict(frame=frame,reassembled_in=ann))
            for f in p.iter('field'):
                v=f.get('value','')
                if len(v)>=entry['body_bytes']*2:
                    try:raw=bytes.fromhex(v)
                    except ValueError:continue
                    if sha(raw[-entry['body_bytes']:])==entry['body_SHA256']:body_matches.append(dict(frame=frame,field=f.get('name'),pos=f.get('pos'),size=f.get('size'),value_bytes=len(raw),exact_body=len(raw)==entry['body_bytes'],complete_body_as_suffix=True))
                if f.get('name')=='_ws.expert.message':experts.append(dict(frame=frame,text=f.get('show')))
                if f.get('name')!='http3.frame':continue
                types=fields(f,'http3.frame_type');sids=fields(f,'http3.frame_streamid')
                if len(types)!=1 or len(sids)!=1 or num(sids[0])!=key[2]:continue
                lengths=fields(f,'http3.frame_length');payload=fields(f,'http3.frame_payload');raw=bytes.fromhex(payload[0].get('value',''))if len(payload)==1 else b''
                views.append(dict(frame=frame,literal_frame_type=num(types[0]),declared_payload_length=num(lengths[0])if len(lengths)==1 else None,exported_payload_bytes=len(raw),payload_SHA256=sha(raw),complete_DATA_fields=len(fields(f,'http3.data')),matches_source_complete_body=sha(raw)==entry['body_SHA256']))
        fin_frame=next(r['frame']for r in contrib if r['FIN']);results.append(dict(connection=key[0],direction=key[1],stream=key[2],source_final_size=FIN,source_body_bytes=entry['body_bytes'],source_body_SHA256=entry['body_SHA256'],source_first_FIN_frame=fin_frame,source_first_complete_contiguous_frame=first_complete,source_missing_range_repaired_after_FIN=first_complete>fin_frame,source_all_overlapping_bytes_equal_proof_SHA256=file_sha(a.source/'summary.json'),source_authenticated_STREAM_contributors=contrib,source_repeated_offset0_frames=[r['frame']for r in contrib if r['offset']==0],reference_reassembly_annotations=annotations,reference_H3_views=views,reference_complete_body_any_alias_or_fullstream_suffix_matches=body_matches,reference_experts=experts,reference_no_complete_body_view_exported=not body_matches,claim='source is complete; reference PDML lacks complete body view,not proof of lost source bytes or unexported reference state'))
    assert len(results)==3;after={p:file_sha(p)for p in bound};assert bound==after
    result=dict(scope=__doc__,source_summary_SHA256=file_sha(a.source/'summary.json'),source_scalar_reference_proof_SHA256=file_sha(a.field_proof/'summary.json'),cases=3,source_all_STREAM_offsets_lengths_FIN_and_data_digests_reference_equal=True,source_repeated_offset0_cases=sum(len(r['source_repeated_offset0_frames'])>1 for r in results),source_gap_filled_after_FIN_cases=sum(r['source_missing_range_repaired_after_FIN']for r in results),reference_missing_complete_body_view_cases=sum(r['reference_no_complete_body_view_exported']for r in results),results=results,inputs=bound,inputs_unchanged=True,no_native_or_Yak_output_read=True,no_claim_about_unexported_Wireshark_memory=True)
    with a.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=file_sha(a.out),cases=3,source_complete=True,post_FIN_repair_cases=result['source_gap_filled_after_FIN_cases'],missing_reference_complete_body_views=result['reference_missing_complete_body_view_cases'])),flush=True)
if __name__=='__main__':main()
