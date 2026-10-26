"""Prove the carrier auditor rejects independent corruptions of an offline case."""
from pathlib import Path
import argparse,copy,json,subprocess,sys
ap=argparse.ArgumentParser(description=__doc__)
for n in ['oracle','events','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists() for p in [a.out.resolve(),*a.out.resolve().parents])
reference=json.loads(a.oracle.read_text(encoding='utf-8-sig'));native=[json.loads(s) for s in a.events.read_text(encoding='utf-8-sig').splitlines()]
assert len(native)==1 and native[0]['status']=='malformed' and len(reference)==1
mutations={
 'wrong-wire-status':lambda r,e:e[0].update(status='incomplete'),
 'wrong-typed-cause':lambda r,e:e[0].update(expert='NeedMore'),
 'invented-application':lambda r,e:e[0].update(protocol='dns'),
 'missing-diagnostic':lambda r,e:e[0].update(error=''),
 'invented-fields':lambda r,e:e[0].update(fields={'Caller':True}),
 'complete-field-coverage':lambda r,e:e[0]['field_values'].update(completeness='message'),
 'lost-incomplete-flag':lambda r,e:e[0]['field_values'].update(incomplete=False),
 'wrong-reference-port':lambda r,e:r[0]['_source']['layers']['udp'].update({'udp.srcport':'40001'}),
}
results=[]
for name,mutate in mutations.items():
 r,e=copy.deepcopy(reference),copy.deepcopy(native);mutate(r,e)
 oracle=a.out/(name+'-oracle.json');events=a.out/(name+'-events.jsonl');out=a.out/(name+'-unexpected.json')
 oracle.write_text(json.dumps(r),encoding='utf-8');events.write_text('\n'.join(json.dumps(x) for x in e)+'\n',encoding='utf-8')
 p=subprocess.run([sys.executable,'-X','utf8',str(Path(__file__).with_name('udp_carrier_wire_audit.py')),str(oracle),str(events),str(out)],capture_output=True)
 detected=p.returncode!=0;assert detected,(name,'corruption escaped independent audit')
 results.append({'case':name,'detected':detected})
(a.out/'summary.json').write_text(json.dumps({'controls':results,'all_detected':True},indent=2)+'\n',encoding='utf-8')
print('detected',len(results),'independent audit corruptions')
