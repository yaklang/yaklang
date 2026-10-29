"""Extend immutable role50 Windows controls with a fully retained opaque slice."""
from pathlib import Path
import copy,hashlib,json,struct
root=Path(__file__).parent
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
old=root/'Windows-SSH-role-source-plan50-01/plan.json'
assert sha(old)=='37b2eea6c83bc639ecc4f222aad82f061458e4c95b6aae1b8921a3fa95816cf7'
p=copy.deepcopy(json.loads(old.read_text('utf8')))
manifest=root/'SSH-role-source50-01/plan02/manifest.json'
assert sha(manifest)=='ae73e1329488cc159cb0958f483c529e7dd50e270b53249dae8521324a2553af'
facts=json.loads((manifest.parent/'source-facts.json').read_text('utf8'))['cases']
case,step=next((c,s)for c in facts for s in c['steps']if s['frame']==11)
raw=Path(step['input_path']).read_bytes();assert len(raw)==48 and step['kind']=='protected-sentinel'
capture=Path(p['cases'][0]['capture']).read_bytes();at=24;frames=[]
while at<len(capture):
 _,_,n,_=struct.unpack('<IIII',capture[at:at+16]);at+=16;frames.append(capture[at:at+n]);at+=n
full=copy.deepcopy(p['cases'][1]);full.update(name='protected-full-slice',frame=11,sourcecase=case['sourcecase'],
 full_physical_bytes=len(frames[10]),full_physical_SHA256=hashlib.sha256(frames[10]).hexdigest(),
 logical_source_bytes=48,logical_source_SHA256=sha(step['input_path']),raw_path=step['input_path'],
 retained_bytes=48,retained_SHA256=sha(step['input_path']),preview_only=False,
 byte_scope='Full48-byte post-NEWKEYS slice is observed opaque stream bytes, not an independently decoded SSH packet.')
assert frames[10][54:]==raw
p['cases'].append(full)
for c in p['cases']:
 c['opaque']=c['name']!='wrong-client-reply'
 if c['opaque']:
  c['required_group_kind']='observation'
  c['source_caption']='SSH · encrypted stream '+('preview · '+str(c['retained_bytes'])+'/'+str(c['logical_source_bytes'])+' observed bytes'if c['preview_only']else'bytes · '+str(c['retained_bytes'])+' observed bytes')
 else:c.update(required_group_kind='PDU',source_caption='SSH · reassembled PDU')
p.update(old_source_plan_SHA256=sha(old),new_source_expectation='Only caption/scope changes; existing64 SDK source facts, raw/sample lengths, physical frame metadata and private identities remain unchanged.',
 no_complete_PDU_claim_for_fully_retained_opaque=True,plan_owner='UI preview51, distinct from the future wire-range proposal source51')
out=root/'Windows-SSH-preview-source-plan51-01';out.mkdir(exist_ok=False)
with(out/'plan.json').open('x',encoding='utf8')as f:json.dump(p,f,indent=2);f.write('\n')
print(json.dumps(dict(plan=str(out/'plan.json'),SHA256=sha(out/'plan.json'),cases=len(p['cases']))))
