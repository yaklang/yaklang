"""Verify actual TLS whole-record omissions without inventing TCP recovery.

All emitted complete TLS records bind literal original TCP record occurrences.
Every HTTP plaintext digest binds both independent AEAD and actual peer receipt.
The missing TCP range is unrepaired; later original AEAD counters are contextual
evidence, not a grant to skip ciphertext or to deliver HTTP through a hole.
"""
import argparse,base64,collections,copy,csv,json,sys
from decimal import Decimal
from pathlib import Path
from recovery_probe import file_sha,sha,keys,tls_clear_records,write
from sparse_epoch_packet_audit import verify_receipt

def run(a):
    sys.path.insert(0,str(a.audit))
    from tls_resumption_audit import wire_connections
    from capture_clock_wire_audit import physical,ip
    from vxlan_endpoint_projection_audit import capture_records
    plan=json.loads((a.plan/'plan-before.json').read_text('utf8'))
    root=Path(plan['source_capture']).parent;peer=json.loads((root/'manifest.json').read_text('utf8'))
    source,conns,owners,tls,hellos,total=wire_connections(Path(plan['source_capture']),peer)
    source_metadata=list(capture_records(Path(plan['source_capture'])))
    assert len(source)==226 and len(conns)==8 and not peer['failures']
    assert file_sha(plan['source_capture'])==plan['source_capture_SHA256']==peer['capture_SHA256']
    entries=collections.defaultdict(list)
    for (key,digest),vs in tls.items():
        for e in vs:assert sha(e['wire'])==digest;entries[key].append(e)
    plaintext=tls_clear_records(entries,hellos,keys(root/'session.keys'))
    assert len(plaintext)==136 and sum(e.get('authenticated')is True for e in plaintext)==94
    clear={(e['connection'],e['direction'],e['record_SHA256']):e for e in plaintext}
    a.out.mkdir(exist_ok=False);checks=[]
    for c in plan['cases']:
        cap=Path(c['capture']);target=c['target_original_SYN_generation'];folder=a.actual/c['case']
        proof=verify_receipt(folder,cap,c['keylog']);assert proof['plan_SHA256']==file_sha(a.plan/'plan-before.json')
        doc=json.loads((cap.parent/'manifest.json').read_text('utf8'))
        assert file_sha(cap.parent/'manifest.json')==c['source_manifest_SHA256']
        ledger=doc['ledger'];physical_rows=physical(cap);index={v['source_frame']:v['number']for v in ledger}
        assert len(index)==len(ledger)==len(physical_rows)==225
        metadata=list(capture_records(cap))
        for r,original,meta in zip(physical_rows,ledger,metadata):
            old=source[original['source_frame']-1]
            oldmeta=source_metadata[original['source_frame']-1]
            assert old['wire']==r['wire']==meta['wire']and old['time_ns']==r['time_ns']==meta['time_ns']
            assert oldmeta['original_length']==meta['original_length']==meta['captured_length']==len(r['wire'])
            assert sha(r['wire'])==original['source_wire_SHA256']
        omitted=c['omitted_source_frame'];start,end=c['omitted_TCP_sequence_start'],c['omitted_TCP_sequence_end']
        for n in index:
            if owners.get(n)!=(target,1):continue
            _,_,tcp=ip(source[n-1]);off=(tcp[12]>>4)*4;body=tcp[off:];seq=int.from_bytes(tcp[4:8],'big')
            assert not body or not(seq<end and seq+len(body)>start),'no source retransmission fills omitted range'
        expected={}
        for key,records in entries.items():
            for record in records:
                if key==(target,1) and min(record['refs'])>=omitted:continue
                expected[key[0],key[1],sha(record['wire'])]=record
        assert len(expected)==132
        native=[json.loads(x)for x in(folder/'events.jsonl').read_text('utf8').splitlines()]
        byid={e['id']:e for e in native};assert len(native)==len(byid)==180
        seen=collections.Counter();http=collections.Counter();header_fields=0;incomplete=[];parent_checks=0
        for e in native:
            if e['protocol']not in ('tls','http'):assert e['status']=='unrecognized' and e['transport']=='udp';continue
            cid,direction=e['flow'],e['direction'];assert cid in conns and direction in(0,1)
            if e['status']=='incomplete':
                assert(cid,direction)==(target,1) and e['protocol']=='tls'
                assert 'unfilled sequence gap' in e['summary']and not e['source_bytes'].get('PacketRefs')
                assert e['source_bytes']['Kind']=='reassembled'and not e['raw']
                incomplete.append({'PDU':e['id'],'flow':cid,'direction':direction,'summary':e['summary']});continue
            assert e['status']=='decoded';raw=base64.b64decode(e['raw']);src=e['source_bytes']
            assert src['SHA256']==sha(raw)and e['domain']=={'Section':0,'Interface':0,'Encapsulation':''}
            refs=src['PacketRefs'];assert refs and all(r['Domain']==e['domain']for r in refs)
            reverse={v:k for k,v in index.items()}
            assert all(owners[reverse[r['Number']]]==(cid,direction)for r in refs)
            if e['protocol']=='tls':
                key=(cid,direction,sha(raw));assert key in expected
                rec=expected[key];assert raw==rec['wire']and[r['Number']for r in refs]==[index[n]for n in rec['refs']]
                seen[key]+=1;fields=collections.defaultdict(list)
                for f in e['field_values']['values']:fields[f['name']].append(f['value'])
                for name,v in [('tls.record.wire_type',raw[0]),('tls.record.version',int.from_bytes(raw[1:3],'big')),('tls.record.length',len(raw)-5)]:assert fields[name]==[v];header_fields+=1
                assert(e['session'].get('Authentication Verified')is True)==(clear[key].get('authenticated')is True)
            else:
                assert src['Kind']=='decrypted' and src['ParentPDUs'] and len(src['ParentPDUs'])==1
                p=byid[src['ParentPDUs'][0]];pk=(cid,direction,sha(base64.b64decode(p['raw'])))
                assert p['protocol']=='tls'and p['flow']==cid and p['direction']==direction and p['session']['Authentication Verified']is True
                assert sha(raw)==clear[pk]['plaintext_SHA256']and len(raw)==clear[pk]['plaintext_length']
                assert refs==p['source_bytes']['PacketRefs'];parent_checks+=1
                conn=conns[cid];opkey='request_SHA256'if direction==0 else'response_SHA256'
                assert any(op['profile']==conn['profile']and op['generation']==conn['generation']and op[opkey]==sha(raw)for op in peer['operations'])
                http[cid,direction,sha(raw)]+=1
        assert seen==collections.Counter({key:1 for key in expected})and len(incomplete)==1
        expected_http=collections.Counter()
        for cid,conn in conns.items():
            for op in peer['operations']:
                if op['profile']!=conn['profile']or op['generation']!=conn['generation']:continue
                expected_http[cid,0,op['request_SHA256']]+=1
                if cid!=target:expected_http[cid,1,op['response_SHA256']]+=1
        assert http==expected_http and sum(http.values())==45
        reference=a.reference/c['profile'];producer=json.loads((reference/'producer.json').read_text('utf8'))
        assert producer['exit_code']==0 and producer['capture_SHA256']==file_sha(cap)and producer['keylog_SHA256']==file_sha(c['keylog'])
        assert producer['export_SHA256']==file_sha(reference/'reference.tsv')
        ws=list(csv.DictReader((reference/'reference.tsv').read_text('utf8').splitlines(),delimiter='\t'))
        assert len(ws)==225;wsreq=[];wsresponses=0;loss=[]
        for n,(r,wire)in enumerate(zip(ws,physical_rows),1):
            assert int(r['frame.number'])==n and int(r['frame.cap_len'])==int(r['frame.len'])==len(wire['wire'])
            assert int(Decimal(r['frame.time_epoch'])*10**9)==wire['time_ns']
            if r['tcp.stream']:
                original=reverse[n];assert int(r['tcp.stream'])+1==owners[original][0]
            if r['http.request.method']:
                assert all(v=='GET'for v in r['http.request.method'].split(','))
                assert len(r['http.request.method'].split(','))==len(r['http.request.uri'].split(','))
                wsreq.extend(r['http.request.uri'].split(','))
            if r['http.response.code']:assert all(v=='200'for v in r['http.response.code'].split(','));wsresponses+=len(r['http.response.code'].split(','))
            if r['tcp.analysis.lost_segment']:loss.append(n)
            if r['tcp.stream']==str(target-1):assert not r['http.response.code']
        req=[f"/{o['profile']}/{o['generation']}/{o['index']}"for o in peer['operations']]
        assert collections.Counter(wsreq)==collections.Counter(req)and len(wsreq)==24 and wsresponses==21 and loss
        target_req=sum(n for(k,d,h),n in http.items()if k==target and d==0)
        assert target_req==3
        checks.append(dict(case=c['case'],actual_source=proof['source'],producer_SHA256=file_sha(folder/'producer.json'),reference_producer_SHA256=file_sha(reference/'producer.json'),literal_retained_physical_frames=225,complete_source_TLS_records=132,exact_TLS_header_fields=header_fields,authenticated_HTTP_plaintexts_matching_actual_peers=45,HTTP_parent_sources_exact=parent_checks,reverse_requests_preserved=target_req,source_valid_later_records_blocked_by_TCP_hole=len(c['retained_later_records_with_independent_AEAD_authentication']),native_gap=incomplete[0],WS_requests=24,WS_responses=21,WS_gap_frames=loss,missing_range_not_repaired=True,record_boundary_recovery_or_random_ciphertext_scanning_claim=False))
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),plan_SHA256=file_sha(a.plan/'plan-before.json'),checks=checks,full_TLS_feature_parity_claim=False)
    write(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':file_sha(a.out/'summary.json'),'checks':checks}))
if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('plan','actual','audit','reference','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
