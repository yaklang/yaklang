"""Certify exact owned TLS observation controls and generation isolation.

No network loss is claimed. Retained frames/bytes/times and declared omissions
or swaps are independently checked against the original owned capture. Every
unaffected native event is compared in full after only source-attested physical
frame, PDU and flow identifier translation. Plaintext requires an authenticated
TLS parent in its own original generation. Reference comparisons use messages,
not counts of frames carrying HTTP; reordered reference TCP reassembly is an
explicit separate preference. Missing-record recovery remains a common limit.
"""
import argparse,base64,collections,copy,csv,hashlib,json
from pathlib import Path
from tls_resumption_audit import wire_connections,rows,sha,audit as audit_baseline
from capture_clock_wire_audit import physical
from owned_quic_clock_audit import vals,number
from owned_tls_clock_audit import raw_records


def event_key(e,numbers):
    refs=tuple(numbers[r['Number']]for r in(e['source_bytes'].get('PacketRefs')or[]))
    raw=e['raw']
    if raw is None:assert e['status']==e['completeness']=='incomplete'and not e['fields'];raw=''
    return(e['protocol'],e['status'],e['transport'],e['direction'],refs,sha(base64.b64decode(raw,validate=True)))


def normalize(e,ids,flows,numbers):
    e=copy.deepcopy(e)
    original_id,original_response=e['id'],e.get('response_to',0)
    def ident(v,m):assert type(v)is int;return m[v]if v else 0
    def source(s):
        for key in['ParentPDU','ParentPDUs']:
            if key in s and s[key]is not None:s[key]=[ident(v,ids)for v in s[key]]if key=='ParentPDUs'else ident(s[key],ids)
        for r in(s.get('PacketRefs')or[]):assert type(r['Number'])is int;r['Number']=numbers[r['Number']]
        if s.get('CompletedIn'):r=s['CompletedIn'];r['Number']=numbers[r['Number']]
    e['id']=ident(e['id'],ids)
    for key in['flow','tcp_flow_id']:
        if key in e:e[key]=ident(e[key],flows)
    if e.get('response_to'):e['response_to']=ident(e['response_to'],ids)
    if e.get('transaction'):
        assert e['protocol']=='http'and e['transaction']==(original_id if e['direction']==0 else original_response),'only proven HTTP request-PDU transaction aliases can be translated'
        e['transaction']=ident(e['transaction'],ids)
    source(e['source_bytes']);fv=e.get('field_values')
    if fv:
        fv['pdu']=ident(fv['pdu'],ids);source(fv['source'])
        for value in(fv.get('values')or[]):
            key=value['name'];mapping=ids if key in['pdu.id','pdu.response_to']else flows if key in['flow.id','tcp.flow.id']else numbers if key=='pdu.completed_in'else None
            if mapping is None:continue
            original=value['value'];translated=ident(original,mapping);value['value']=translated
            if'exact_value'in value:assert value['exact_value']==str(original);value['exact_value']=str(translated)
    return json.dumps(e,sort_keys=True,separators=(',',':'))


def audit(source_capture,source_manifest,baseline,case,replay,reorder_reference=None):
    baseline_proof=audit_baseline(source_capture,baseline/'events.jsonl',baseline/'yak.jsonl',baseline/'flows.json',source_manifest,baseline/'oracle.json',baseline/'labels.tsv')
    peer=json.loads(source_manifest.read_text('utf8'));original,connections,owners,wire,hellos,total=wire_connections(source_capture,peer)
    doc=json.loads((case/'manifest.json').read_text('utf8'));actual=physical(case/'observation.pcap');ledger=doc['ledger'];name=doc['case'];profile=doc['profile'];target=next(cid for cid,c in connections.items()if(c['profile'],c['generation'])==(profile,1))
    assert doc['source_capture_SHA256']==sha(source_capture.read_bytes())and doc['source_manifest_SHA256']==sha(source_manifest.read_bytes())
    assert doc['target_original_connection']==target and doc['target_generation']==1 and doc['network_fault_injection']is False
    assert doc['capture_SHA256']==sha((case/'observation.pcap').read_bytes())and len(actual)==len(ledger)==doc['physical_records']
    base_reference=json.loads((baseline/'oracle.json').read_text('utf8'));source_layers={i:r['_source']['layers']for i,r in enumerate(base_reference,1)}
    target_frames=connections[target]['frames'];client_hello=[n for n in target_frames if 1 in map(number,vals(source_layers[n],'tls.handshake.type'))];server_hello=[n for n in target_frames if 2 in map(number,vals(source_layers[n],'tls.handshake.type'))]
    requests=[n for n in target_frames if vals(source_layers[n],'http.request.method')];responses=[n for n in target_frames if vals(source_layers[n],'http.response.code')]
    assert len(client_hello)==len(server_hello)==1 and len(requests)==len(responses)==3
    mode=name[len(profile)+1:]
    expected_omitted={'missing-ClientHello':set(client_hello),'missing-ServerHello':set(server_hello),'midstream-resumed':{n for n in target_frames if n<requests[0]},'missing-one-server-record':{responses[0]},'reordered-server-records':set(),'capture-starts-midstream':set(range(1,requests[0]))}[mode]
    expected_order=[n for n in range(1,len(original)+1)if n not in expected_omitted]
    if mode=='reordered-server-records':a,b=map(expected_order.index,responses[:2]);expected_order[a],expected_order[b]=expected_order[b],expected_order[a]
    assert[entry['source_frame']for entry in ledger]==expected_order and{entry['source_frame']for entry in doc['omitted_records']}==expected_omitted
    numbers={};kept=set(expected_order)
    for n,(r,entry)in enumerate(zip(actual,ledger),1):
        old=entry['source_frame'];assert type(old)is int and type(entry['number'])is int and entry['number']==n;assert r==original[old-1]
        assert entry['source_time_ns']==r['time_ns']and entry['source_wire_SHA256']==sha(r['wire'])and entry['source_section']==r['section']and entry['source_interface']==r['interface'];numbers[n]=old
    for entry in doc['omitted_records']:
        old=original[entry['source_frame']-1];assert entry['source_time_ns']==old['time_ns']and entry['source_wire_SHA256']==sha(old['wire'])and entry['reason']==mode
    native=rows(replay/'events.jsonl');base=rows(baseline/'events.jsonl');identical={n:n for n in range(1,len(original)+1)}
    base_keys={event_key(e,identical):e for e in base};assert len(base_keys)==len(base)
    current_keys={event_key(e,numbers):e for e in native};assert len(current_keys)==len(native)
    ids={e['id']:base_keys[key]['id']for key,e in current_keys.items()if key in base_keys};flowmap={0:0};flow_packets={};target_skipped=[];ambiguous_namespace=[]
    flowdoc=json.loads((replay/'flows.json').read_text('utf8'));assert not flowdoc['error']
    for p in flowdoc['packets']:
        assert type(p['flow'])is int and type(p['direction'])is int and len(p['refs'])==1
        n=p['refs'][0]['Number'];assert type(n)is int and n not in flow_packets;old=numbers[n];cid,direction=owners[old];assert p['direction']==direction
        assert p['refs'][0]['Domain']==dict(Section=0,Interface=0,Encapsulation='')
        mapped=flowmap.setdefault(p['flow'],cid)
        if mapped!=cid:
            assert mode=='midstream-resumed'and cid==target and mapped==target-1 and connections[mapped]['profile']==profile and connections[mapped]['generation']==0,'unexplained cross-generation native namespace'
            ambiguous_namespace.append(dict(frame=n,source_frame=old,native_flow=p['flow'],original_closed_generation=mapped,actual_peer_generation=cid))
        flow_packets[n]=cid
    for n,old in numbers.items():
        if old not in owners:continue
        cid,direction=owners[old]
        if cid!=target:assert flow_packets.get(n)==cid
        elif n not in flow_packets:target_skipped.append(old)
    assert len(flowmap)-1==len(flowdoc['flows'])
    for f in flowdoc['flows']:
        c=connections[flowmap[f['flow']]];assert f['source']==f'127.0.0.1:{c["client_port"]}'and f['destination']==f'127.0.0.1:{c["server_port"]}'
        assert f['half_open']is(mode=='capture-starts-midstream'and flowmap[f['flow']]==target)
    expected_unaffected={}
    for key,e in base_keys.items():
        refs=[r['Number']for r in(e['source_bytes'].get('PacketRefs')or[])];cids={owners[n][0]for n in refs if n in owners}
        if cids=={target}or not any(n in kept for n in refs):continue
        assert all(n in kept for n in refs)and key in current_keys;expected_unaffected[key]=e
    native_by_id={e['id']:e for e in native};base_http=collections.Counter();current_http=collections.Counter();native_target=collections.Counter();unaffected=0;target_field_checks=0;target_flow_only_diagnostics=[]
    identity_ids={e['id']:e['id']for e in base};identity_flows={0:0,**{cid:cid for cid in connections}}
    for key,e in current_keys.items():
        refs=e['source_bytes'].get('PacketRefs')or[];cids={owners[numbers[r['Number']]][0]for r in refs if numbers[r['Number']]in owners}
        assert all(type(r['Number'])is int and all(type(r['Domain'][k])is int for k in['Section','Interface'])for r in refs)
        assert len(cids)<=1
        cid=next(iter(cids))if cids else None
        if cid is None and e['transport']=='tcp':
            assert e['status']==e['completeness']=='incomplete'and e['raw']is None and e['fields']is None and e['fields_error']=='protocol parser: event is incomplete, not an exact message'
            assert type(e['tcp_flow_id'])is int and flowmap[e['tcp_flow_id']]==target
            cid=target;target_flow_only_diagnostics.append(e['id'])
        if cid is not None:
            assert type(e['direction'])is int and type(e['tcp_flow_id'])is int and flowmap[e['tcp_flow_id']]==cid
            assert all(owners[numbers[r['Number']]]==(cid,e['direction'])for r in refs)
        if cid!=target:
            assert key in expected_unaffected and normalize(e,ids,flowmap,numbers)==normalize(expected_unaffected[key],identity_ids,identity_flows,identical);unaffected+=1
        else:native_target[e['protocol']+':'+e['status']]+=1
        if cid==target and mode in['missing-ClientHello','missing-ServerHello','capture-starts-midstream']:
            assert e['protocol']=='tls'and(e.get('session')or{}).get('Authentication Verified')is not True,'a missing Hello cannot borrow old randoms, keys or authenticated identity'
        if e['protocol']=='http':
            assert e['status']=='decoded'and e['source_bytes']['Kind']=='decrypted'and e['source_bytes']['ParentPDUs']
            for pdu in e['source_bytes']['ParentPDUs']:
                parent=native_by_id[pdu];assert parent['protocol']=='tls'and parent['tcp_flow_id']==e['tcp_flow_id']and parent['direction']==e['direction']and parent['session']['Authentication Verified']is True
            assert key in base_keys and e['fields']==base_keys[key]['fields'];target_field_checks+=int(cid==target)
            if e['direction']==1:
                request=native_by_id[e['response_to']];assert request['protocol']=='http'and request['direction']==0 and request['tcp_flow_id']==e['tcp_flow_id']
                assert e['transaction']==e['response_to']
            else:assert e['transaction']==e['id']
            current_http[cid,e['direction'],sha(base64.b64decode(e['raw']))]+=1
    assert unaffected==len(expected_unaffected)
    for e in base:
        if e['protocol']!='http':continue
        refs=e['source_bytes']['PacketRefs'];cid=e['tcp_flow_id']
        if all(r['Number']in kept for r in refs)and(cid!=target or mode=='reordered-server-records'or mode=='missing-one-server-record'and e['direction']==0):base_http[cid,e['direction'],sha(base64.b64decode(e['raw']))]+=1
    assert current_http==base_http,'only explicitly unavailable target plaintext can be absent'
    reference_dir=reorder_reference if mode=='reordered-server-records'else replay
    assert reference_dir is not None
    if mode=='reordered-server-records':
        producer=json.loads((reference_dir/'oracle-producer.json').read_text('utf8'));assert'tcp.reassemble_out_of_order:TRUE'in producer['arguments']
    reference=json.loads((reference_dir/'oracle.json').read_text('utf8'));assert len(reference)==len(actual)
    reference_wire=collections.Counter();base_wire=collections.Counter();reference_http=collections.Counter();reference_labels={}
    for n,entry in enumerate(reference,1):
        layers=entry['_source']['layers'];assert number(layers['frame']['frame.number'])==n and bytes.fromhex(layers['frame_raw'][0])==actual[n-1]['wire'];old=numbers[n]
        if old not in owners:continue
        cid,direction=owners[old]
        if cid!=target:
            for wire_record in raw_records(layers):reference_wire[cid,direction,sha(wire_record)]+=1
            for wire_record in raw_records(source_layers[old]):base_wire[cid,direction,sha(wire_record)]+=1
        methods=vals(layers,'http.request.method');uris=vals(layers,'http.request.uri')
        if methods:
            assert direction==0 and len(methods)==len(uris)
            for method,uri in zip(methods,uris):
                assert method=='GET';c=connections[cid];parts=uri.split('/');assert parts[1]==c['profile']and int(parts[2])==c['generation'];op=next(o for o in peer['operations']if(o['profile'],o['generation'],o['index'])==(parts[1],int(parts[2]),int(parts[3])));reference_http[cid,0,op['request_SHA256']]+=1
        bodies=list(raw_records(layers,'http.file_data_raw'));codes=vals(layers,'http.response.code');assert len(codes)==len(bodies)
        for code,body in zip(codes,bodies):
            assert direction==1 and number(code)==200;c=connections[cid];op=next(o for o in peer['operations']if(o['profile'],o['generation'],o['body_SHA256'])==(c['profile'],c['generation'],sha(body)));reference_http[cid,1,op['response_SHA256']]+=1
    assert reference_wire==base_wire and reference_http==current_http
    cli_rows=[r for r in rows(replay/'yak.jsonl')if r.get('record_type')!='protocol_publication'];assert len(cli_rows)==len(actual)
    base_cli={r['number']:r for r in rows(baseline/'yak.jsonl')if r.get('record_type')!='protocol_publication'}
    target_display_differences=[];closed_display_corrections=[]
    for n,row in enumerate(cli_rows,1):
        old=numbers[n];assert type(row['number'])is int and row['number']==n and row['captured_length']==row['length']==len(actual[n-1]['wire'])
        if old in owners and owners[old][0]==target:
            if mode=='midstream-resumed'and(row.get('connection_protocol')or row.get('connection_stack')or row.get('stream_id')):
                target_display_differences.append(dict(frame=n,source_frame=old,connection_protocol=row.get('connection_protocol'),tcp_flow_id=row.get('tcp_flow_id'),stream_id=row.get('stream_id'),reason='unconfirmed post-close packet inherits old application context or page association'))
            continue
        before=base_cli[old]
        corrected=before.get('tcp_flow_state')=='closed-observation'and row.get('tcp_flow_state')=='closed-observation'and row.get('stream_id',0)==0 and not row.get('connection_protocol')and not row.get('connection_stack')and'after observed close; generation unconfirmed'in row['info']
        if corrected:
            assert old in owners and any(rst<old for rst in connections[owners[old][0]]['reset_frames'])
            assert row['protocol']=='TCP'and row['protocol_stack']==['Loopback','IPv4','TCP']and row['visibility']=='header-only'and row['protocol_status']=='context-required'
            assert row.get('tcp_flow_id')and flowmap[row['tcp_flow_id']]==owners[old][0]
            closed_display_corrections.append(dict(frame=n,source_frame=old,physical_TCP_flow=row['tcp_flow_id'],reason='closed native namespace retained as TCP evidence; old application/page identity removed'))
        for field in['protocol','transport','source','destination','source_port','destination_port','capture_domain','message_domain']+([]if corrected else['protocol_stack','protocol_status','visibility']):
            assert row.get(field)==before.get(field),(name,n,field)
    return dict(
        scope=__doc__,case=name,source_capture_SHA256=sha(source_capture.read_bytes()),
        capture_SHA256=sha((case/'observation.pcap').read_bytes()),
        control_manifest_SHA256=sha((case/'manifest.json').read_bytes()),
        SDK_SHA256=sha((replay/'events.jsonl').read_bytes()),CLI_SHA256=sha((replay/'yak.jsonl').read_bytes()),
        flows_SHA256=sha((replay/'flows.json').read_bytes()),reference_SHA256=sha((reference_dir/'oracle.json').read_bytes()),
        auditor_SHA256=sha(Path(__file__).read_bytes()),baseline_auditor_SHA256=baseline_proof['auditor_SHA256'],
        original_full_capture_baseline_independently_passed=True,
        literal_retained_packets=len(actual),literal_omitted_packets=len(expected_omitted),
        swapped_frames=responses[:2]if mode=='reordered-server-records'else None,
        unaffected_native_events_full_values_and_owned_references_exact=unaffected,
        native_flow_to_original_SYN_generation=flowmap,
        ambiguous_closed_tuple_namespace_associations=ambiguous_namespace,
        target_flow_only_diagnostics_without_packet_witness=target_flow_only_diagnostics,
        known_CLI_context_display_differences=target_display_differences,
        intentional_source_proven_closed_observation_display_corrections=closed_display_corrections,
        display_projection_correct=not target_display_differences,
        verification_result='failed: stale closed-generation application context'if target_display_differences else'passed within the stated observation scope',
        all_native_and_reference_HTTP_messages=len(list(current_http.elements())),
        all_native_plaintext_authenticated_in_own_generation=True,target_HTTP_detailed_field_checks=target_field_checks,
        target_native_observations=dict(native_target),target_frames_excluded_from_ambiguous_closed_tuple=target_skipped,
        reference_out_of_order_TCP_reassembly_enabled=mode=='reordered-server-records',
        missing_record_recovery_is_common_limit=mode=='missing-one-server-record',
        network_loss_claim=False,all_protocol_recovery_parity_claim=False)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['source-capture','source-manifest','baseline','case','replay','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--reorder-reference',type=Path);a=ap.parse_args()
    for p in vars(a).values():
        if p is not None:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    proof=audit(a.source_capture,a.source_manifest,a.baseline,a.case,a.replay,a.reorder_reference)
    with a.out.open('x',encoding='utf8')as out:json.dump(proof,out,indent=2);out.write('\n')
    print(json.dumps(dict(case=proof['case'],unaffected=proof['unaffected_native_events_full_values_and_owned_references_exact'],HTTP=proof['all_native_and_reference_HTTP_messages'])),flush=True)
