"""Freeze Windows source expectations for a wrong sender and an opaque preview."""
from pathlib import Path
import hashlib,json,struct
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
plan=root/'SSH-role-source50-01/plan02'
assert sha(plan/'manifest.json')=='ae73e1329488cc159cb0958f483c529e7dd50e270b53249dae8521324a2553af'
m=json.loads((plan/'manifest.json').read_text('utf8'))
capture=Path(m['capture']['path']);assert sha(capture)==m['capture']['SHA256']
facts=json.loads((plan/'source-facts.json').read_text('utf8'))['cases']
steps={s['frame']:(c,s)for c in facts for s in c['steps']}
wire=capture.read_bytes();at=24;physical={}
while at<len(wire):
 _,_,n,_=struct.unpack('<IIII',wire[at:at+16]);at+=16;physical[len(physical)+1]=wire[at:at+n];at+=n
assert len(physical)==115
cases=[]
for number,name,preview in[(36,'wrong-client-reply',False),(13,'protected-preview',True)]:
 c,s=steps[number];raw=Path(s['input_path']).read_bytes();frame=physical[number]
 assert frame[54:]==raw and len(raw)==192
 retained=raw[:64]if preview else raw
 cases.append(dict(name=name,frame=number,sourcecase=c['sourcecase'],capture=str(capture),capture_SHA256=sha(capture),
  full_physical_bytes=len(frame),full_physical_SHA256=hashlib.sha256(frame).hexdigest(),
  logical_source_bytes=len(raw),logical_source_SHA256=sha(s['input_path']),raw_path=s['input_path'],
  retained_bytes=len(retained),retained_SHA256=hashlib.sha256(retained).hexdigest(),preview_only=preview,
  expected_protocol='SSH',expected_stack=['Ethernet','IPv4','TCP','SSH'],
  expected_status='encrypted'if preview else'malformed',
  query='fields pdu.id == <positive unique source-verified event id>',
  required_text=['NEWKEYS','plaintext unavailable']if preview else['KEY_EXCHANGE_31 (unexpected sender)','Expected Sender Role: server','Observed Sender Role: client'],
  select_field=''if preview else'Expected Sender Role',select_value=''if preview else'server',
  forbidden_text=['Host Key Fingerprint:','Host Key SHA256:','Host Key Format:','Server Curve25519 Public Key:'],
  byte_scope='Full captured frame remains available; SDK encrypted source is a retained64 preview with logical length192.'if preview else'Full captured frame and full192-byte reassembled source, with refused method fields.',
  no_claimed_range_for_role_metadata=True))
out=root/'Windows-SSH-role-source-plan50-01';out.mkdir(exist_ok=False)
with(out/'plan.json').open('x',encoding='utf8')as f:json.dump(dict(source_manifest_SHA256=sha(plan/'manifest.json'),
 source_cases=9,physical_frames=115,default_cache_packets=20000,default_history_mib=64,
 authored_not_live=True,cases=cases),f,indent=2);f.write('\n')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'))))
