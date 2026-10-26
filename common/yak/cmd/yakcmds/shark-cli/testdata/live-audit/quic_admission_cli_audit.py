"""Check complete CLI exports for exactly scoped QUIC remainder presentation edits."""
import argparse,collections,hashlib,itertools,json
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['before','after','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();assert not any((p/'.git').exists()for p in[a.out.resolve(),*a.out.resolve().parents]);checks=0
def check(ok,why):
 global checks
 checks+=1;assert ok,why
def eq(x,y,why):check(x==y,(why,x,y))
def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()
changed_rows=[];changed_paths=collections.Counter();frames=0
with a.before.open(encoding='utf-8-sig')as bf,a.after.open(encoding='utf-8-sig')as af:
 for x,y in itertools.zip_longest(bf,af):
  check(x is not None and y is not None,'same number of CLI frames');old,new=json.loads(x),json.loads(y);frames+=1
  if old==new:continue
  keys={k for k in set(old)|set(new)if old.get(k)!=new.get(k)};check(keys<=set(['info','protocol_scope','protocol_status','protocol_evidence','pdus']),('unexpected row change',frames,keys));eq(len(old.get('pdus',[])),len(new.get('pdus',[])),'same observed PDU count');tails=[]
  for op,np in zip(old.get('pdus',[]),new.get('pdus',[])):
   if op==np:continue
   pk={k for k in set(op)|set(np)if op.get(k)!=np.get(k)};eq(pk,{'summary'},'only unparsed PDU summary changed');eq(np['protocol'],'quic','QUIC remainder');eq(np['status'],'malformed','malformed bytes retained');check(np['summary'].startswith('Unparsed QUIC datagram remainder · offset=')and np['summary'].endswith('authentication not verified'),'visible unparsed range');tails.append(np['id']);changed_paths['pdus[].summary']+=1
  check(bool(tails),'row edit has an actual refused remainder PDU')
  if any(p['status']in['decoded','deferred']for p in new['pdus']):
   eq(new['protocol_status'],'partial','valid packet plus refused tail is partial');eq(new['protocol_scope'],'mixed datagram outcomes','range-aware scope');eq(new['protocol_evidence'],'packet authentication is per packet; unparsed remainder is not verified','no packet-wide verification overstatement');check(new['info'].startswith('Unverified QUIC remainder ('),'warning before long authenticated packet summary')
  else:
   eq(new['protocol_status'],'malformed','entire unparsed datagram is not a partial decoded packet');eq(new['protocol_evidence'],'unparsed bytes after observed QUIC packet; authentication not verified','only connection context survives');check(new['info'].startswith('Unparsed QUIC datagram remainder · offset=0'),'entire refused datagram explicitly starts at zero')
  changed_rows.append({'physical_frame':frames,'tail_PDUs':tails,'changed_root_keys':sorted(keys)})
  for k in keys-{'pdus'}:changed_paths[k]+=1
out={'scope':__doc__,'frames':frames,'checks':checks,'changed_rows':changed_rows,'changed_paths':dict(changed_paths),'unexpected_export_differences':[],'before_sha256':sha(a.before),'after_sha256':sha(a.after)};a.out.parent.mkdir(parents=True,exist_ok=True);a.out.write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps({k:v for k,v in out.items()if k not in['scope','changed_rows']}));print('Changed rows',len(changed_rows))
