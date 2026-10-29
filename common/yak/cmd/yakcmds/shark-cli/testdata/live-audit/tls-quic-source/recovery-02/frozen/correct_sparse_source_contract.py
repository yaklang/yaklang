"""Explicit source-derived correction of an overstrong frozen KU bridge.

The first plan wrongly froze a failed PDU's absent CompletedIn and transaction0.
Complete authenticated packets acquire their sole physical completion witness;
new client-initiated bidi STREAMs contain an observed contiguous HEADERS frame.
The anchors below are STREAM/request-source anchors. Dynamic QPACK header
semantics are not inferred. No candidate/new native export is read here.
"""
import base64,collections,copy,json,sys
from pathlib import Path
import xml.etree.ElementTree as ET
from recovery_probe import keys,file_sha,sha,write
from quic_sparse_epoch import unprotect,values,num,vint
from sparse_epoch_packet_audit import literal_frames

def run():
    p=Path(__file__).parent.parent;out=p/'QUIC-sparse-forward-source-correction-02'
    audit=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\yak\cmd\yakcmds\shark-cli\testdata\live-audit')
    sys.path.insert(0,str(audit));from vxlan_endpoint_projection_audit import capture_records
    original_plan=p/'QUIC-sparse-forward-plan-before-01'
    frozen=json.loads((original_plan/'summary.json').read_text('utf8'))
    requirements=json.loads((original_plan/'recovery-core-requirements.json').read_text('utf8'))
    source_plan=p/'QUIC-sparse-epoch-plan-before-01';plan=json.loads((source_plan/'plan-before.json').read_text('utf8'))
    rows=[json.loads(x)for x in(source_plan/'packet-source-plan.jsonl').read_text('utf8').splitlines()]
    records=list(capture_records(Path(plan['source_capture'])))
    assert file_sha(plan['source_capture'])==plan['source_capture_SHA256']
    secret=keys(plan['cases'][0]['keylog']);ws=ET.parse(p/'TLS-QUIC-reference25-01/quic-original/reference.pdml').getroot();randoms={};suites={}
    for packet in ws:
        cid=num(packet,'quic.connection.number')
        for f in values(packet,'tls.handshake.random'):
            rnd=bytes.fromhex(f.get('show').replace(':',''))
            if('CLIENT_TRAFFIC_SECRET_0',rnd)in secret:randoms[cid]=rnd
        for f in values(packet,'tls.handshake.ciphersuite'):suites[cid]=int(f.get('show'),0)
    oldpath=p/'QUIC-sparse-epoch-actual26-01/all-client-middle-epoch-omitted/events.jsonl'
    assert file_sha(oldpath)==frozen['baseline_export_SHA256']
    old=[json.loads(x)for x in oldpath.read_text('utf8').splitlines()]
    index={r['source_frame']:r['number']for r in plan['ledger']};retained=[r for r in rows if r['source_frame']in index]
    assert len(old)==len(retained)==1880
    assemblies={};anchors={};anchor_proofs={};packet_state={};seenPN=set()
    for e,r in zip(old,retained):
        frame=index[r['source_frame']];raw=records[r['source_frame']-1]['wire'][r['packet_offset']:r['packet_end']]
        assert sha(raw)==r['packet_SHA256']and base64.b64decode(e['raw'])==raw
        assert e['source_bytes']['PacketRefs']==[{'Number':frame,'Domain':{'Section':0,'Interface':0,'Encapsulation':''}}]
        if not r.get('independently_AEAD_authenticated'):continue
        label=('SERVER'if r['direction']else'CLIENT')+'_TRAFFIC_SECRET_0'
        w,phase,width=unprotect(raw,r['packet_number'],bytes.fromhex(r['destination_cid']),secret[label,randoms[r['oracle_connection']]],suites[r['oracle_connection']],r['epoch'])
        assert sha(w)==r['plaintext_SHA256'];core=literal_frames(w)
        identity=(r['oracle_connection'],r['direction'],r['packet_number']);duplicate=identity in seenPN;seenPN.add(identity)
        stream_states=[]
        for ordinal,fr in enumerate(core):
            if fr['Frame Type']!='STREAM':continue
            sid=fr['Stream ID'];key=(r['oracle_connection'],r['direction'],sid)
            state=assemblies.setdefault(key,dict(cells={},contiguous=0,FIN=None,done=False,first_source=None))
            before=state['contiguous'];data=base64.b64decode(fr['Stream Data'])
            assert len(data)==fr['Length']
            if state['first_source']is None:state['first_source']={'PDU':e['id'],'frame':frame,'source_frame':r['source_frame'],'offset':fr['Offset']}
            for off,b in enumerate(data,fr['Offset']):
                assert off not in state['cells']or state['cells'][off]==b;state['cells'][off]=b
            if fr['FIN']:
                end=fr['Offset']+len(data);assert state['FIN']is None or state['FIN']==end;state['FIN']=end
            while state['contiguous']in state['cells']:state['contiguous']+=1
            complete=state['FIN']is not None and state['contiguous']==state['FIN']
            final_delivery=complete and not state['done'];state['done']=complete
            eligible=not duplicate and(state['contiguous']>before or final_delivery)
            anchor_key=(r['oracle_connection'],sid)
            if eligible and r['direction']==0 and sid%4==0 and anchor_key not in anchors:
                prefix=bytes(state['cells'][i]for i in range(state['contiguous']))
                typ,at=vint(prefix,0);length,payload=vint(prefix,at)
                assert typ==1 and payload+length<=len(prefix),'anchor requires an actual complete first HEADERS wire frame'
                assert state['first_source']['offset']==0
                anchors[anchor_key]=state['first_source']['PDU']
                anchor_proofs[anchor_key]=dict(connection=r['oracle_connection'],stream_id=sid,client_initiated_bidi=True,first_source=state['first_source'],anchor_PDU=anchors[anchor_key],first_HTTP3_frame_type=typ,HEADERS_length=length,HEADERS_wire_SHA256=sha(prefix[:payload+length]),HEADERS_varint_header_hex=prefix[:payload].hex(),QPACK_encoded_required_insert_count=prefix[payload],QPACK_prefix_hex=prefix[payload:payload+2].hex(),dynamic_header_semantics_certified=False)
            anchor=anchors.get(anchor_key,0)if eligible and sid%4==0 else 0
            other=assemblies.get((r['oracle_connection'],1-r['direction'],sid),{})
            closed=bool(state['done']and other.get('done'))
            stream_states.append(dict(ordinal=ordinal,stream_id=sid,eligible=eligible,anchor=anchor,both_observed_directions_complete=closed,contiguous=state['contiguous'],observed_FIN=state['FIN']))
        nonzero={x['anchor']for x in stream_states if x['anchor']}
        transaction=next(iter(nonzero))if len(nonzero)==1 else 0
        packet_state[e['id']]=dict(direction=r['direction'],transaction=transaction,response_to=transaction if r['direction']==1 else 0,streams=stream_states)
    assert anchors[0,16]==299 and anchors[0,20]==408
    assert all(assemblies[0,0,sid]['first_source']['offset']==0 for sid in(16,20))
    assert assemblies[0,0,10]['contiguous']==1 and any(off>=4 for off in assemblies[0,0,10]['cells'])
    # Correct only source completion and source-derived STREAM anchors. The old
    # plan, failed bridge and all original baseline objects remain untouched.
    corrected=copy.deepcopy(requirements);changes=[]
    for r in corrected:
        before=copy.deepcopy(r['immutable_event_keys']);facts=packet_state[r['PDU']]
        src=r['immutable_event_keys']['source_bytes'];assert'CompletedIn'not in src and len(src['PacketRefs'])==1
        src['CompletedIn']=copy.deepcopy(src['PacketRefs'][0])
        r['immutable_event_keys']['transaction']=facts['transaction']
        r['immutable_event_keys']['response_to']=facts['response_to']
        changes.append(dict(PDU=r['PDU'],completion_source=src['CompletedIn'],source_STREAM_transaction=facts['transaction'],source_STREAM_response_to=facts['response_to'],old_source_and_transaction=before))
    expected=[];modified=[]
    byid={e['id']:e for e in old}
    for ident in frozen['target_remaining_events_requiring_explicit_state_review']:
        e=copy.deepcopy(byid[ident]);facts=packet_state[ident]
        native_streams=[f for f in e['session']['Frames']if f['Frame Type']=='STREAM']
        assert len(native_streams)==len(facts['streams'])
        for fr,context in zip(native_streams,facts['streams']):
            assert fr['Stream ID']==context['stream_id']
            if 'Request PDU ID'in fr:
                assert context['eligible']
                fr['Request PDU ID']=fr['Transaction ID']=context['anchor']
            if 'HTTP3 Stream State'in fr:
                fr['HTTP3 Stream State']='closed'if context['both_observed_directions_complete']else'half-closed'
                if 'HTTP3 Stream State'in e['session']:e['session']['HTTP3 Stream State']=fr['HTTP3 Stream State']
        e['transaction'],e['response_to']=facts['transaction'],facts['response_to']
        if facts['response_to']:
            vals=e['field_values']['values'];event=[v for v in vals if v['origin']=='event'];other=[v for v in vals if v['origin']!='event']
            assert [v['name']for v in event]==sorted(v['name']for v in event)and all(v['name']!='pdu.response_to'for v in event)
            event.append({'name':'pdu.response_to','value':facts['response_to'],'exact_value':str(facts['response_to']),'origin':'event'})
            event.sort(key=lambda v:v['name']);e['field_values']['values']=event+other
        if e!=byid[ident]:modified.append(ident)
        expected.append(e)
    assert len(expected)==117 and len(modified)==53
    out.mkdir(exist_ok=False)
    write(out/'recovery-core-requirements.json',corrected);write(out/'source-correction-details.json',changes)
    write(out/'remaining-target-whole-expected.json',expected)
    write(out/'STREAM-anchor-source-proofs.json',[anchor_proofs[0,16],anchor_proofs[0,20]])
    write(out/'packet-source-STREAM-state.json',packet_state)
    api=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\pcapx\pcaputil\protocol_field_registry.go')
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),original_frozen_plan_SHA256=file_sha(original_plan/'summary.json'),original_requirements_SHA256=file_sha(original_plan/'recovery-core-requirements.json'),original_failure_retained=True,baseline_actual_export_SHA256=file_sha(oldpath),source_plan_SHA256=file_sha(source_plan/'plan-before.json'),corrected_source_completion_occurrences=105,corrected_transaction_source_anchors=[299,408],corrected_core_requirements_SHA256=file_sha(out/'recovery-core-requirements.json'),source_correction_details_SHA256=file_sha(out/'source-correction-details.json'),whole_expected_remaining_target_events=117,whole_expected_changed_remaining_events=53,whole_expected_literal_remaining_events=64,remaining_whole_expected_SHA256=file_sha(out/'remaining-target-whole-expected.json'),anchor_source_proofs_SHA256=file_sha(out/'STREAM-anchor-source-proofs.json'),source_packet_state_SHA256=file_sha(out/'packet-source-STREAM-state.json'),event_projection_API_SHA256=file_sha(api),candidate_output_read=False,QPACK_headers_or_full_request_semantics_claim=False,missing_QPACK_decoder_stream10_range=[1,4],missing_prefix_request_fabrication=False)
    write(out/'summary.json',summary);print(json.dumps({'summary':str(out/'summary.json'),'SHA256':file_sha(out/'summary.json'),'target117_expected_changed':53,'candidate_output_read':False}))
if __name__=='__main__':run()
