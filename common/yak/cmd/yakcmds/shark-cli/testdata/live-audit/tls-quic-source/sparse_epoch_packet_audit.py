"""Source-first QUIC packet/AEAD/frame witnesses for a missed whole key epoch.

The original packet plan predates candidate execution. This verifier recomputes
RFC 9001 short-packet AEAD and RFC 9000 wire frame fields independently. Neither
native labels nor Wireshark decryption success grants authentication. Plaintext
and secrets remain in memory; only digests and field witnesses are saved.
"""
import argparse, base64, collections, copy, hashlib, json, sys
import xml.etree.ElementTree as ET
from pathlib import Path
from quic_sparse_epoch import unprotect, values, num, vint
from recovery_probe import keys, file_sha, sha, write, guard


def literal_frames(w):
    out=[]; at=0
    def vi():
        nonlocal at
        n,at=vint(w,at); return n
    def take(n):
        nonlocal at
        assert 0<=n<=len(w)-at
        b=w[at:at+n]; at+=n; return base64.b64encode(b).decode('ascii')
    while at<len(w):
        typ=vi()
        if typ==0:
            n=1
            while at<len(w) and w[at]==0:at+=1;n+=1
            f={'Frame Type':'PADDING','Length':n}
        elif typ==1:f={'Frame Type':'PING'}
        elif typ in (2,3):
            largest,delay,count,first=vi(),vi(),vi(),vi()
            ranges=[{'Gap':0,'ACK Range':first}]
            ranges += [{'Gap':vi(),'ACK Range':vi()} for _ in range(count)]
            f={'Frame Type':'ACK' if typ==2 else 'ACK_ECN','Largest Acknowledged':largest,'ACK Delay':delay,'ACK Range Count':count,'ACK Ranges':ranges}
            if typ==3:f.update({'ECT0 Count':vi(),'ECT1 Count':vi(),'CE Count':vi()})
        elif typ==6:
            off,n=vi(),vi();f={'Frame Type':'CRYPTO','Offset':off,'Length':n,'Crypto Data':take(n)}
        elif 8<=typ<=15:
            sid=vi();off=vi() if typ&4 else 0;n=vi() if typ&2 else len(w)-at
            f={'Frame Type':'STREAM','Stream ID':sid,'Offset':off,'Length':n,'FIN':bool(typ&1),'Stream Data':take(n)}
        elif typ==24:
            seq,retire=vi(),vi();assert at<len(w);n=w[at];at+=1;assert 1<=n<=20
            f={'Frame Type':'NEW_CONNECTION_ID','Sequence':seq,'Retire Prior To':retire,'CID':take(n),'Reset Token':take(16)}
        elif typ==25:f={'Frame Type':'RETIRE_CONNECTION_ID','Sequence':vi()}
        elif typ in (26,27):f={'Frame Type':'PATH_CHALLENGE' if typ==26 else 'PATH_RESPONSE','Data':take(8)}
        elif typ==29:
            error=vi();n=vi();f={'Frame Type':'CONNECTION_CLOSE_APP','Error Code':error,'Reason':base64.b64decode(take(n)).decode('utf8')}
        elif typ==30:f={'Frame Type':'HANDSHAKE_DONE'}
        else:raise AssertionError(('uncovered source frame type',typ,at))
        out.append(f)
    assert at==len(w)
    return out


def exact_core(actual, expected):
    assert len(actual)==len(expected),('frame count',len(actual),len(expected))
    n=0
    for a,b in zip(actual,expected):
        for k,v in b.items():
            assert k in a and type(a[k]) is type(v) and a[k]==v,(k,a.get(k),v)
            n+=1
    return n


def native_binding(e,row,frame,wire):
    raw=base64.b64decode(e['raw']);assert raw==wire
    assert sha(raw)==row['packet_SHA256']
    domain={'Section':0,'Interface':0,'Encapsulation':''}
    assert e['domain']==domain and e['direction']==row['direction']
    assert e['source'].endswith(':'+str(row['source_port'])) and e['destination'].endswith(':'+str(row['destination_port']))
    source=e['source_bytes'];assert source['Kind']=='captured' and source['SHA256']==sha(raw)
    assert source['PacketRefs']==[{'Number':frame,'Domain':domain}]
    assert e['field_values']['source']==source


def verify_receipt(folder, expected_cap, expected_key):
    proof=json.loads((folder/'producer.json').read_text('utf8'))
    assert proof['exit_code']==0 and proof['capture_SHA256']==file_sha(expected_cap)
    assert proof['keylog_SHA256']==file_sha(expected_key)
    assert proof['export_SHA256']==file_sha(folder/'events.jsonl')
    assert proof['stderr_SHA256']==file_sha(folder/'stderr')
    binary=Path(proof['arguments'][0]);build_path=binary.parent/'events-receipt.json'
    build=json.loads(build_path.read_text('utf8'));assert build['exit_code']==0
    assert proof['build_receipt_SHA256']==file_sha(build_path)
    assert proof['source']==build['source'] and file_sha(binary)==proof['binary_SHA256']==build['binary_SHA256']
    return proof


def run(a):
    guard();sys.path.insert(0,str(a.audit))
    from vxlan_endpoint_projection_audit import capture_records
    plan=json.loads((a.plan/'plan-before.json').read_text('utf8'))
    assert file_sha(a.plan/'packet-source-plan.jsonl')==plan['source_packet_plan_SHA256']
    rows=[json.loads(x) for x in (a.plan/'packet-source-plan.jsonl').read_text('utf8').splitlines()]
    source=list(capture_records(Path(plan['source_capture'])))
    assert file_sha(plan['source_capture'])==plan['source_capture_SHA256']
    assert len(source)==plan['original_physical_frames']==2036 and len(rows)==2051
    secrets=keys(plan['cases'][0]['keylog']);original_ws=ET.parse(a.original_reference/'reference.pdml').getroot()
    assert file_sha(a.original_reference/'reference.pdml')==plan['source_reference_SHA256']
    randoms={};suites={};packet_expected={}
    for p in original_ws:
        cid=num(p,'quic.connection.number')
        for f in values(p,'tls.handshake.random'):
            r=bytes.fromhex(f.get('show').replace(':',''))
            if ('CLIENT_TRAFFIC_SECRET_0',r)in secrets:randoms[cid]=r
        for f in values(p,'tls.handshake.ciphersuite'):suites[cid]=int(f.get('show'),0)
    assert set(randoms)==set(suites)=={0,1,2}
    count=0
    for r in rows:
        raw=source[r['source_frame']-1]['wire'][r['packet_offset']:r['packet_end']]
        assert sha(raw)==r['packet_SHA256']
        if r.get('independently_AEAD_authenticated')is True:
            label=('SERVER' if r['direction'] else 'CLIENT')+'_TRAFFIC_SECRET_0'
            w,phase,width=unprotect(raw,r['packet_number'],bytes.fromhex(r['destination_cid']),secrets[label,randoms[r['oracle_connection']]],suites[r['oracle_connection']],r['epoch'])
            assert sha(w)==r['plaintext_SHA256'] and len(w)==r['plaintext_bytes']
            assert phase==r['key_phase'] and width==r['packet_number_bytes']
            packet_expected[r['source_frame'],r['packet_SHA256']]=literal_frames(w);count+=1
    assert count==2027
    a.out.mkdir(exist_ok=False)
    results=[];target=[]
    for case in plan['cases']:
        folder=a.actual/case['case'];proof=verify_receipt(folder,case['capture'],case['keylog'])
        assert proof['plan_SHA256']==file_sha(a.plan/'plan-before.json')
        events=[json.loads(x) for x in(folder/'events.jsonl').read_text('utf8').splitlines()]
        ledger=plan['ledger'] if case['case']=='all-client-middle-epoch-omitted' else [{'source_frame':i,'number':i}for i in range(1,len(source)+1)]
        index={r['source_frame']:r['number']for r in ledger};retained=[r for r in rows if r['source_frame']in index]
        assert len(retained)==len(events)
        caprows=list(capture_records(Path(case['capture'])))
        for r in ledger:
            x,y=source[r['source_frame']-1],caprows[r['number']-1]
            assert x['wire']==y['wire'] and x['time_ns']==y['time_ns'] and x['original_length']==y['original_length']
        wsdir=a.sparse_reference if case['case']=='all-client-middle-epoch-omitted' else a.original_reference
        wsp=json.loads((wsdir/'producer.json').read_text('utf8'));assert wsp['exit_code']==0 and wsp['capture_SHA256']==case['capture_SHA256'] and wsp['keylog_SHA256']==case['keylog_SHA256'] and file_sha(wsdir/'reference.pdml')==wsp['export_SHA256']
        packets=ET.parse(wsdir/'reference.pdml').getroot();assert len(packets)==len(caprows)
        flow_to_conn={};counts=collections.Counter();fields=0;seen=set();negatives=[]
        for r,e in zip(retained,events):
            frame=index[r['source_frame']];raw=source[r['source_frame']-1]['wire'][r['packet_offset']:r['packet_end']]
            native_binding(e,r,frame,raw)
            assert e['id']not in seen;seen.add(e['id'])
            if e['flow']in flow_to_conn:assert flow_to_conn[e['flow']]==r['oracle_connection']
            else:flow_to_conn[e['flow']]=r['oracle_connection']
            s=e['session']or{};counts['all_packets']+=1
            if not r.get('independently_AEAD_authenticated'):continue
            qs=[q for q in packets[frame-1].findall('proto') if q.get('name')=='quic' and int(q.get('pos'))==r['packet_offset'] and int(q.get('size'))==r['packet_end']-r['packet_offset']]
            assert len(qs)==1;ws=qs[0];failed=bool(values(ws,'quic.decryption_failed'))
            if s.get('Authentication Verified')is True:
                assert s['Packet Number']==r['packet_number'] and s['Key Phase']==r['key_phase'] and s['Packet Number Length']==r['packet_number_bytes']
                assert s['Peer Identity Verified']is False
                fields+=exact_core(s.get('Frames',[]),packet_expected[r['source_frame'],r['packet_SHA256']])
                counts['source_authenticated_native_short']+=1
                if failed:counts['native_recovered_but_WS_failed']+=1
                else:
                    assert num(ws,'quic.packet_number')==r['packet_number']
                    assert num(ws,'quic.packet_number_length')+1==r['packet_number_bytes']
                    assert (values(ws,'quic.key_phase')[0].get('show')=='True')==bool(r['key_phase'])
                    counts['native_and_WS_source_short_exact']+=1
            else:
                assert s.get('Authentication Failure')=='packet-protection-failed' and s.get('Authentication Verified')is False
                assert not s.get('Frames') and not s.get('Decrypted') and 'Packet Number'not in s
                counts['source_valid_native_unopened_short']+=1
                assert r['oracle_connection']==plan['target_oracle_connection'] and r['direction']==plan['target_direction'] and r['epoch']==2
                assert failed
                counts['source_valid_native_and_WS_unopened_short']+=1
                target.append(dict(case=case['case'],source_frame=r['source_frame'],frame=frame,PDU=e['id'],flow=e['flow'],source_packet=r,expected_core_frames=packet_expected[r['source_frame'],r['packet_SHA256']],native_status=e['status'],native_admission=e['admission']))
            # A wire, direction, source/domain or plaintext-field forgery must fail.
            if len(negatives)<5:
                label=['Raw','direction','domain','reference','frame-value'][len(negatives)]
                bad=copy.deepcopy(e)
                if label=='Raw':bad['raw']=base64.b64encode(raw[:-1]+bytes([raw[-1]^1])).decode()
                elif label=='direction':bad['direction']=1-e['direction']
                elif label=='domain':bad['domain']['Interface']+=1
                elif label=='reference':bad['source_bytes']['PacketRefs'][0]['Number']+=1
                else:
                    core=copy.deepcopy(packet_expected[r['source_frame'],r['packet_SHA256']]);core[0]['Frame Type']='FORGED'
                try:
                    if label=='frame-value':exact_core(s['Frames'],core)
                    else:native_binding(bad,r,frame,raw)
                except AssertionError:negatives.append(label)
                else:raise AssertionError(('accepted negative control',label))
        assert set(flow_to_conn.values())=={0,1,2} and len(flow_to_conn)==3
        results.append(dict(case=case['case'],counts=dict(counts),independently_checked_wire_frame_fields=fields,flow_to_source_connection=flow_to_conn,rejected_controls=negatives,producer_SHA256=file_sha(folder/'producer.json'),reference_producer_SHA256=file_sha(wsdir/'producer.json')))
    write(a.out/'unopened-source-packets.json',target)
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),source_packet_plan_SHA256=file_sha(a.plan/'packet-source-plan.jsonl'),plan_SHA256=file_sha(a.plan/'plan-before.json'),source_authenticated_original_short_packets=count,results=results,unopened_source_valid_packets=len(target),unopened_source_plan_SHA256=file_sha(a.out/'unopened-source-packets.json'),full_HTTP3_recovery_or_Top30_parity_claim=False)
    write(a.out/'summary.json',summary);print(json.dumps({'summary':str(a.out/'summary.json'),'SHA256':file_sha(a.out/'summary.json'),'results':results,'unopened':len(target)}))


if __name__=='__main__':
    p=argparse.ArgumentParser(description=__doc__)
    for n in ('plan','actual','audit','original-reference','sparse-reference','out'):p.add_argument('--'+n,type=Path,required=True)
    run(p.parse_args())
