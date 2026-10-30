"""Freeze literal field-selection expectations from the independent wire ledger."""
from pathlib import Path
import hashlib,json,struct
P=Path(__file__).parent
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
ledger=P/'SSH-wire-ranges-source51-01/range-plan.json'
assert sha(ledger)=='e74cc22176d251d38015ceb1a2eff19393d3f377a8462adcf2d032ee538e4a48'
rows={r['frame']:r for r in json.loads(ledger.read_text('utf8'))if r['dataset']=='source50'}
capture=P/'SSH-role-source50-01/plan02/ssh-role-source50-authored9.pcap'
assert sha(capture)=='22f3a36ed8dce4fa4cead19aeba75a9c4632df0a5f9eaf433c62d22e2adf906a'
wire=capture.read_bytes();frames=[];at=24
while at<len(wire):
 _,_,n,_=struct.unpack('<IIII',wire[at:at+16]);at+=16;frames.append(wire[at:at+n]);at+=n
assert len(frames)==115
cases=[]
for name,frame,field,value,path,down in[
 ('reply-host-format',9,'Host Key Format','ssh-ed25519','Host Key Format',0),
 ('reply-signature-format',9,'Signature Format','ssh-ed25519','Payload/Signature/Signature Format',50),
 ('KEXINIT-selected-context',7,'Encryption C2S','aes128-ctr',None,0),
]:
 r=rows[frame];physical=frames[frame-1];raw=Path(r['raw_path']).read_bytes()
 assert physical[54:]==raw and sha(r['raw_path'])==r['raw_SHA256']
 assert hashlib.sha256(physical).hexdigest()==r['physical_frame_SHA256']
 span=None
 if path:
  ranges=r['Session_current_literal_ranges']if '/'not in path else r['API_fields_ranges']
  span=ranges[path];assert raw[span[0]:span[1]]==value.encode('ascii')
 else:
  assert field not in r['Session_current_literal_ranges']
 cases.append(dict(name=name,frame=frame,sourcecase=r['sourcecase'],capture=str(capture),capture_SHA256=sha(capture),raw_path=r['raw_path'],raw_SHA256=r['raw_SHA256'],physical_SHA256=r['physical_frame_SHA256'],physical_bytes=len(physical),PDU_bytes=len(raw),select_field=field,select_value=value,field_down=down,expected_PDU_span=span,expected_selected_source='owned reassembled PDU'if span else'unchanged full physical frame',expected_protocol='SSH',expected_stack=['Ethernet','IPv4','TCP','SSH'],word='KEXINIT'if frame==7 else'Host Key Format'))
out=P/'Windows-SSH-ranges-source-plan53-01';out.mkdir(exist_ok=False)
plan=dict(source_ledger_SHA256=sha(ledger),authored_not_live=True,physical_frames=115,default_cache_packets=20000,default_history_mib=64,expected_not_from_candidate=True,cases=cases)
(out/'plan.json').write_text(json.dumps(plan,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(cases=len(cases),plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'))))
