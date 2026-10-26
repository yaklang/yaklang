"""Deliberately corrupt a previously verified positive export to test the auditor.
Input is the two-message pinned IOS-XE upstream case, never a rewritten reference.
"""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['events','oracle','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
base=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()]
assert len(base)==2 and base[1]['fields']['Data Record Count']==18
audit=Path(__file__).with_name('ipfix_wire_audit.py');results=[]
for kind in ['value','range','IE-ID','enterprise','schema-hash','record-count','namespace','source-frame','missing','extra','projection-value','projection-range']:
 x=copy.deepcopy(base);m=x[1]['fields'];row=m['FlowSets'][0];f=row['Records'][0]['Fields'][0]
 if kind=='value':f['Value']='198.51.100.9'
 elif kind=='range':f['Wire Field Ranges']['Value'][0]+=1
 elif kind=='IE-ID':f['Field Type']=12
 elif kind=='enterprise':f['Enterprise']=True
 elif kind=='schema-hash':row['Template PDU SHA256']='0'*64
 elif kind=='record-count':m['Data Record Count']-=1
 elif kind=='namespace':x[1]['protocol']='netflow-v9'
 elif kind=='source-frame':x[1]['source_bytes']['CompletedIn']['Number']=1
 elif kind=='missing':x.pop()
 elif kind=='extra':x.append(copy.deepcopy(x[-1]))
 elif kind.startswith('projection-'):
  v=next(v for v in x[1]['field_values']['values'] if v['name']=='cflow.srcaddr')
  if kind=='projection-value':v['value']='198.51.100.9'
  else:v['pdu_range']['offset']+=1
 p=a.out/(kind+'.jsonl');p.write_text(''.join(json.dumps(y)+'\n' for y in x),encoding='utf8')
 r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--oracle',str(a.oracle),'--events',str(p),'--out',str(a.out/(kind+'-unexpected.json'))],capture_output=True)
 assert r.returncode!=0,('audit accepted corruption',kind)
 results.append({'corruption':kind,'detected':True})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'corruptions detected')
