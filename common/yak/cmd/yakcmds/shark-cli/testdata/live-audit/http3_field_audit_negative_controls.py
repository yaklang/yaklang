"""Corrupt real-socket HTTP3 field observations; verify rejection/difference signals."""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
base=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()];reference=json.loads(a.oracle.read_text(encoding='utf-8-sig'));audit=Path(__file__).with_name('http3_field_wire_audit.py')
def run(ep,op,out):return subprocess.run([sys.executable,'-X','utf8',str(audit),'--manifest',str(a.manifest),'--oracle',str(op),'--events',str(ep),'--out',str(out)],capture_output=True)
r=run(a.events,a.oracle,a.out/'positive.json');assert r.returncode==0,r.stderr.decode('utf8','replace')
results=[]
kinds=['name','value','status','digest','protocol','transport','authentication','ALPN','flow','direction','stream-id','missing','duplicate','bad-diagnostic','response-association']
for kind in kinds:
 events=copy.deepcopy(base)
 def heads():return [(e,fr,h) for e in events for fr in (e.get('session')or{}).get('Frames',[]) for h in fr.get('HTTP3 Frames',[]) if h['Frame Type']=='HEADERS']
 e,fr,h=heads()[0]
 if kind=='name':h['Headers'][0]['Name']='x-altered'
 elif kind=='value':h['Headers'][0]['Value']='ALTERED'
 elif kind=='status':h['Header Conformance Status']='malformed'
 elif kind=='digest':h['Frame SHA256']='0'*64
 elif kind=='protocol':e['protocol']='ssh'
 elif kind=='transport':e['transport']='tcp'
 elif kind=='authentication':e['session']['Authentication Verified']=False
 elif kind=='ALPN':e['session']['ALPN']='other'
 elif kind=='flow':e['flow']+=10000
 elif kind=='direction':h['Direction']=1-h['Direction']
 elif kind=='stream-id':h['Stream ID']+=1000
 elif kind=='missing':fr['HTTP3 Frames'].remove(h)
 elif kind=='duplicate':fr['HTTP3 Frames'].append(copy.deepcopy(h))
 elif kind=='bad-diagnostic':
  _,_,h=next((e,f,h) for e,f,h in heads() if h['Header Conformance Status']=='malformed');h['Header Conformance Error']=''
 elif kind=='response-association':
  _,_,h=next((e,f,h) for e,f,h in heads() if h['Direction']==1 and h.get('Associated Request')is not True);h['Associated Request']=True
 ep=a.out/(kind+'.jsonl');ep.write_text(''.join(json.dumps(x)+'\n' for x in events),encoding='utf8');r=run(ep,a.oracle,a.out/(kind+'-unexpected.json'))
 assert r.returncode!=0 and b'AssertionError' in r.stderr,(kind,r.returncode,r.stderr.decode('utf8','replace'))
 results.append({'corruption':kind,'detected':'rejected'})

def field_nodes(x):
 if isinstance(x,list):
  for v in x:yield from field_nodes(v)
 elif isinstance(x,dict):
  if 'http3.headers.header.value_raw' in x:yield x
  for v in x.values():yield from field_nodes(v)
oracle=copy.deepcopy(reference);node=next(field_nodes(oracle));old=node['http3.headers.header.value_raw'][0];node['http3.headers.header.value_raw'][0]=('00' if old[:2]!='00' else '01')+old[2:]
op=a.out/'reference-value.json';op.write_text(json.dumps(oracle),encoding='utf8');out=a.out/'reference-value-observed.json';r=run(a.events,op,out)
assert r.returncode==0,r.stderr.decode('utf8','replace')
assert json.loads(out.read_text())['reference_raw_field_differences'],'reference discrepancy was silently accepted'
results.append({'corruption':'reference-value','detected':'explicit retained field difference'})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'corruptions detected')
