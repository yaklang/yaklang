"""Inspect original official RSVP common headers/object envelopes and TShark.

Independent source inventory only. No Yak Go/binary/output is consulted. The
reference output is evidence, never used to repair source lengths or packets.
"""
from pathlib import Path
import collections,hashlib,importlib.util,json,os,subprocess,sys,time,xml.etree.ElementTree as ET
root=Path(__file__).parent;out=root/sys.argv[1];out.mkdir(exist_ok=False)
def sha(p):
 with Path(p).open('rb')as f:return hashlib.file_digest(f,'sha256').hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
base=root/'OSPFv3-independent-source37-a82-01';doc=read(base/'inputs01/inputs.json');rp=Path(doc['capture_reader']);assert sha(rp)==doc['capture_reader_SHA256'];sys.path[:0]=[str(base),str(rp.parent)]
from scan_source37 import Census
spec=importlib.util.spec_from_file_location('RSVP_original_capture_reader',rp);reader=importlib.util.module_from_spec(spec);spec.loader.exec_module(reader)
spec=importlib.util.spec_from_file_location('RSVP_common_source',root/'census_RSVP_source45.py');source=importlib.util.module_from_spec(spec);spec.loader.exec_module(source)
cap=root/'RSVP-primary-download45-01/rsvp-PATH-RESV.pcap';assert sha(cap)=='aa7d8448cd5eaf17112e67adf3e801cb45119e62c63c75f132ad596edfc10361'
tshark=Path(r'C:\tmp\yak-shark-protocol-audit-20261024\wireshark-portable\App\Wireshark\tshark.exe');assert sha(tshark)=='662e10fd28a8c58575fe567845d0248080a2856c3d18d6f2ff7a4b4c8511f69b'
inputs={str(p):sha(p)for p in[Path(__file__),rp,base/'scan_source37.py',base/'ospf_source_codec.py',root/'census_RSVP_source45.py',cap,tshark]+[root/f'RSVP-primary-download45-01/rfc{n}.txt'for n in(2205,2961,2747)]}
class V(Census):
 def __init__(self,*args):super().__init__(*args);self.rsvp=[]
 def upper(self,proto,at,end,path,depth):
  if proto==46:self.rsvp.append((at,end,path));return
  return super().upper(proto,at,end,path,depth)
facts=[];counter=collections.Counter();frames=0
for n,r in enumerate(reader.capture_records(cap),1):
 frames+=1;v=V(r['wire'],r,n,counter);v.run()
 for start,end,path in v.rsvp:
  assert all(p['available_end']==p['declared_end']for p in path if p['kind']in('IPv4','IPv6'))
  raw=out/f'frame{n}.frame';raw.write_bytes(r['wire']);pdu=out/f'frame{n}.pdu';pdu.write_bytes(r['wire'][start:end]);observation=source.inspect(pdu.read_bytes())
  assert'refusal'not in observation and observation.get('object_envelopes_complete')and observation['checksum_arithmetic']=='valid'
  facts.append(dict(frame=n,domain={k:r[k]for k in('section','interface','link')},time_ns=r['time_ns'],captured_length=r['captured_length'],original_length=r['original_length'],physical_file=str(raw),physical_SHA256=sha(raw),PDU_file=str(pdu),PDU_SHA256=sha(pdu),PDU_physical_range=[start,end],carrier=path,source=observation))
write(out/'source-facts.json',facts)
config=out/'reference-config';config.mkdir();env=dict(os.environ,APPDATA=str(config),WIRESHARK_CONFIG_DIR=str(config));env.pop('SSLKEYLOGFILE',None)
args=[str(tshark),'-n','-2','-r',str(cap),'-T','pdml'];start=time.monotonic()
with(out/'reference.pdml').open('xb')as f,(out/'reference.stderr').open('xb')as err:r=subprocess.run(args,env=env,stdout=f,stderr=err,timeout=45,creationflags=0x08000000)
assert r.returncode==0
rows=[]
for n,packet in enumerate(ET.parse(out/'reference.pdml').getroot().findall('packet'),1):
 rows.append(dict(frame=n,protocols=[p.attrib.get('name')for p in packet.findall('proto')],fields=[dict(f.attrib)for f in packet.iter('field')if f.attrib.get('name','').startswith('rsvp.')or f.attrib.get('name')=='_ws.expert.message']))
write(out/'reference-fields.json',rows);assert inputs=={p:sha(p)for p in inputs}
result=dict(scope=__doc__,inputs=inputs,inputs_unchanged=True,physical_frames=frames,RSVP_packets=len(facts),message_types=dict(collections.Counter(f['source']['type']for f in facts)),objects=sum(len(f['source']['objects'])for f in facts),source_facts_SHA256=sha(out/'source-facts.json'),reference_fields_SHA256=sha(out/'reference-fields.json'),reference_PDML_SHA256=sha(out/'reference.pdml'),reference_arguments=args,reference_exit=0,reference_seconds=time.monotonic()-start,comparison_not_yet_claimed=True,Go_or_native_or_CLI_read=False)
write(out/'summary.json',result);print(json.dumps(dict(summary=str(out/'summary.json'),SHA256=sha(out/'summary.json'),frames=frames,RSVP=len(facts),objects=result['objects'])))
