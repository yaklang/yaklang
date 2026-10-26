"""Corrupt fresh SDK HTTP/3 DATA evidence and require the wire audit to fail."""
from pathlib import Path
import argparse,base64,copy,json,subprocess,sys
ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
events=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()]
index=next(i for i,e in enumerate(events) if any(h.get('Frame Type')=='DATA' and h.get('Payload') for fr in (e.get('session') or {}).get('Frames',[]) for h in fr.get('HTTP3 Frames',[])))
def selected(es):
 e=es[index]
 for fr in e['session']['Frames']:
  for h in fr.get('HTTP3 Frames',[]):
   if h.get('Frame Type')=='DATA' and h.get('Payload'):return e,fr,h
def payload(es):
 e,fr,h=selected(es);b=bytearray(base64.b64decode(h['Payload']));b[0]^=1;h['Payload']=base64.b64encode(b).decode()
def omit(es):
 e,fr,h=selected(es);del h['Payload']
def duplicate(es):es.append(copy.deepcopy(es[index]))
mutations={
 'wrong-payload':payload,
 'wrong-direction':lambda es:selected(es)[0].update(direction=1-selected(es)[0]['direction']),
 'wrong-stream':lambda es:selected(es)[1].update({'Stream ID':selected(es)[1]['Stream ID']+4}),
 'wrong-declared-length':lambda es:selected(es)[2].update({'DATA Length':selected(es)[2]['DATA Length']+1}),
 'unverified-plaintext':lambda es:selected(es)[0]['session'].update({'Authentication Verified':False}),
 'missing-source':lambda es:selected(es)[0]['source_bytes'].update(PacketRefs=[]),
 'omitted-DATA':omit,
 'duplicate-delivery':duplicate,
 'wrong-STREAM-offset':lambda es:selected(es)[1].update({'Offset':selected(es)[1]['Offset']+1}),
 'wrong-STREAM-length':lambda es:selected(es)[1].update({'Length':selected(es)[1]['Length']+1}),
 'wrong-STREAM-FIN':lambda es:selected(es)[1].update({'FIN':not selected(es)[1]['FIN']}),
}
results=[]
for name,mutate in mutations.items():
 es=copy.deepcopy(events);mutate(es);p=a.out/(name+'-events.jsonl');out=a.out/(name+'-unexpected.json')
 p.write_text('\n'.join(json.dumps(e) for e in es)+'\n',encoding='utf-8')
 r=subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('quic_http3_data_audit.py')),str(a.oracle),str(p),str(out)],capture_output=True)
 assert r.returncode!=0,(name,'corruption escaped independent wire audit')
 results.append({'case':name,'detected':True})
(a.out/'summary.json').write_text(json.dumps({'controls':results,'all_detected':True},indent=2)+'\n',encoding='utf-8')
print('detected',len(results),'HTTP/3 audit corruptions')
