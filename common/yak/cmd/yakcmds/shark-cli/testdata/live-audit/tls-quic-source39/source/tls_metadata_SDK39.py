"""Fresh isolated TLS raw reference, source-plaintext bridge, legacy metadata audit.

Ciphertext records/handshake plaintext first bind independently authenticated
source39 digests. The existing PSK/ticket field verifier then checks native fields,
typed aliases, per-connection context and explicit lack of binder/trust claims.
"""
import argparse,collections,hashlib,importlib.util,json,os,shutil,subprocess,sys,time,traceback
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def load(name,p):
    spec=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m);return m
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('sources','phase','audit','reference','actual','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();assert shutil.disk_usage('C:/').free>20*2**30;sys.path.insert(0,str(a.audit))
    source=a.sources/'TLS-source03-audit02';assert file_sha(source/'summary.json')=='2deb23f783baec94e24718269c754d597b1244cac5eae05823d0d7160c627a8d';s=read(source/'summary.json')
    records=[json.loads(line)for line in(source/'record-source.jsonl').read_text('utf8').splitlines()];assert file_sha(source/'record-source.jsonl')==s['record_source_SHA256']
    rp=read(a.reference/'producer.json');base=rp['executions'][0]['arguments'];assert base[-2:]==['-T','pdml'];args=base[:-2]+['-T','json','-x','--no-duplicate-keys']
    cap=a.phase/'fresh-TLS-resumption39-03/live.pcap';manifest=a.phase/'fresh-TLS-resumption39-03/manifest.json';events=a.actual/'TLS/events.jsonl';producer=read(a.actual/'TLS/events-producer.json');assert file_sha(events)==producer['export_SHA256']
    paths=[Path(__file__),source/'summary.json',source/'record-source.jsonl',cap,manifest,events,a.actual/'TLS/events-producer.json',a.audit/'tls_resumption_metadata_audit.py',a.audit/'tls_resumption_audit.py',a.reference/'producer.json',Path(args[0])];bound={str(p):file_sha(p)for p in paths}
    cfg=a.out/'empty-config';cfg.mkdir();env=os.environ.copy();env.update(APPDATA=str(cfg),WIRESHARK_CONFIG_DIR=str(cfg));env.pop('SSLKEYLOGFILE',None)
    try:
        start=time.monotonic()
        with(a.out/'reference.json').open('xb')as out,(a.out/'stderr').open('xb')as err:p=subprocess.run(args,stdout=out,stderr=err,env=env,creationflags=0x08000000,timeout=120)
        assert p.returncode==0;reference=read(a.out/'reference.json');audit=load('fresh_TLS_extension_oracle',a.audit/'tls_resumption_metadata_audit.py')
        physical,conns,owners,wire,hellos,total=audit.wire_connections(cap,read(manifest));expected=collections.defaultdict(collections.deque)
        for r in records:expected[r['connection'],r['direction'],r['record_SHA256']].append(r)
        ciphertext=0;messages=0;plain_record_hashes=0
        for frame,packet in enumerate(reference,1):
            layers=packet['_source']['layers'];assert bytes.fromhex(layers['frame_raw'][0])==physical[frame-1]['wire']
            if frame not in owners:continue
            cid,d=owners[frame]
            for raw,record in audit.records(layers):
                key=cid,d,sha(raw);assert expected[key];r=expected[key].popleft();ciphertext+=1
                hand=list(audit.handshakes(record))
                if r['plaintext_type']==22:
                    plain=b''.join(value for value,metadata in hand);assert sha(plain)==r['plaintext_SHA256']and len(plain)==r['plaintext_bytes'];messages+=len(hand);plain_record_hashes+=1
                else:assert not hand
        assert ciphertext==136 and messages==56 and all(not values for values in expected.values())
        write(a.out/'source-handshake-bridge.json',dict(scope='Every raw reference ciphertext record and complete handshake plaintext binds independently decoded source39 before checking metadata',literal_TLS_records=ciphertext,handshake_messages=messages,handshake_plaintext_record_digests=plain_record_hashes,all_ciphertext_and_handshake_plaintext_equal_source=True,source_summary_SHA256=file_sha(source/'summary.json'),reference_SHA256=file_sha(a.out/'reference.json'),no_binder_or_certificate_trust_claim=True))
        proof=audit.audit(cap,events,manifest,a.out/'reference.json');write(a.out/'metadata-native-proof.json',proof)
        after={p:file_sha(p)for p in bound};assert bound==after
        result=dict(scope=__doc__,source_summary_SHA256=file_sha(source/'summary.json'),SDK_export_SHA256=file_sha(events),reference_SHA256=file_sha(a.out/'reference.json'),reference_exit_code=p.returncode,seconds=round(time.monotonic()-start,3),source_handshake_bridge_SHA256=file_sha(a.out/'source-handshake-bridge.json'),native_metadata_proof_SHA256=file_sha(a.out/'metadata-native-proof.json'),TLS_records=ciphertext,handshake_messages=messages,record_authentication_and_all_PSK_ticket_wire_derived_context_alias_fields_exact=True,public_GetFields_equals_private_session=True,no_old_connection_context_on_reused_tuple=True,no_raw_ticket_binder_alias_or_false_verification=True,inputs=bound,inputs_unchanged=bound==after,whole_event_JSON_equality_claim=False)
        write(a.out/'summary.json',result);print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'),records=ciphertext,handshakes=messages,all_metadata_exact=True)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc()));raise
if __name__=='__main__':main()
