"""Independent IPv4-declared OSPFv2 source envelopes, not a native decoder.

Reuse the already frozen carrier/record reader, never a candidate event or a
payload signature search. Authentication/LSDB/neighbor state is not inferred.
This inventory intentionally does not claim every LSA body field is covered.
"""
from pathlib import Path
import argparse,collections,hashlib,importlib.util,ipaddress,json,sys

root=Path(__file__).parent
source=root/'OSPFv3-independent-source37-a82-01'
sys.path.insert(0,str(source))
from scan_source37 import Census
from ospf_source_codec import checksum,fletcher
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
def number(w,at,n):
    assert 0<=at<=at+n<=len(w),(at,n,len(w))
    return int.from_bytes(w[at:at+n],'big')

class V2Census(Census):
    def __init__(self,*args):super().__init__(*args);self.v2=[]
    def enter(self,kind,start,end,path,depth=0):
        if kind=='ipv4':
            self.require(start,20,end)
            if self.w[start]>>4==4 and self.w[start+9]==89:
                ihl=(self.w[start]&15)*4;total=self.u(start+2)
                self.require(start,ihl,end)
                assert ihl>=20 and total>=ihl
                flags=self.u(start+6)
                if flags&0x3fff:
                    self.counts['IPv4_OSPF_fragment_needs_separate_reassembly']+=1
                    return
                at=start+ihl;limit=min(start+total,end)
                self.v2.append(dict(payload_range=[at,limit],IPv4_range=[start,limit],declared_IPv4_end=start+total,parent_complete=start+total<=end,IPv4_flags_fragment=flags,IPv4_header_checksum_match=checksum(self.w[start:at])==0,source=str(ipaddress.IPv4Address(self.w[start+12:start+16])),destination=str(ipaddress.IPv4Address(self.w[start+16:start+20])),carrier=path+[dict(kind='IPv4',range=[start,at],protocol=89)]))
                return
        return super().enter(kind,start,end,path,depth)

def decode(w):
    assert len(w)>=24 and w[0]==2
    n=number(w,2,2);assert n>=24 and n<=len(w)
    typ=w[1];auth=number(w,14,2)
    out=dict(version=2,type=typ,packet_length=n,router_ID=str(ipaddress.IPv4Address(w[4:8])),area_ID=str(ipaddress.IPv4Address(w[8:12])),AuType=auth,auth_wire_range=[16,24],authentication_verified=False,tail_bytes=len(w)-n,packet_checksum_match=checksum(w[:16]+bytes(8)+w[24:n])==0,checksum_applicability='base RFC2328 excludes8 authentication bytes' if auth in(0,1) else 'cryptographic or unknown authentication context;no packet-checksum verdict',header_range=[0,24],LSA_headers=[],full_LSAs=[],scalar_ranges={})
    if auth==2:
        out['crypto_observed_header']=dict(reserved=number(w,16,2),key_ID=w[18],digest_length=w[19],sequence=number(w,20,4),authentication_verified=False)
    if typ==1:
        assert n>=44 and (n-44)%4==0
        out['hello']=dict(network_mask=str(ipaddress.IPv4Address(w[24:28])),hello_interval=number(w,28,2),options=w[30],priority=w[31],dead_interval=number(w,32,4),designated_router=str(ipaddress.IPv4Address(w[36:40])),backup_designated_router=str(ipaddress.IPv4Address(w[40:44])),neighbors=[dict(value=str(ipaddress.IPv4Address(w[a:a+4])),range=[a,a+4])for a in range(44,n,4)])
    elif typ==2:
        assert n>=32 and (n-32)%20==0
        out['database_description']=dict(MTU=number(w,24,2),options=w[26],flags=w[27],sequence=number(w,28,4))
        out['LSA_headers']=[header(w,a)for a in range(32,n,20)]
    elif typ==3:
        assert (n-24)%12==0
        out['requests']=[dict(type=number(w,a,4),link_state_ID=str(ipaddress.IPv4Address(w[a+4:a+8])),advertising_router=str(ipaddress.IPv4Address(w[a+8:a+12])),range=[a,a+12])for a in range(24,n,12)]
    elif typ==4:
        assert n>=28
        count=number(w,24,4);assert count<=(n-28)//20
        at=28
        for _ in range(count):
            h=header(w[:n],at);size=h['length'];assert size>=20 and at+size<=n
            h.update(complete_LSA_range=[at,at+size],body_range=[at+20,at+size],body_SHA256=hashlib.sha256(w[at+20:at+size]).hexdigest(),Fletcher=fletcher(w[at:at+size]))
            out['full_LSAs'].append(h);at+=size
        assert at==n
    elif typ==5:
        assert (n-24)%20==0
        out['LSA_headers']=[header(w,a)for a in range(24,n,20)]
    else:out['unknown_message_body_opaque']=True
    return out

def header(w,at):
    assert at+20<=len(w)
    return dict(range=[at,at+20],age=number(w,at,2),options=w[at+2],type=w[at+3],link_state_ID=str(ipaddress.IPv4Address(w[at+4:at+8])),advertising_router=str(ipaddress.IPv4Address(w[at+8:at+12])),sequence=number(w,at+12,4),checksum=number(w,at+16,2),length=number(w,at+18,2))

def main():
    ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--out',type=Path,required=True);a=ap.parse_args();a.out.mkdir()
    inventory=source/'inputs01/inputs.json';doc=json.loads(inventory.read_text('utf8'));assert len(doc['inputs'])==37
    reader=Path(doc['capture_reader']);assert sha(reader)==doc['capture_reader_SHA256']
    sys.path.insert(0,str(reader.parent))
    spec=importlib.util.spec_from_file_location('original_capture_reader',reader);m=importlib.util.module_from_spec(spec);spec.loader.exec_module(m)
    summaries=[]
    for item in doc['inputs']:
        capture=Path(item['capture']);assert sha(capture)==item['capture_SHA256']
        counters=collections.Counter();selected=[];refusals=0
        folder=a.out/('input-%02d'%item['input']);folder.mkdir();frames=folder/'original-frames';frames.mkdir()
        count=0
        for count,record in enumerate(m.capture_records(capture),1):
            visit=V2Census(record['wire'],record,count,counters)
            try:visit.run()
            except(AssertionError,ValueError,IndexError):refusals+=1
            for index,candidate in enumerate(visit.v2):
                raw=record['wire'];wire=raw[slice(*candidate['payload_range'])]
                file=frames/('frame-%d-%d.bin'%(count,index));file.write_bytes(raw)
                row=dict(frame=count,domain=dict(section=record['section'],interface=record['interface'],link=record['link']),time_ns=record['time_ns'],captured_length=record['captured_length'],original_length=record['original_length'],original_frame=str(file),original_frame_SHA256=sha(file),carrier=candidate)
                try:row['source_OSPFv2']=decode(wire)
                except(AssertionError,ValueError,IndexError)as error:row['body_source_refusal']=str(error)
                selected.append(row)
        assert count==item['physical_frames'] and sha(capture)==item['capture_SHA256']
        write(folder/'source-facts.json',selected)
        summary=dict(input=item['input'],capture=str(capture),capture_SHA256=sha(capture),physical_records=count,declared_IPv4_OSPF=len(selected),complete_v2=sum('source_OSPFv2'in r for r in selected),message_types=dict(collections.Counter(str(r['source_OSPFv2']['type'])for r in selected if 'source_OSPFv2'in r)),auth_types=dict(collections.Counter(str(r['source_OSPFv2']['AuType'])for r in selected if 'source_OSPFv2'in r)),full_LSA_types=dict(collections.Counter(str(h['type'])for r in selected if 'source_OSPFv2'in r for h in r['source_OSPFv2']['full_LSAs'])),facts_SHA256=sha(folder/'source-facts.json'),carrier_counters=dict(counters),other_parent_refusals=refusals)
        summaries.append(summary);print(json.dumps({k:summary[k]for k in('input','declared_IPv4_OSPF','complete_v2','message_types','auth_types')}),flush=True)
    write(a.out/'summary.json',dict(scope=__doc__,auditor_SHA256=sha(__file__),inventory_SHA256=sha(inventory),frozen_carrier_source_SHA256=sha(source/'scan_source37.py'),captures=summaries,total_selected=sum(r['declared_IPv4_OSPF']for r in summaries),total_complete_v2=sum(r['complete_v2']for r in summaries),native_or_CLI_output_read=False,Go_run=False,LSA_body_semantics_not_yet_parsed=True))
    print(json.dumps(dict(summary=str(a.out/'summary.json'),SHA256=sha(a.out/'summary.json'))))
if __name__=='__main__':main()
