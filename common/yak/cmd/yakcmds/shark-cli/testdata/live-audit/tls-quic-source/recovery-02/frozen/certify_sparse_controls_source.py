"""Independently authenticate all retained short packets in recovery controls."""
import json,sys
from pathlib import Path
import xml.etree.ElementTree as ET
from cryptography.exceptions import InvalidTag
from recovery_probe import file_sha,sha,keys,write
from quic_sparse_epoch import unprotect,values,num
from sparse_epoch_packet_audit import literal_frames

def run():
    p=Path(__file__).parent.parent;out=p/'QUIC-sparse-recovery-controls-source-before-01'
    sys.path.insert(0,r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\yak\cmd\yakcmds\shark-cli\testdata\live-audit')
    from vxlan_endpoint_projection_audit import capture_records
    source=json.loads((p/'QUIC-sparse-epoch-plan-before-01/plan-before.json').read_text('utf8'))
    plan=json.loads((p/'QUIC-sparse-recovery-controls-before-01/plan-before.json').read_text('utf8'))
    rows=[json.loads(x)for x in(p/'QUIC-sparse-epoch-plan-before-01/packet-source-plan.jsonl').read_text('utf8').splitlines()]
    byframe={}
    for r in rows:byframe.setdefault(r['source_frame'],[]).append(r)
    secrets=keys(plan['cases'][0]['keylog']);ws=ET.parse(p/'TLS-QUIC-reference25-01/quic-original/reference.pdml').getroot();randoms={};suites={}
    for packet in ws:
        cid=num(packet,'quic.connection.number')
        for f in values(packet,'tls.handshake.random'):
            rnd=bytes.fromhex(f.get('show').replace(':',''))
            if('CLIENT_TRAFFIC_SECRET_0',rnd)in secrets:randoms[cid]=rnd
        for f in values(packet,'tls.handshake.ciphersuite'):suites[cid]=int(f.get('show'),0)
    out.mkdir(exist_ok=False);results=[]
    for case in plan['cases']:
        cap=Path(case['capture']);assert file_sha(cap)==case['capture_SHA256']and file_sha(cap.parent/'manifest.json')==case['source_manifest_SHA256']
        doc=json.loads((cap.parent/'manifest.json').read_text('utf8'));records=list(capture_records(cap));assert len(records)==len(doc['ledger'])
        good=[];bad=[]
        for record,entry in zip(records,doc['ledger']):
            assert sha(record['wire'])==entry['source_wire_SHA256']and record['time_ns']==entry['source_time_ns']
            for r in byframe[entry['source_frame']]:
                if not r.get('independently_AEAD_authenticated'):continue
                raw=record['wire'][r['packet_offset']:r['packet_end']]
                label=('SERVER'if r['direction']else'CLIENT')+'_TRAFFIC_SECRET_0'
                try:w,phase,width=unprotect(raw,r['packet_number'],bytes.fromhex(r['destination_cid']),secrets[label,randoms[r['oracle_connection']]],suites[r['oracle_connection']],r['epoch'])
                except InvalidTag:
                    assert entry.get('declared_mutation')and r['source_frame']==464
                    bad.append(dict(frame=entry['number'],source_frame=entry['source_frame'],packet_SHA256=sha(raw),independent_tag_failure=True));continue
                assert not entry.get('declared_mutation')and sha(raw)==r['packet_SHA256']and sha(w)==r['plaintext_SHA256']
                assert phase==r['key_phase']and width==r['packet_number_bytes']
                good.append(dict(frame=entry['number'],source_frame=r['source_frame'],packet_SHA256=sha(raw),PN=r['packet_number'],epoch=r['epoch'],phase=phase,connection=r['oracle_connection'],direction=r['direction'],core_frames=literal_frames(w)))
        expected=1855 if case['case']=='first-epoch2-tag-damaged' else 1858
        assert len(good)==expected and len(bad)==(1 if case['case']=='first-epoch2-tag-damaged'else 0)
        target=out/(case['case']+'-source-packets.json');write(target,dict(good=good,declared_bad_tags=bad))
        results.append(dict(case=case['case'],manifest_SHA256=case['source_manifest_SHA256'],capture_SHA256=case['capture_SHA256'],independently_authenticated_short_occurrences=len(good),declared_extra_bad_tags=len(bad),source_packets_SHA256=file_sha(target)))
    summary=dict(scope=__doc__,auditor_SHA256=file_sha(__file__),plan_SHA256=file_sha(p/'QUIC-sparse-recovery-controls-before-01/plan-before.json'),results=results)
    write(out/'summary.json',summary);print(json.dumps({'summary':str(out/'summary.json'),'SHA256':file_sha(out/'summary.json'),'results':results}))
if __name__=='__main__':run()
