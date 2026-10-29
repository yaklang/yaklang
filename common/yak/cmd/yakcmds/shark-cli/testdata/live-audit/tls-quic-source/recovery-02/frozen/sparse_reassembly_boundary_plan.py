"""Independent captured STREAM intervals and bounded HTTP/3 source observations.

This is a source/state boundary description, not a promise of application
recovery. Missing captured STREAM bytes, QPACK instructions, request context and
peer delivery remain separate from short-packet AEAD authentication.
"""
import base64,collections,hashlib,json,sys
import xml.etree.ElementTree as ET
from pathlib import Path
from sparse_epoch_packet_audit import literal_frames
from quic_sparse_epoch import unprotect,values,num,vint
from recovery_probe import keys,file_sha,sha,write

def intervals(cells):
    out=[]
    for x in sorted(cells):
        if out and out[-1][1]==x:out[-1][1]=x+1
        else:out.append([x,x+1])
    return out

def run():
    phase=Path(__file__).parent.parent;out=phase/'QUIC-sparse-application-boundaries-before-01'
    audit=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\yak\cmd\yakcmds\shark-cli\testdata\live-audit')
    sys.path.insert(0,str(audit));from vxlan_endpoint_projection_audit import capture_records
    plan=json.loads((phase/'QUIC-sparse-epoch-plan-before-01/plan-before.json').read_text('utf8'))
    before=json.loads((phase/'QUIC-sparse-forward-plan-before-01/summary.json').read_text('utf8'))
    rows=[json.loads(x)for x in(phase/'QUIC-sparse-epoch-plan-before-01/packet-source-plan.jsonl').read_text('utf8').splitlines()]
    records=list(capture_records(Path(plan['source_capture'])));secret=keys(plan['cases'][0]['keylog'])
    ws=ET.parse(phase/'TLS-QUIC-reference25-01/quic-original/reference.pdml').getroot();randoms={};suites={}
    for p in ws:
        cid=num(p,'quic.connection.number')
        for f in values(p,'tls.handshake.random'):
            r=bytes.fromhex(f.get('show').replace(':',''))
            if ('CLIENT_TRAFFIC_SECRET_0',r)in secret:randoms[cid]=r
        for f in values(p,'tls.handshake.ciphersuite'):suites[cid]=int(f.get('show'),0)
    index={v['source_frame']:v['number']for v in plan['ledger']};streams={};fins={};by_source={}
    for r in rows:
        if r['source_frame']not in index or not r.get('independently_AEAD_authenticated'):continue
        raw=records[r['source_frame']-1]['wire'][r['packet_offset']:r['packet_end']]
        label=('SERVER'if r['direction']else'CLIENT')+'_TRAFFIC_SECRET_0'
        plain,_,_=unprotect(raw,r['packet_number'],bytes.fromhex(r['destination_cid']),secret[label,randoms[r['oracle_connection']]],suites[r['oracle_connection']],r['epoch'])
        assert sha(plain)==r['plaintext_SHA256'];frames=literal_frames(plain)
        by_source[index[r['source_frame']],r['packet_SHA256']]=frames
        for f in frames:
            if f['Frame Type']!='STREAM':continue
            key=(r['oracle_connection'],r['direction'],f['Stream ID']);cells=streams.setdefault(key,{})
            body=base64.b64decode(f['Stream Data']);assert len(body)==f['Length']
            for off,b in enumerate(body,f['Offset']):
                assert off not in cells or cells[off]==b;cells[off]=b
            if f['FIN']:
                end=f['Offset']+len(body);assert key not in fins or fins[key]==end;fins[key]=end
    native=[json.loads(x)for x in(phase/'QUIC-sparse-epoch-actual26-01/all-client-middle-epoch-omitted/events.jsonl').read_text('utf8').splitlines()]
    byid={e['id']:e for e in native};selected=[e for e in native if e['id']in before['target_remaining_events_requiring_explicit_state_review']]
    assert len(selected)==117 and all(e['flow']==1 and e['direction']==1 for e in selected)
    exact_h3=[]
    for e in selected:
        for fr in(e.get('session')or{}).get('Frames',[]):
            for h in fr.get('HTTP3 Frames',[]):
                key=(0,1,h['Stream ID']);cells=streams[key];start=h['Stream Offset'];end=start+h['Frame Wire Length']
                assert all(i in cells for i in range(start,end));w=bytes(cells[i]for i in range(start,end))
                assert sha(w)==h['Frame SHA256'];typ,at=vint(w,0);n,payload=vint(w,at)
                assert typ==h['Frame Type Value']and n==h['Payload Length']and payload+n==len(w)
                assert base64.b64decode(h['Frame Header Bytes'])==w[:payload]
                for view,lo,hi in [(h['Byte Source'],start,end),(h['Payload Byte Source'],start+payload,end)]:
                    assert view['Kind']=='quic-authenticated-stream-reassembly'and view['Stream ID']==h['Stream ID']and view['Direction']==1
                    assert(view['Offset'],view['Length'])==(lo,hi-lo)and view['Byte Coverage Complete']is True
                    coverage=set();parents=set()
                    for r in view['Ranges']:
                        p=byid[r['Parent PDU']];assert p['flow']==e['flow']and p['direction']==1 and p['id']<=e['id']
                        assert p['session']['Authentication Verified']is True
                        refs=p['source_bytes']['PacketRefs'];assert len(refs)==1 and refs[0]['Domain']==p['domain']
                        core=by_source[refs[0]['Number'],p['source_bytes']['SHA256']]
                        begin,finish=r['Offset'],r['Offset']+r['Length'];assert lo<=begin<finish<=hi
                        assert r['Packet References']==[{'Packet':refs[0]['Number'],**refs[0]['Domain']}]
                        source_fr=[f for f in core if f['Frame Type']=='STREAM'and f['Stream ID']==h['Stream ID']and f['Offset']<=begin and finish<=f['Offset']+f['Length']]
                        assert source_fr
                        assert any(base64.b64decode(f['Stream Data'])[begin-f['Offset']:finish-f['Offset']]==bytes(cells[i]for i in range(begin,finish))for f in source_fr)
                        coverage.update(range(begin,finish));parents.add(p['id'])
                    assert coverage==set(range(lo,hi))and view['Parent PDUs']==sorted(parents)
                exact_h3.append(dict(PDU=e['id'],stream_id=h['Stream ID'],offset=start,length=len(w),type=typ,frame_SHA256=sha(w)))
    assert len(exact_h3)==4
    target=[]
    for key,cells in sorted(streams.items()):
        if key[0]!=0:continue
        spans=intervals(cells);end=fins.get(key);full=bool(end is not None and spans==[[0,end]])
        target.append(dict(connection=key[0],direction=key[1],stream_id=key[2],captured_intervals=spans,observed_FIN_offset=end,byte_coverage_complete_to_FIN=full,known_captured_bytes=len(cells),missing_prefix=bool(spans and spans[0][0]>0),internal_holes=[[a[1],b[0]]for a,b in zip(spans,spans[1:])]))
    out.mkdir(exist_ok=False);write(out/'source-stream-intervals.json',target)
    summary=dict(scope=__doc__,script_SHA256=file_sha(__file__),source_plan_SHA256=file_sha(phase/'QUIC-sparse-epoch-plan-before-01/plan-before.json'),forward_requirements_SHA256=file_sha(phase/'QUIC-sparse-forward-plan-before-01/summary.json'),remaining_target_reverse_packets=117,all_remaining_packets_independently_AEAD_authenticated=True,wire_frame_core_occurrences={'STREAM':86,'ACK':38,'PING':3},existing_native_HTTP3_frames_with_exact_captured_bytes_and_parent_ranges=exact_h3,source_stream_intervals_SHA256=file_sha(out/'source-stream-intervals.json'),target_client_streams=sum(r['direction']==0 for r in target),target_client_streams_missing_prefix_or_internal_bytes=sum(r['direction']==0 and(r['missing_prefix']or bool(r['internal_holes']))for r in target),newly_authenticated_packets_do_not_fill_omitted_STREAM_intervals=True,HTTP3_dynamic_QPACK_headers_request_response_context_and_transaction_ids_not_fully_certified=True,full_application_recovery_claim=False)
    write(out/'summary.json',summary);print(json.dumps({'summary':str(out/'summary.json'),'SHA256':file_sha(out/'summary.json'),'selected':117,'HTTP3_exact':4,'client_streams':summary['target_client_streams'],'client_holes':summary['target_client_streams_missing_prefix_or_internal_bytes']}))
if __name__=='__main__':run()
