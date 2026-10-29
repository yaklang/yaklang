"""Freeze two native-console cases from source facts, before candidate UI use.

One exact official packet and one authored two-message datagram; neither is a
new live capture. Field identities, values and spans come only from frozen
source plans. Outputs, capture bytes and screenshots remain outside Git.
"""
from pathlib import Path
import hashlib,json,struct
root=Path(__file__).parent;out=root/'Windows-GTPv2-source-plan44-01';out.mkdir(exist_ok=False)
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def write(p,v):p.write_text(json.dumps(v,indent=2)+'\n',encoding='utf8')
def records(p):
 data=Path(p).read_bytes();assert data[:4]==bytes.fromhex('d4c3b2a1');at=24;rows=[]
 assert struct.unpack_from('<I',data,20)[0]==1
 while at<len(data):
  sec,usec,cap,orig=struct.unpack_from('<IIII',data,at);at+=16
  raw=data[at:at+cap];assert len(raw)==cap;at+=cap;rows.append((sec,usec,orig,raw))
 assert at==len(data);return rows
official=root/'GTP-official-download43-01/project-upload.pcap'
facts=root/'GTP-official-source43-02/official-source-facts.json'
grid=root/'GTPv2-boundary-source43-01/plan03/source-grid.json'
authored=grid.with_name('authored-controls.pcap')
assert sha(official)=='b62192ea1eba8e3bce2bff600222ce1ac41caf01867f0336a026f161da355030'
assert sha(facts)=='6b1fc5ec07ebf0467064754501dbba70fd74656fec8fde17129a373d34a958b8'
assert sha(grid)=='a576ac2e5d6b5128475c217b574e27453558b2742a063fd82c0db4e6875ba0e2'
names=read(root/'GTPv2-boundary-source43-01/message-type-names.json')['names']
fact=read(facts)[1];assert fact['frame']==2
a,b=fact['bounded_messages'][0]['field_spans']['type']
wire=next(x for x in read(grid)if x['case']==5)['wires'][0]
leaf=next(x for x in wire['source_leaves']if x['path']==['Messages',1,'Message Type'])
cases=[]
for name,source,frame,physical_sha,pdu_span,field_span,typ,down,proof in [
 ('Official-Create-Bearer',official,2,fact['original_frame_SHA256'],fact['payload_physical_span'],[a,b],fact['bounded_messages'][0]['type'],8,facts),
 ('Piggyback-Second-Message',authored,wire['authored_capture_frame'],wire['captured_frame_SHA256'],[42,42+wire['captured_PDU_bytes']],leaf['span'],leaf['value'],42,grid)]:
 sec,usec,orig,raw=records(source)[frame-1];assert hashlib.sha256(raw).hexdigest()==physical_sha
 first,last=pdu_span;pdu=raw[first:last];lo,hi=field_span;assert pdu[lo:hi]==bytes([typ])
 physical=out/(name+'.frame');physical.write_bytes(raw)
 capture=out/(name+'.pcap')
 capture.write_bytes(struct.pack('<IHHIIII',0xa1b2c3d4,2,4,0,0,65535,1)+struct.pack('<IIII',sec,usec,len(raw),orig)+raw)
 helper=root/'windows-PIM-helper26/native-console.exe';renderer=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
 cases.append(dict(name=name,original_public_capture=str(source),original_public_capture_SHA256=sha(source),original_public_frame=frame,source_grid=str(proof),source_grid_SHA256=sha(proof),capture=str(capture),capture_SHA256=sha(capture),physical_file=str(physical),physical_SHA256=physical_sha,physical_bytes=len(raw),PDU_range=pdu_span,PDU_SHA256=hashlib.sha256(pdu).hexdigest(),PDU_bytes=len(pdu),query='proto:gtpv2',field_name='Message Type',field_value=f'{typ} ({names[str(typ)].strip()})',field_down=down,field_PDU_range=field_span,field_physical_range=[first+lo,first+hi],field_hex=pdu[lo:hi].hex(),progressive_stack=['Ethernet','IPv4','UDP','GTPv2-C'],helper=str(helper),helper_SHA256=sha(helper),renderer=str(renderer),renderer_SHA256=sha(renderer),packet_cache_default=20000,history_budget_default_mib=64,derived_one_packet_not_live=True,authored_control=source==authored))
write(out/'cases.json',cases)
write(out/'plan.json',dict(scope=__doc__,cases_SHA256=sha(out/'cases.json'),candidate_read=False,source_cases=2,console_cells_not_terminal_pixels=True,constructor_SHA256=sha(__file__)))
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),cases=[dict(name=c['name'],frame=c['original_public_frame'],selected=c['field_PDU_range'])for c in cases])))
