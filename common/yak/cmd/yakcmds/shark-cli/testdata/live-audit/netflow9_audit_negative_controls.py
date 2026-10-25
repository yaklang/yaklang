from pathlib import Path
import json,copy,subprocess,sys
import argparse
ap=argparse.ArgumentParser(description='Corrupt one retained field/provenance witness and require the independent audit to reject it.')
ap.add_argument('--events',required=True,type=Path)
ap.add_argument('--oracle',required=True,type=Path)
ap.add_argument('--out',required=True,type=Path)
ap.add_argument('--audit',type=Path,default=Path(__file__).with_name('netflow9_wire_audit.py'))
a=ap.parse_args();root=a.out;root.mkdir(parents=True,exist_ok=True)
events=[e for l in a.events.open(encoding='utf-8-sig') if (e:=json.loads(l)).get('protocol')=='netflow-v9']
assert events
def first(rows):
 for e in rows:
  for s in e['session']['FlowSets']:
   if s.get('Records'):return e,s,s['Records'][0]['Fields'][0]
cases=['primitive','value-range','definition-hash','definition-range','source-reference','typed-alias']
summary=[]
for case in cases:
 rows=copy.deepcopy(events);e,s,f=first(rows)
 if case=='primitive': f['Value']+=1
 elif case=='value-range':f['Wire Field Ranges']['Value'][0]+=1
 elif case=='definition-hash':s['Template Definition SHA256']='0'*64
 elif case=='definition-range':s['Template Definition Offset']+=1
 elif case=='source-reference':s['Template Packet References'][0]['Packet']+=1
 elif case=='typed-alias':next(v for v in e['field_values']['values'] if v['name']=='cflow.version')['value']=8
 target=root/('negative-'+case+'.jsonl');target.write_bytes(('\n'.join(json.dumps(x) for x in rows)+'\n').encode())
 out=root/('negative-'+case+'.json');run=subprocess.run([sys.executable,'-X','utf8',str(a.audit),str(a.oracle),str(target),str(out)],capture_output=True)
 assert run.returncode==1,(case,run.returncode,run.stderr.decode())
 result=json.loads(out.read_bytes());assert result['selected_differences']
 summary.append({'case':case,'exit':run.returncode,'detected_checks':sorted(set(d['check'] for d in result['selected_differences'])),'differences':len(result['selected_differences'])})
 print(summary[-1],flush=True)
(root/'negative-control-summary.json').write_bytes((json.dumps(summary,indent=2)+'\n').encode())