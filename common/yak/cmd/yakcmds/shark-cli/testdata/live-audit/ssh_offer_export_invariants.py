"""Constrain full-public-export changes to literal SSH offer names and evidence.
Every original raw byte, field tree, connection, transaction and other protocol
must remain identical. This is a differential contract for the public inputs;
independent role correctness is certified separately on actual owned traffic.
"""
import argparse,base64,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','out']:ap.add_argument('--'+n,type=Path,required=True)
ap.add_argument('--kind',choices=['SDK','CLI'],required=True);a=ap.parse_args();checks=0
for p in[a.before,a.after,a.out]:assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,why)
def rows(p):
 with p.open(encoding='utf-8-sig')as f:
  for line in f:yield json.loads(line)
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for w in iter(lambda:f.read(1<<20),b''):h.update(w)
 return h.hexdigest()
context='client/server role not observed';alias='ssh.negotiation.context';names=['Kex Algorithms','Host Key Algorithms','Encryption C2S','Encryption S2C','MAC C2S','MAC S2C','Compression C2S','Compression S2C','Languages C2S','Languages S2C']
def wire_offers(e):
 w=base64.b64decode(e['raw']);check(len(w)>=16 and len(w)%8==0,'complete initial packet');eq(int.from_bytes(w[:4],'big'),len(w)-4,'literal packet length');body=w[5:len(w)-w[4]];eq(body[0],20,'literal KEXINIT code');at=17;out={}
 for name in names:
  check(at+4<=len(body),'literal name-list length');n=int.from_bytes(body[at:at+4],'big');at+=4;check(n<=len(body)-at,'literal name-list body');out[name]=body[at:at+n].decode('ascii');at+=n
 eq(len(body)-at,5,'complete flags/reserved');return out
def clean_alias(e):
 f=e.get('field_values')or{};vals=f.get('values',[]);added=[v for v in vals if v['name']==alias]
 if added:
  eq(len(added),1,'one role evidence field');eq(added[0]['value'],context,'literal evidence limitation');eq(added[0]['origin'],'context','context field has no invented wire origin');check('pdu_range'not in added[0],'no fake context byte range');f['values']=[v for v in vals if v['name']!=alias]
 if alias in(e.get('fields')or{}):eq(e['fields'].pop(alias),[context],'matching compact context field')
 return len(added)
count=0;changed=0;kex_count=0;contexts=0;frames=[]
for b,c in itertools.zip_longest(rows(a.before),rows(a.after)):
 check(b is not None and c is not None,'complete equal record count');count+=1
 if b==c:continue
 changed+=1
 if a.kind=='SDK':
  eq((b['protocol'],c['protocol']),('ssh','ssh'),('only SSH metadata changes',count));old=b.pop('session');new=c.pop('session');eq((old['Packet Name'],new['Packet Name']),('KEXINIT','KEXINIT'),'only KEXINIT offer projections change');offers=wire_offers(c);kex_count+=1
  for name in names[2:8]:
   eq(new.pop('Offered '+name),offers[name],('literal independently decoded sender offer',count,name))
   if name in new:
    check(bool(new.get('Kex Algorithm')),'selected field needs existing negotiated KEX context');eq(old[name],new[name],('original selected algorithm retained',count,name))
   else:eq(old.pop(name),offers[name],('original raw offer renamed',count,name))
  if'Negotiation Context'in new:
   eq(new.pop('Negotiation Context'),context,'explicit evidence limitation');eq(old.pop('Context Level'),'observed','prior context marker');eq(new.pop('Context Level'),'partial','evidence limitation explicit');eq(clean_alias(c),1,'queryable role limitation');contexts+=1
  eq(old,new,('all remaining SSH state retained',count));eq(b,c,('all bytes, trees, metadata and original typed fields retained',count));frames.append({'PDU':c['id'],'physical_frames':[r['Number']for r in c['source_bytes']['PacketRefs']]})
 else:
  bs=b.pop('pdus',[]);cs=c.pop('pdus',[]);eq(len(bs),len(cs),'same packet PDUs');added=0
  for old,new in zip(bs,cs):
   if old==new:continue
   eq((old['protocol'],new['protocol']),('ssh','ssh'),'only native SSH query metadata changes');added+=clean_alias(new);eq(old,new,'all original PDU scalar fields retained')
  check(added>0,'changed row needs explicit new role context');old_info=b.pop('info');new_info=c.pop('info');expected=old_info.replace('KEXINIT · ','KEXINIT · '+context+' · ',1)if'KEXINIT · 'in old_info else old_info+' · '+context;eq(new_info,expected,'single explanatory summary insertion');eq(b,c,'all other physical row fields identical');contexts+=added;frames.append({'physical_frame':c['number']})
out={'scope':__doc__,'kind':a.kind,'checks':checks,'records':count,'changed_records':changed,'KEXINIT_offer_projections':kex_count,'added_role_contexts':contexts,'changed_source_records':frames,'before_sha256':sha(a.before),'after_sha256':sha(a.after),'other_differences':[]};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n');print(json.dumps({k:v for k,v in out.items()if k not in['scope','changed_source_records']}))
