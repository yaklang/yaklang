"""Reject corrupt framing verdicts, connection history and literal byte evidence."""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['capture','manifest','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True);assert not any((p/'.git').exists()for p in[a.out.resolve(),*a.out.resolve().parents]);base=[json.loads(s)for s in a.events.read_text(encoding='utf-8-sig').splitlines()];audit=Path(__file__).with_name('http3_fin_frame_wire_audit.py')
def run(ep,out):return subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--manifest',str(a.manifest),'--oracle',str(a.oracle),'--events',str(ep),'--out',str(out)],capture_output=True)
r=run(a.events,a.out/'positive.json');assert r.returncode==0,r.stderr.decode('utf8','replace')
results=[]
for kind in['code','scope','fault-PDU','direction','offset','component','partial-bytes','partial-digest','source-parent','source-domain','source-hole','sticky-erased','sticky-generation','field-alias','protocol','NeedMore','wire-FIN','missing-frame','duplicate-frame','request-association','source-root-stream']:
 es=copy.deepcopy(base)
 def rows():return[(e,f)for e in es for f in(e.get('session')or{}).get('Frames',[])if'HTTP3 Frame Error'in f]
 e,f=rows()[0];fault=f['HTTP3 Frame Error'];root=e['session']['HTTP3 Connection Error']
 if kind=='code':fault['Error Code']=0x101
 elif kind=='scope':fault['Error Scope']='stream'
 elif kind=='fault-PDU':fault['First PDU ID']+=1
 elif kind=='direction':fault['Direction']=1-fault['Direction']
 elif kind=='offset':fault['Frame Offset']+=1
 elif kind=='component':fault['Incomplete Component']='invented'
 elif kind=='partial-bytes':fault['Partial Frame Bytes']='AA=='
 elif kind=='partial-digest':fault['Partial Frame SHA256']='0'*64
 elif kind=='source-parent':fault['Byte Source']['Ranges'][0]['Parent PDU']=es[0]['id']
 elif kind=='source-domain':fault['Byte Source']['Ranges'][0]['Packet References'][0]['Encapsulation']='forged'
 elif kind=='source-hole':fault['Byte Source']['Ranges'][0]['Length']=0
 elif kind=='source-root-stream':fault['Byte Source']['Stream ID']+=4
 elif kind in['sticky-erased','sticky-generation']:
  later=next(x for x in es if x['flow']==e['flow']and x['id']>e['id']and x.get('session')and not any('HTTP3 Frame Error'in ff for ff in x['session'].get('Frames',[])))
  if kind=='sticky-erased':later['session'].pop('HTTP3 Connection Error')
  else:later['session']['HTTP3 Connection Error']['First PDU ID']=next(x['id']for x,ff in rows()if x['flow']!=e['flow'])
 elif kind=='field-alias':next(v for v in e['field_values']['values']if v['name']=='http3.connection.error_code')['value']=0
 elif kind=='protocol':e['protocol']='quic'
 elif kind=='NeedMore':e['expert']='NeedMore'
 elif kind=='wire-FIN':f['FIN']=not f['FIN']
 elif kind in['missing-frame','duplicate-frame']:
  ee,ff=next((x,ff)for x in es for ff in(x.get('session')or{}).get('Frames',[])if ff.get('HTTP3 Frames')and'HTTP3 Frame Error'not in ff and ff.get('Stream ID',2)&2==0)
  if kind=='missing-frame':ff['HTTP3 Frames'].pop(0)
  else:ff['HTTP3 Frames'].append(copy.deepcopy(ff['HTTP3 Frames'][0]))
 elif kind=='request-association':
  ee,hh=next((x,h)for x in es if x['direction']==1 for ff in(x.get('session')or{}).get('Frames',[])if ff.get('Stream ID')==f['Stream ID']for h in ff.get('HTTP3 Frames',[])if h['Frame Type']=='HEADERS');hh['Associated Request']=True
 ep=a.out/(kind+'.jsonl');ep.write_text(''.join(json.dumps(x)+'\n'for x in es),encoding='utf8');r=run(ep,a.out/(kind+'-unexpected.json'));assert r.returncode!=0 and b'AssertionError'in r.stderr,(kind,r.returncode,r.stderr.decode('utf8','replace'));results.append({'corruption':kind,'detected':'rejected'})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'FIN/layout corruptions rejected')
