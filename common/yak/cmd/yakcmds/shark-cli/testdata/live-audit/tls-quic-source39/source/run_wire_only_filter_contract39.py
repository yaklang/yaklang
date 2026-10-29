"""Execute old closed CLI PDU-scope field queries; do not infer stream absence.

Input/generated PDU identities are bound to independently source-verified raw
slices. The two streams are source-known POST; one packet only buffers a future
range while another completes HEADERS. Default history budget is unchanged.
"""
import argparse,hashlib,json,os,shutil,subprocess,time,traceback
from pathlib import Path

def sha(b):return hashlib.sha256(b).hexdigest()
def file_sha(p):return sha(Path(p).read_bytes())
def read(p):return json.loads(Path(p).read_text('utf8'))
def rows(p):return[json.loads(l)for l in Path(p).read_text('utf-8-sig').splitlines()if l]
def write(p,v):
    with Path(p).open('x',encoding='utf8')as f:json.dump(v,f,indent=2);f.write('\n')
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('sources','phase','actual','bins','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();a.out.mkdir();assert shutil.disk_usage('C:/').free>20*2**30
    proof=a.sources/'SDK-d677-source39-audit01/summary.json';assert file_sha(proof)=='f75f6b9b62a936e1aa3033eb1eca8c5a4cd25ba4b7b3177a2f64d8b851ae158a'
    source=a.sources/'QUIC-source01-audit01';packets=rows(source/'packet-source.jsonl');HTTP=read(source/'HTTP3-source.json');events={e['id']:e for e in rows(a.actual/'QUIC/events.jsonl')};bindings=[]
    header=next(r for r in HTTP if(r['connection'],r['direction'],r['stream'])==(1,0,0));assert dict(header['headers'])[':method']=='POST'
    for ID,frame in[(12,7),(15,10)]:
        e=events[ID];raw=__import__('base64').b64decode(e['raw']);matches=[p for p in packets if p['frame']==frame and p['packet_SHA256']==sha(raw)];assert len(matches)==1 and matches[0]['authenticated']and matches[0]['connection']==1 and matches[0]['direction']==0
        assert e['source_bytes']['PacketRefs']==[dict(Number=frame,Domain=dict(Section=0,Interface=0,Encapsulation=''))]
        bindings.append(dict(native_PDU=ID,source_frame=frame,source_packet_SHA256=sha(raw),source_connection=1,source_direction=0))
    cases=[
        dict(name='buffered-absence-of-current-PDU-method',query='pdu.id == 15 and not http.request.method',frames=[10]),
        dict(name='buffered-does-not-replay-stream-method',query='pdu.id == 15 and http.request.method == "POST"',frames=[]),
        dict(name='buffered-not-PDU-POST-does-not-negate-POST-stream',query='pdu.id == 15 and not (http.request.method == "POST")',frames=[10]),
        dict(name='buffered-explicit-wire-only',query='pdu.id == 15 and http3.wire_only == true',frames=[10]),
        dict(name='completed-HEADERS-has-source-POST',query='pdu.id == 12 and http.request.method == "POST"',frames=[7]),
        dict(name='completed-HEADERS-negated-presence-is-false',query='pdu.id == 12 and not http.request.method',frames=[]),
        dict(name='unknown-http3-method-is-compile-error',query='not http3.request.method',error='unknown display field "http3.request.method"'),
        dict(name='unknown-request-PDU-is-compile-error',query='not http3.request_pdu',error='unknown display field "http3.request_pdu"'),
    ]
    binary=a.bins/'yak.exe';buildpath=a.bins/'yak-receipt.json';build=read(buildpath);assert build['exit_code']==0 and build['source']=='d6772965c669e288705f7e0d0e25c62972226832'and file_sha(binary)==build['binary_SHA256']
    cap=a.phase/'fresh-QUIC-epochs39-01/live.pcap';keys=a.phase/'fresh-QUIC-epochs39-01/session.keys';paths=[Path(__file__),proof,source/'summary.json',source/'packet-source.jsonl',source/'HTTP3-source.json',a.actual/'QUIC/events.jsonl',a.actual/'QUIC/events-producer.json',binary,buildpath,cap,keys]
    bound={str(p):file_sha(p)for p in paths};plan=dict(scope=__doc__,source_bindings=bindings,cases=cases,input_hashes=bound,source_stream_method_is_POST=True,absence_is_only_newly_completed_PDU_header_field=True,default_protocol_history_budget_MiB=64,source_plan_before_execution=True)
    write(a.out/'plan-before.json',plan);results=[]
    try:
        for c in cases:
            d=a.out/c['name'];d.mkdir();args=[str(binary),'shark','-r',str(cap),'--json','--ignore-embedded-secrets','--tls-keylog',str(keys),'--display-filter',c['query']];start=time.monotonic()
            with(d/'output.jsonl').open('xb')as out,(d/'stderr').open('xb')as err:r=subprocess.run(args,stdout=out,stderr=err,env=dict(os.environ,GOMAXPROCS='1',GOMEMLIMIT='1GiB'),creationflags=0x08000000,timeout=60)
            receipt=dict(case=c['name'],query=c['query'],arguments=args,exit_code=r.returncode,seconds=round(time.monotonic()-start,3),stdout_SHA256=file_sha(d/'output.jsonl'),stderr_SHA256=file_sha(d/'stderr'),binary_SHA256=bound[str(binary)],source=build['source'],source_plan_SHA256=file_sha(a.out/'plan-before.json'),capture_SHA256=bound[str(cap)],keylog_SHA256=bound[str(keys)])
            write(d/'producer.json',receipt)
            if'error'in c:
                diagnostic=(d/'output.jsonl').read_text('utf-8-sig')+(d/'stderr').read_text('utf-8-sig');assert r.returncode!=0 and c['error']in diagnostic;receipt['compile_error_matches']=True
            else:
                assert r.returncode==0;output=rows(d/'output.jsonl');numbers=[p['number']for p in output];assert numbers==c['frames'];receipt['matched_physical_frames']=numbers
                for p in output:
                    vals=[v for e in p['pdus']for v in e['field_values']['values']];method=[v['value']for v in vals if v['name']=='http.request.method']
                    if p['number']==10:assert not method
                    if p['number']==7:assert method==['POST']
            results.append(receipt);print(json.dumps(dict(case=c['name'],exit_code=r.returncode,contract_matches=True)),flush=True)
        after={p:file_sha(p)for p in bound};assert bound==after
        write(a.out/'summary.json',dict(scope=__doc__,cases=8,passed_cases=8,source_stream_method_is_POST=True,registered_method_origin='pdu',missing_current_PDU_field_is_not_stream_method_absence=True,does_not_certify_no_request_in_connection=True,unknown_aliases_error_not_false_absence=True,source_plan_SHA256=file_sha(a.out/'plan-before.json'),results=results,input_hashes=bound,inputs_unchanged=True,controller_SHA256=file_sha(__file__)))
        print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=file_sha(a.out/'summary.json'),passed=8)),flush=True)
    except Exception:
        write(a.out/'controller-failure.json',dict(error=traceback.format_exc(),completed_cases=len(results)));raise
if __name__=='__main__':main()
