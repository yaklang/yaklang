"""Declare source-exact capture omissions and reordering of owned resumed TLS.

All retained physical bytes and original timestamps remain literal. These are
offline observation transformations, not claims that the endpoints experienced
network loss. Only generation one of each profile is changed; the surrounding
full/resumed/fresh generations remain available to test connection isolation.
"""
import argparse,hashlib,json,struct,sys
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[1]))
from tls_resumption_audit import wire_connections
from owned_quic_clock_audit import vals,number
from capture_clock_wire_audit import ip


def sha(data):return hashlib.sha256(data).hexdigest()
def pcap(path,observations):
    links={r['link']for r in observations};assert len(links)==1
    with path.open('xb')as out:
        out.write(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,next(iter(links))))
        for r in observations:
            seconds,nanos=divmod(r['time_ns'],10**9);wire=r['wire'];out.write(struct.pack('<IIII',seconds,nanos,len(wire),len(wire))+wire)


def run(capture,manifest,oracle,out,start_only=False):
    peer=json.loads(manifest.read_text('utf8'));records,connections,owners,tls,hellos,total=wire_connections(capture,peer)
    reference={number(e['_source']['layers']['frame']['frame.number']):e['_source']['layers']for e in json.loads(oracle.read_text('utf8'))}
    out.mkdir(parents=True,exist_ok=False)
    for profile in['tls12','tls13']:
        cid=next(n for n,c in connections.items()if(c['profile'],c['generation'])==(profile,1));frames=connections[cid]['frames']
        client_hello=[n for n in frames if 1 in map(number,vals(reference[n],'tls.handshake.type'))]
        server_hello=[n for n in frames if 2 in map(number,vals(reference[n],'tls.handshake.type'))]
        requests=[n for n in frames if vals(reference[n],'http.request.method')]
        responses=[n for n in frames if vals(reference[n],'http.response.code')]
        assert len(client_hello)==len(server_hello)==1 and len(requests)==len(responses)==3
        _,header,payload=ip(records[responses[0]-1]);offset=(payload[12]>>4)*4;wire=payload[offset:]
        assert len(wire)==5+int.from_bytes(wire[3:5],'big')and wire[0]==23
        record_count=sum(1 for(key,digest),entries in tls.items()for e in entries if e['refs']==[responses[0]]and e['wire']==wire);assert record_count==1
        modes=[('missing-ClientHello',set(client_hello),None),('missing-ServerHello',set(server_hello),None),('midstream-resumed',{n for n in frames if n<requests[0]},None),('missing-one-server-record',{responses[0]},None),('reordered-server-records',set(),responses[:2])]
        if start_only:modes=[('capture-starts-midstream',set(range(1,requests[0])),None)]
        for mode,removed,swapped in modes:
            order=[n for n in range(1,len(records)+1)if n not in removed]
            if swapped:
                a,b=map(order.index,swapped);order[a],order[b]=order[b],order[a]
            dest=out/(profile+'-'+mode);dest.mkdir();observations=[records[n-1]for n in order];pcap(dest/'observation.pcap',observations)
            ledger=[dict(number=i,source_frame=n,source_time_ns=records[n-1]['time_ns'],source_wire_SHA256=sha(records[n-1]['wire']),source_section=records[n-1]['section'],source_interface=records[n-1]['interface'],change='exact-reordered-record'if swapped and n in swapped else'exact-retained-record')for i,n in enumerate(order,1)]
            omissions=[dict(source_frame=n,source_time_ns=records[n-1]['time_ns'],source_wire_SHA256=sha(records[n-1]['wire']),reason=mode)for n in sorted(removed)]
            receipt=dict(scope=__doc__,generator_SHA256=sha(Path(__file__).read_bytes()),source_capture=str(capture.resolve()),source_capture_SHA256=sha(capture.read_bytes()),source_manifest_SHA256=sha(manifest.read_bytes()),source_reference_SHA256=sha(oracle.read_bytes()),capture_SHA256=sha((dest/'observation.pcap').read_bytes()),case=dest.name,profile=profile,target_original_connection=cid,target_generation=1,original_physical_records=len(records),physical_records=len(observations),omitted_records=omissions,swapped_original_frame_pair=swapped,ledger=ledger,all_retained_wire_bytes_and_timestamps_exact=True,network_fault_injection=False,endpoint_resumption_receipts_refer_to_original_capture_only=True,deleted_record_was_one_complete_TLS_record=mode=='missing-one-server-record')
            (dest/'manifest.json').write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf8');print(json.dumps(dict(case=dest.name,removed=len(removed),swapped=swapped,records=len(observations))),flush=True)


if __name__=='__main__':
    ap=argparse.ArgumentParser(description=__doc__)
    for name in['capture','manifest','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
    ap.add_argument('--capture-start-only',action='store_true')
    a=ap.parse_args()
    for p in[a.capture,a.manifest,a.oracle,a.out]:assert not any((v/'.git').exists()for v in[p.resolve(),*p.resolve().parents])
    run(a.capture,a.manifest,a.oracle,a.out,a.capture_start_only)
