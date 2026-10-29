"""Locate every IPv4-declared OSPF fragment outside the complete556 inventory.

Only original carrier declarations are followed. Incomplete fragment coverage
does not authorize OSPF decoding, signature searching or cross-datagram stitching.
"""
import argparse,collections,hashlib,importlib.util,json,sys,time
from pathlib import Path

def sha(p):
    h=hashlib.sha256()
    with Path(p).open('rb')as f:
        for b in iter(lambda:f.read(1024*1024),b''):h.update(b)
    return h.hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def load(name,p):
    s=importlib.util.spec_from_file_location(name,p);m=importlib.util.module_from_spec(s);s.loader.exec_module(m);return m
def main():
    ap=argparse.ArgumentParser(description=__doc__)
    for n in('phase','census','out'):ap.add_argument('--'+n,type=Path,required=True)
    a=ap.parse_args();assert not a.out.exists();assert sha(a.census/'summary.json')=='ae983a9c7c699b7b629f8e6208d8226a5b59394ae3d3eb3185e6fa6dea3047bb'
    census=load('OSPFv2_original_carrier_census',a.phase/'ospfv2_source_census39.py');inputs=read(a.phase/'OSPFv3-independent-source37-a82-01/inputs01/inputs.json');sys.path.insert(0,str(Path(inputs['capture_reader']).parent));reader=load('OSPF_original_capture_reader',Path(inputs['capture_reader']));assert sha(inputs['capture_reader'])==inputs['capture_reader_SHA256'];results=[];counts=collections.Counter();started=time.monotonic()
    class Observe(census.V2Census):
        def enter(self,kind,start,end,path,depth=0):
            if kind=='ipv4'and start+20<=end and self.w[start]>>4==4 and self.w[start+9]==89:
                ihl=(self.w[start]&15)*4;total=int.from_bytes(self.w[start+2:start+4],'big');flags=int.from_bytes(self.w[start+6:start+8],'big')
                if ihl>=20 and start+ihl<=end and total>=ihl and flags&0x3fff:
                    begin=start+ihl;stop=min(start+total,end);payload=self.w[begin:stop]
                    self.fragment_findings.append(dict(IPv4_header_range=[start,begin],parent_available_end=end,declared_IPv4_end=start+total,parent_complete=start+total<=end,IPv4_flags_and_offset=flags,fragment_offset_bytes=(flags&0x1fff)*8,more_fragments=bool(flags&0x2000),source='.'.join(str(b)for b in self.w[start+12:start+16]),destination='.'.join(str(b)for b in self.w[start+16:start+20]),IPv4_ID=int.from_bytes(self.w[start+4:start+6],'big'),payload_range=[begin,stop],captured_payload_bytes=len(payload),payload_SHA256=hashlib.sha256(payload).hexdigest(),IPv4_header_checksum_match=census.checksum(self.w[start:begin])==0,carrier=path,full_frame_SHA256=hashlib.sha256(self.w).hexdigest()))
            return super().enter(kind,start,end,path,depth)
    for entry in read(a.census/'summary.json')['captures']:
        expected=entry['carrier_counters'].get('IPv4_OSPF_fragment_needs_separate_reassembly',0)
        if not expected:continue
        capture=Path(entry['capture']);assert sha(capture)==entry['capture_SHA256'];local=[];n=0
        for n,record in enumerate(reader.capture_records(capture),1):
            visit=Observe(record['wire'],record,n,counts);visit.fragment_findings=[]
            try:visit.run()
            except(AssertionError,ValueError,IndexError):pass
            for f in visit.fragment_findings:f.update(input=entry['input'],frame=n,domain=dict(section=record['section'],interface=record['interface'],link=record['link']),time_ns=record['time_ns']);local.append(f)
        assert n==entry['physical_records']and len(local)==expected and sha(capture)==entry['capture_SHA256'];results.extend(local)
    groups=collections.defaultdict(list)
    for r in results:groups[r['input'],r['domain']['section'],r['domain']['interface'],r['source'],r['destination'],r['IPv4_ID']].append(r)
    groupfacts=[]
    for key,parts in groups.items():
        ranges=sorted([p['fragment_offset_bytes'],p['fragment_offset_bytes']+p['captured_payload_bytes']]for p in parts);complete=bool(parts)and ranges[0][0]==0 and any(not p['more_fragments']for p in parts)and all(a[1]==b[0]for a,b in zip(ranges,ranges[1:]))and all(p['parent_complete']for p in parts)
        groupfacts.append(dict(input=key[0],source=key[3],destination=key[4],IPv4_ID=key[5],source_frames=[p['frame']for p in parts],captured_byte_ranges=ranges,complete_nonoverlapping_parent_valid_coverage=complete,OSPF_payload_decoded=False))
    assert not any(g['complete_nonoverlapping_parent_valid_coverage']for g in groupfacts)
    report=dict(scope=__doc__,original_census_summary_SHA256=sha(a.census/'summary.json'),source_fragments=len(results),source_complete_reassemblies=0,results=results,groups=groupfacts,seconds=round(time.monotonic()-started,3),extra_complete_OSPFv2_messages_inferred=0,no_candidate_or_native_output_read=True,source_code_SHA256=sha(__file__),carrier_source_SHA256=sha(a.phase/'ospfv2_source_census39.py'),capture_reader_SHA256=sha(inputs['capture_reader']))
    with a.out.open('x',encoding='utf8')as f:json.dump(report,f,indent=2);f.write('\n')
    print(json.dumps(dict(proof=str(a.out),SHA256=sha(a.out),source_fragments=len(results),complete_reassemblies=0,frames=[r['frame']for r in results])),flush=True)
if __name__=='__main__':main()
