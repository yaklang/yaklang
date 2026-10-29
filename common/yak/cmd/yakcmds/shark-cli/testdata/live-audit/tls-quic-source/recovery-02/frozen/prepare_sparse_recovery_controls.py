"""Freeze source-exact late epochs and one declared tag mutation.

These are offline observation controls derived from an owned real capture.
Endpoints did not experience the added observation loss/reordering/corruption.
Unchanged records keep complete wire bytes, original lengths and ns timestamps.
"""
import base64,copy,hashlib,json,struct,sys
from pathlib import Path
import xml.etree.ElementTree as ET
from recovery_probe import keys,file_sha,sha,write,guard
from quic_sparse_epoch import unprotect,values,num

def checksum(w):
    if len(w)&1:w+=b'\0'
    n=sum(int.from_bytes(w[i:i+2],'big')for i in range(0,len(w),2))
    while n>>16:n=(n&65535)+(n>>16)
    return(~n)&65535

def save_cap(path,records):
    links={r['link']for r in records};assert len(links)==1
    with path.open('xb')as f:
        f.write(struct.pack('<IHHIIII',0xa1b23c4d,2,4,0,0,65535,next(iter(links))))
        for r in records:
            s,ns=divmod(r['time_ns'],10**9);w=r['wire'];f.write(struct.pack('<IIII',s,ns,len(w),r['original_length'])+w)

def run():
    guard();p=Path(__file__).parent.parent;out=p/'QUIC-sparse-recovery-controls-before-01'
    sys.path.insert(0,r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang\common\yak\cmd\yakcmds\shark-cli\testdata\live-audit')
    from vxlan_endpoint_projection_audit import capture_records
    plan=json.loads((p/'QUIC-sparse-epoch-plan-before-01/plan-before.json').read_text('utf8'))
    rows=[json.loads(x)for x in(p/'QUIC-sparse-epoch-plan-before-01/packet-source-plan.jsonl').read_text('utf8').splitlines()]
    original=list(capture_records(Path(plan['source_capture'])));case=plan['cases'][1]
    sparse=list(capture_records(Path(case['capture'])));assert len(sparse)==1865
    r=next(r for r in rows if r['oracle_connection']==0 and r['direction']==0 and r.get('epoch')==2)
    number=next(v['number']for v in plan['ledger']if v['source_frame']==r['source_frame']);assert number==294
    assert r['packet_offset']==32 and r['packet_end']==len(sparse[number-1]['wire'])
    late=next(v for v in rows if v['oracle_connection']==0 and v['direction']==0 and v.get('epoch')==1)
    old=max((v for v in rows if v['oracle_connection']==0 and v['direction']==0 and v.get('epoch')==0),key=lambda v:v['packet_number'])
    assert late['packet_number']==58 and old['packet_number']==57
    out.mkdir(exist_ok=False);cases=[]
    for name in ['first-epoch2-tag-damaged','late-epoch1-and-old-epoch0-duplicate']:
        folder=out/name;folder.mkdir();records=copy.deepcopy(sparse);ledger=copy.deepcopy(plan['ledger']);mutations=[]
        if name=='first-epoch2-tag-damaged':
            w=bytearray(records[number-1]['wire']);assert w[:4]==b'\x02\0\0\0'and w[4]>>4==4 and w[13]==17
            badat=r['packet_end']-1;w[badat]^=1;ip=4;udp=ip+(w[ip]&15)*4
            oldsum=int.from_bytes(w[udp+6:udp+8],'big')
            if oldsum:
                w[udp+6:udp+8]=b'\0\0';length=int.from_bytes(w[udp+4:udp+6],'big');pseudo=bytes(w[ip+12:ip+20])+b'\0\x11'+length.to_bytes(2,'big')
                cs=checksum(pseudo+bytes(w[udp:udp+length]));w[udp+6:udp+8]=(cs or 65535).to_bytes(2,'big')
            records[number-1]['wire']=bytes(w)
            mutations=[dict(frame=number,source_frame=r['source_frame'],declared_XOR_byte_offset=badat,XOR=1,UDP_checksum_recomputed_if_original_nonzero=bool(oldsum),before_wire_SHA256=sha(sparse[number-1]['wire']),after_wire_SHA256=sha(bytes(w)))]
            ledger[number-1]['declared_mutation']=mutations[0]
        else:
            insert=[late['source_frame'],old['source_frame']]
            records[number:number]=[copy.deepcopy(original[n-1])for n in insert]
            ledger[number:number]=[dict(source_frame=n,original_timestamp_retained=True,change='late-existing-source-packet')for n in insert]
        for n,item in enumerate(ledger,1):
            item['number']=n;item['source_wire_SHA256']=sha(records[n-1]['wire']);item['source_time_ns']=records[n-1]['time_ns']
        dest=folder/'observation.pcap';save_cap(dest,records)
        reread=list(capture_records(dest));assert len(reread)==len(records)
        for a,b in zip(reread,records):assert a['wire']==b['wire']and a['time_ns']==b['time_ns']and a['original_length']==b['original_length']
        doc=dict(scope=__doc__,generator_SHA256=file_sha(__file__),source_plan_SHA256=file_sha(p/'QUIC-sparse-epoch-plan-before-01/plan-before.json'),source_capture_SHA256=plan['source_capture_SHA256'],base_capture_SHA256=case['capture_SHA256'],case=name,capture=str(dest),capture_SHA256=file_sha(dest),keylog=case['keylog'],keylog_SHA256=case['keylog_SHA256'],ledger=ledger,declared_mutations=mutations,first_new_epoch_source_packet=r,late_old_source_packets=[late,old]if name!='first-epoch2-tag-damaged'else[],network_fault_injection=False)
        write(folder/'manifest.json',doc)
        cases.append(dict(case=name,capture=str(dest),capture_SHA256=file_sha(dest),keylog=case['keylog'],keylog_SHA256=case['keylog_SHA256'],source_manifest_SHA256=file_sha(folder/'manifest.json')))
    summary=dict(scope=__doc__,generator_SHA256=file_sha(__file__),source_plan_SHA256=file_sha(p/'QUIC-sparse-epoch-plan-before-01/plan-before.json'),cases=cases,required_authentication_recovery_after_damaged_first_epoch2_packet=True,late_epoch1_requires_authentication_without_phase_rollback=True,late_epoch0_original_PN57_requires_duplicate_without_frame_redelivery=True,original_real_capture_or_source_exact_offline_transformation=True)
    write(out/'plan-before.json',summary);print(json.dumps({'plan':str(out/'plan-before.json'),'SHA256':file_sha(out/'plan-before.json'),'cases':len(cases)}))
if __name__=='__main__':run()
