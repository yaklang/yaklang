"""Reject independently corrupted HTTP/3 source graphs and reference bindings.
All mutated exports and references stay outside Git; the original capture is read-only.
"""
import argparse,base64,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
base=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()]
reference=json.loads(a.oracle.read_text(encoding='utf-8-sig'));audit=Path(__file__).with_name('quic_source_graph_audit.py')

def run(events,oracle,out):
 return subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--oracle',str(oracle),'--events',str(events),'--out',str(out)],capture_output=True)

positive=run(a.events,a.oracle,a.out/'positive.json')
assert positive.returncode==0,positive.stderr.decode('utf8','replace')
results=[]
kinds=['source-offset','source-length','source-kind','source-direction','range-offset','range-length','range-parent','range-packet','range-domain','event-domain','event-endpoint','authentication','coverage-flag','parent-set','packet-set','frame-type','frame-length','frame-header','header-selection','frame-hash','wire-length','duplicate-frame','missing-frame','typed-type','typed-length','delivered-offset','raw-stream-offset','oracle-physical-bytes','coalesced-DCID-ambiguity']
for kind in kinds:
 events=copy.deepcopy(base);oracle=copy.deepcopy(reference)
 e,h=next((e,h) for e in events for f in (e.get('session')or{}).get('Frames',[]) for h in f.get('HTTP3 Frames',[]) if h['Frame Type']=='DATA' and h['Payload Length']>0)
 src=h['Byte Source'];r=src['Ranges'][0]
 if kind=='source-offset':src['Offset']+=1
 elif kind=='source-length':src['Length']+=1
 elif kind=='source-kind':src['Kind']='captured-ciphertext'
 elif kind=='source-direction':src['Direction']=1-src['Direction']
 elif kind=='range-offset':r['Offset']+=1
 elif kind=='range-length':r['Length']+=1
 elif kind=='range-parent':r['Parent PDU']=0
 elif kind=='range-packet':r['Packet References'][0]['Packet']=462
 elif kind=='range-domain':r['Packet References'][0]['Interface']+=1
 elif kind=='event-domain':e['domain']['Section']+=1
 elif kind=='event-endpoint':e['source']='192.0.2.9:9'
 elif kind=='authentication':next(x for x in events if x['id']==r['Parent PDU'])['session']['Authentication Verified']=False
 elif kind=='coverage-flag':src['Byte Coverage Complete']=False
 elif kind=='parent-set':src['Parent PDUs'].append(0)
 elif kind=='packet-set':src['Packet References']=[]
 elif kind=='frame-type':h['Frame Type Value']=1
 elif kind=='frame-length':h['Payload Length']+=1
 elif kind=='frame-header':h['Frame Header Bytes']=base64.b64encode(b'\x00\x00').decode()
 elif kind=='header-selection':h['Frame Header Ranges']['Payload Length'][0]+=1
 elif kind=='frame-hash':h['Frame SHA256']='0'*64
 elif kind=='wire-length':h['Frame Wire Length']+=1
 elif kind in ['duplicate-frame','missing-frame']:
  parent=next(f for f in e['session']['Frames'] if h in f.get('HTTP3 Frames',[]))['HTTP3 Frames']
  if kind=='duplicate-frame':parent.append(copy.deepcopy(h))
  else:parent.remove(h)
 elif kind.startswith('typed-'):
  name='http3.frame_type' if kind=='typed-type' else 'http3.frame_length'
  next(x for x in e['field_values']['values'] if x['name']==name)['value']+=1
 elif kind=='delivered-offset':
  fr=next(f for f in e['session']['Frames'] if f.get('Delivered Bytes',0)>0);fr['Delivered Byte Source']['Offset']+=1
 elif kind=='raw-stream-offset':next(f for f in e['session']['Frames'] if f.get('Frame Type')=='STREAM')['Byte Source']['Offset']+=1
 elif kind=='oracle-physical-bytes':oracle[0]['_source']['layers']['frame_raw'][0]='00'+oracle[0]['_source']['layers']['frame_raw'][0][2:]
 elif kind=='coalesced-DCID-ambiguity':
  qs=next(p['_source']['layers']['quic'] for p in oracle if isinstance(p['_source']['layers']['quic'],list) and any('quic.connection.number' not in q and 'quic.dcid' in q for q in p['_source']['layers']['quic']))
  next(q for q in qs if 'quic.connection.number' not in q and 'quic.dcid' in q)['quic.dcid']='00'
 ep=a.out/(kind+'.jsonl');op=a.out/(kind+'-oracle.json')
 ep.write_text(''.join(json.dumps(x)+'\n' for x in events),encoding='utf8');op.write_text(json.dumps(oracle),encoding='utf8')
 result=run(ep,op,a.out/(kind+'-unexpected.json'))
 assert result.returncode!=0 and b'AssertionError' in result.stderr,(kind,result.returncode,result.stderr.decode('utf8','replace'))
 results.append({'corruption':kind,'detected':True})
(a.out/'summary.json').write_text(json.dumps({'positive_checks':json.loads((a.out/'positive.json').read_text())['checks'],'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8')
print('All',len(results),'corruptions detected')
