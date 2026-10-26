"""Corrupt independently captured HTTP3 body observations and require rejection.
All mutated exports stay outside the checkout; no network replay is modified.
"""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists()for p in[a.out.resolve(),*a.out.resolve().parents])
base=[json.loads(s)for s in a.events.read_text(encoding='utf-8-sig').splitlines()];audit=Path(__file__).with_name('http3_body_wire_audit.py')
def run(ep,mp,op,out):return subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--manifest',str(mp),'--oracle',str(op),'--events',str(ep),'--out',str(out)],capture_output=True)
r=run(a.events,a.manifest,a.oracle,a.out/'positive.json');assert r.returncode==0,r.stderr.decode('utf8','replace')
results=[]
kinds=['observed-count','declared-length','normalization','final-FIN','length-verdict','header-PDU','header-flow','header-domain','header-direction','context-stream','context-direction','source-offset','wire-FIN','app-length','app-bytes','header-value','missing-app','duplicate-app','protocol','transport','ALPN','authentication','flow-generation','missing-error','peer-receipt','reference-packet']
for kind in kinds:
 es=copy.deepcopy(base)
 def stream_rows():return[(e,f)for e in es for f in(e.get('session')or{}).get('Frames',[])if f.get('Frame Type')=='STREAM']
 def heads():return[(e,f,h)for e,f in stream_rows()for h in f.get('HTTP3 Frames',[])if h['Frame Type']=='HEADERS']
 e,f=next((e,f)for e,f in stream_rows()if(f.get('HTTP3 Message Conformance')or{}).get('Length Check')=='matched');c=f['HTTP3 Message Conformance'];mp,op=a.manifest,a.oracle
 if kind=='observed-count':c['Observed DATA Bytes']+=1
 elif kind=='declared-length':c['Declared Content-Length']='999999'
 elif kind=='normalization':c['Length Normalization']='invented'
 elif kind=='final-FIN':c['Drained FIN']=False
 elif kind=='length-verdict':c['Length Check']='not-declared'
 elif kind=='header-PDU':c['Header PDU ID']=next(x['id']for x in es if x['id']<e['id']and not(x.get('session')or{}).get('Frames'))
 elif kind in['header-flow','header-domain','header-direction']:
  he=next(x for x in es if x['id']==c['Header PDU ID'])
  if kind=='header-flow':he['flow']+=1000
  elif kind=='header-domain':he['domain']['Section']+=1
  else:he['direction']=1-he['direction']
 elif kind=='context-stream':c['Stream ID']+=4
 elif kind=='context-direction':c['Direction']=1-c['Direction']
 elif kind=='source-offset':f['Byte Source']['Offset']+=1
 elif kind=='wire-FIN':f['FIN']=not f['FIN']
 elif kind in['app-length','app-bytes','header-value','missing-app','duplicate-app']:
  e,f,h=heads()[0]
  if kind=='app-length':h['Payload Length']+=1
  elif kind=='app-bytes':h['QPACK Block']='AA=='
  elif kind=='header-value':h['Headers'][0]['Value']='ALTERED'
  elif kind=='missing-app':f['HTTP3 Frames'].remove(h)
  else:f['HTTP3 Frames'].append(copy.deepcopy(h))
 elif kind=='protocol':e['protocol']='tcp'
 elif kind=='transport':e['transport']='tcp'
 elif kind=='ALPN':e['session']['ALPN']='other'
 elif kind=='authentication':e['session']['Authentication Verified']=False
 elif kind=='flow-generation':e['flow']+=10000
 elif kind=='missing-error':
  # Remove every stream-local diagnostic for one declared malformed direction.
  for ee,ff in stream_rows():
   if ee['flow']==1 and ee['direction']==0 and ff['Stream ID']==4:ff.pop('HTTP3 Stream Error',None)
 elif kind=='peer-receipt':
  m=json.loads(a.manifest.read_text());m['operations'][0]['request_hex']='00'+m['operations'][0]['request_hex'][2:];mp=a.out/'peer-receipt.json';mp.write_text(json.dumps(m),encoding='utf8')
 elif kind=='reference-packet':
  ref=json.loads(a.oracle.read_text());node=next(x['_source']['layers']for x in ref if'quic'in x['_source']['layers']);node['frame_raw'][0]='ff'+node['frame_raw'][0][2:];op=a.out/'reference-packet.json';op.write_text(json.dumps(ref),encoding='utf8')
 ep=a.out/(kind+'.jsonl');ep.write_text(''.join(json.dumps(x)+'\n'for x in es),encoding='utf8');r=run(ep,mp,op,a.out/(kind+'-unexpected.json'))
 assert r.returncode!=0 and b'AssertionError'in r.stderr,(kind,r.returncode,r.stderr.decode('utf8','replace'))
 results.append({'corruption':kind,'detected':'rejected'})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'corruptions rejected')
