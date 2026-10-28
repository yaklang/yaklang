"""Whole-row ICMP presentation alias audit; native identities remain literal.

Consumes the source-frozen 02d2-to-fef whole-CLI differential. This proves label
placement only. It does not claim additional packet or message decoding. Real
repeated IP/ICMP layers remain in their original order and spelling except for
the first matching physical ICMPv4 alias selected by the native ICMP event.
"""
from pathlib import Path
import argparse, copy, hashlib, json

def sha(p):
 h=hashlib.sha256()
 with p.open('rb')as f:
  for b in iter(lambda:f.read(1<<20),b''):h.update(b)
 return h.hexdigest()

def same(x,y):
 if type(x)is not type(y):return False
 if isinstance(x,dict):return x.keys()==y.keys()and all(same(x[k],y[k])for k in x)
 if isinstance(x,list):return len(x)==len(y)and all(same(a,b)for a,b in zip(x,y))
 return x==y

def expected(old):
 label=old['protocol'];family={'ICMP':'icmp','ICMPV6':'icmpv6'}[label]
 # CLI PDUs are the exported field projections, not full SDK events. Their
 # complete literal equality is checked; SDK transport/source ownership is
 # established by the separate complete native-generation certificate.
 assert any(p['protocol']==family and p['status']=='decoded'and p['completeness']=='message'for p in old.get('pdus')or[])
 out=copy.deepcopy(old);stack=out['protocol_stack']
 assert stack[-1]==label
 stack.pop()
 if family=='icmp':
  index=stack.index('ICMPv4');stack[index]='ICMP'
 else:
  assert 'ICMPv6'in stack
  out['protocol']='ICMPv6'
 return out

def main():
 ap=argparse.ArgumentParser(description=__doc__)
 ap.add_argument('--diff',type=Path,required=True);ap.add_argument('--out',type=Path,required=True)
 a=ap.parse_args();a.out.mkdir(exist_ok=False)
 full=json.loads((a.diff/'summary.json').read_text())
 assert len(full)==37 and {v['case']for v in full if v['changed']}=={'weber','sharon'}
 results=[];positives={}
 for corpus,count in[('weber',5476),('sharon',5495)]:
  ledger=a.diff/(corpus+'-changed.jsonl');n=0;families={}
  for line in ledger.open(encoding='utf8'):
   v=json.loads(line);old,new=v['old'],v['new']
   assert old['number']==new['number']==v['frame']
   want=expected(old);assert same(want,new),(corpus,v['frame'],'whole alias projection mismatch')
   n+=1;families[old['protocol']]=families.get(old['protocol'],0)+1
   positives.setdefault(old['protocol'],(old,new))
  assert n==count
  results.append(dict(corpus=corpus,whole_expected=n,families=families,complete_changed_ledger_SHA256=sha(ledger),parent_case_summary_SHA256=sha(a.diff/(corpus+'.json'))))
 controls=[]
 for family,(old,new)in positives.items():
  for name in['protocol','source','native-PDU','layer-deletion','duplicate-IP','type']:
   forged=copy.deepcopy(new)
   if name=='protocol':forged['protocol']='TCP'
   elif name=='source':forged['source']='203.0.113.254'
   elif name=='native-PDU':forged['pdus']=[{'id':True}]
   elif name=='layer-deletion':forged['protocol_stack']=forged['protocol_stack'][1:]
   elif name=='duplicate-IP':forged['protocol_stack'].insert(1,'IPv4')
   else:forged['number']=str(forged['number'])
   assert not same(expected(old),forged)
   controls.append(family+'/'+name)
 summary=dict(scope=__doc__,before_source='02d2e54263a8cc501ef9f278896da428379ddb52',after_source='fef614bae17b2d9b12e337baa5eeebab09fead05',complete_generations=37,whole_expected=10971,native_PDUs_whole_literal=True,cases=results,negative_controls=controls,parent_complete_summary_SHA256=sha(a.diff/'summary.json'),auditor_SHA256=sha(Path(__file__)))
 (a.out/'summary.json').write_text(json.dumps(summary,indent=2)+'\n',encoding='utf8');print(json.dumps(summary),flush=True)

if __name__=='__main__':main()
