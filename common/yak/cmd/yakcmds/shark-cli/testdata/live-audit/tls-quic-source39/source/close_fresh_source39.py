"""Counts/hash closure for fresh owned source/AEAD/isolated reference evidence.

No native/Yak output is read. Complete data and all key/capture/export artifacts
remain external references. This is not a full TLS/QUIC feature certificate.
"""
import argparse,ast,hashlib,json,shutil
from pathlib import Path

def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--root',type=Path,required=True);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();assert not a.out.exists()
    root=a.root;phase=root.parent;artifacts=[]
    def bind(p,expected,role):
        p=Path(p);digest=sha(p)
        if expected is not None:assert digest==expected
        if p.suffix=='.py':ast.parse(p.read_bytes())
        artifacts.append(dict(path=str(p),SHA256=digest,bytes=p.stat().st_size,role=role));return read(p)if p.suffix=='.json'else None
    tls=bind(root/'TLS-source03-audit02/summary.json','2deb23f783baec94e24718269c754d597b1244cac5eae05823d0d7160c627a8d','closed fresh TLS source-only proof')
    quic=bind(root/'QUIC-source01-audit01/summary.json','6020e5c719c3d02eb42881671e0dfe8845a59ec0cebbac1d49769a5a8d826859','closed fresh QUIC independent all-space AEAD/CID/PN/STREAM/QPACK source')
    ws=bind(root/'QUIC-WS-fields02/summary.json','07112a08fd973eadb8c51cdf67a76462cd242af40ec84c347f4f020bbd51f917','closed packet and frame source-vs-reference field checks;3DATA view omissions retained')
    bind(root/'QUIC-WS-missing-DATA-owner01.json','27f39aaba38af92aa4f81e720383e4029363a9afc5d7386cf402ac52a3fb6b6d','source CID/PN/offset/FIN owner trace and all-alias/tail-frame reference search')
    negative=bind(root/'QUIC-source-negative01.json','01646f414fd28f2305fa6b2f43c11cda3b49105b305e8e7eed8d04e4d319b411','18 positive cipher/direction/epoch and138 rejected key/wire hypotheses')
    bind(root/'reference-TLS03-01/producer.json','fc2fae00a752e3c21cffb69b31ef63ab446ac96cbe3502ed9f7b74f1a8d7f062','isolated pinned TShark TLS PDML/literal-column producer')
    bind(root/'reference-QUIC01-01/producer.json','e0ba97ee4fe9aaa93b098cf76d6fcf2301e1c231ab0f8384c0685ee15412cacc','isolated pinned TShark QUIC PDML/literal-column producer')
    bind(root/'reference-QUIC-CRYPTO01/producer.json','133426d6744e495edf54cbd768143505c7332e63729ed2b725189fd3ba90e45a','12 FT_NONE CRYPTO raw-JSON source-byte occurrence checks')
    for name in('run_isolated_reference39.py','tls_source39.py','tls_source39_v02.py','quic_source39.py','quic_reference_fields39.py','quic_reference_fields39_v02.py','quic_crypto_raw_reference39.py','quic_missing_DATA_owner39.py','quic_source_negative39.py','close_fresh_source39.py'):bind(root/name,None,'external reproducible source;original bytes,never native parser')
    for name in('TLS-source03-audit01/controller-failure.json','TLS-reference-selector-correction02.json','QUIC-WS-fields01/summary.json','QUIC-reference-selector-correction02.json'):bind(root/name,None,'preserved reference-selector checker failure/correction;not product bug')
    for proof,folder,names in[(tls,root/'TLS-source03-audit02',['record-source.jsonl','HTTP-source.json','connection-source.json']),(quic,root/'QUIC-source01-audit01',['packet-source.jsonl','HTTP3-source.json','connection-source.json','handshake-source.json','HTTP3-frame-source.json'])]:
        for n in names:bind(folder/n,None,'external original source-fact table;do not archive or infer future native output')
        for p,digest in proof.get('input_hashes',proof.get('inputs',{})).items():assert sha(p)==digest
    for name in('fresh-TLS-resumption39-03','fresh-QUIC-epochs39-01'):
        for n in('live.pcap','session.keys','manifest.json'):bind(phase/name/n,None,'external private capture/key/peer receipt input;hash only,not copied')
    assert ws['authenticated_packet_and_frame_scalar_failures']==0 and len(ws['missing_complete_reference_DATA_anchors'])==3
    assert tls['AEAD_authenticated_records']==94 and quic['source_AEAD_authenticated_packets']==2022 and negative['rejected_mutations']==138
    counts=dict(TLS=dict(physical_frames=227,TCP_frames=224,marker_frames=3,SYN_generations=8,real_peer_resumed_connections=4,records=136,record_wire_bytes=49970,independent_AEAD_success=94,independent_AEAD_failure=0,authenticated_close_notify=16,HTTP_requests=24,HTTP_responses=24,all_HTTP_peer_hashes_and_reference_fields_match=True),
        QUIC=dict(physical_frames=2018,marker_frames=2,protected_packets=2025,independent_AEAD_success=2022,damaged_unopened_no_PN_or_plaintext=3,source_CID_connections=3,cipher_suites=3,epochs_each_direction=[0,1,2],authenticated_out_of_order_packets=180,unprotected_tail_views=6,HTTP3_requests=18,HTTP3_responses=18,all_source_HEADERS_QPACK_and_DATA_peer_hashes_match=True,WS_packet_frame_scalar_blob_checks=72632,WS_scalar_differences=0,WS_raw_CRYPTO_occurrences=12,WS_header_views=39,WS_unique_source_header_anchors=36,WS_explicit_repeated_header_views=3,WS_complete_DATA_views=33,WS_missing_complete_DATA_views=3,post_FIN_hole_repair_source_cases=2),source_only_negative_AEAD_hypotheses_rejected=138)
    result=dict(scope=__doc__,counts=counts,artifacts=artifacts,inputs_unchanged=True,no_native_or_Yak_output_read=True,no_Wireshark_internal_state_or_bug_claim=True,full_feature_parity_claim=False,certificate_trust_PSK_binder_0RTT_or_peer_replay_verification_claim=False,controller_failures_are_reference_selector_contract_issues=True,free_bytes_at_closure=shutil.disk_usage('C:/').free,
        primary_references=['https://www.rfc-editor.org/rfc/rfc9000.html#section-17.1','https://www.rfc-editor.org/rfc/rfc9001.html#section-5','https://www.rfc-editor.org/rfc/rfc9001.html#section-6','https://www.rfc-editor.org/rfc/rfc8446.html#section-5','https://www.rfc-editor.org/rfc/rfc9204.html'],
        source_schema_entrypoints={'TLS_records':'TLS-source03-audit02/record-source.jsonl:source SYN connection,direction,index,TCP stream offset,refs,wire digest/header,AEAD sequence+epoch/plaintext digest','TLS_HTTP':'TLS-source03-audit02/HTTP-source.json:connection,direction,index,application stream range,full message/body digests,authenticated TLS record parents','QUIC_packets':'QUIC-source01-audit01/packet-source.jsonl:physical frame/PDU slice/hash,ports,CID source connection,direction,PN/phase/width/epoch,all frame scalars and byte digests;unopened has no authenticated fields','QUIC_HTTP3':'QUIC-source01-audit01/HTTP3-source.json:CID connection,direction,stream,complete QPACK headers,body digest/bytes,FIN size,source contributors','QUIC_H3_frames':'QUIC-source01-audit01/HTTP3-frame-source.json:connection,direction,stream,type,logical stream interval,payload digest/completion frame'},
        source_HTTP3_contributor_union_is_not_a_required_native_canonical_ref_set=True)
    assert all(sha(r['path'])==r['SHA256']for r in artifacts)
    with a.out.open('x',encoding='utf8')as f:json.dump(result,f,indent=2);f.write('\n')
    print(json.dumps(dict(manifest=str(a.out),SHA256=sha(a.out),source_artifacts=len(artifacts),native_output_read=False)),flush=True)
if __name__=='__main__':main()
