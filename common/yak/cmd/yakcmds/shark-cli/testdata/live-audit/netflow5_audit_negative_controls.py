"""Require independently detected failures after corrupting retained v5 evidence."""
from pathlib import Path
import argparse,json,copy,subprocess,sys
ap=argparse.ArgumentParser(description=__doc__);ap.add_argument('--events',required=True,type=Path);ap.add_argument('--oracle',required=True,type=Path);ap.add_argument('--out',required=True,type=Path);ap.add_argument('--audit',type=Path,default=Path(__file__).with_name('netflow5_wire_audit.py'));a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
rows=[e for l in a.events.open(encoding='utf-8-sig') if (e:=json.loads(l)).get('protocol')=='netflow-v5'];assert rows
summary=[]
for case in ['address','value-range','source-hash','typed-alias','nanoseconds','record-order','omit-field','omit-alias']:
 mutated=copy.deepcopy(rows);e=mutated[0];record=e['session']['Records'][0];field=record['Fields'][0]
 if case=='address':field['Value']='192.0.2.99'
 elif case=='value-range':field['Wire Field Ranges']['Value'][0]+=1
 elif case=='source-hash':e['source_bytes']['SHA256']='0'*64
 elif case=='typed-alias':next(v for v in e['field_values']['values'] if v['name']=='cflow.nexthop')['value']='192.0.2.99'
 elif case=='nanoseconds':e['session']['Unix Nanoseconds']+=1
 elif case=='record-order':record['Index']+=1
 elif case=='omit-field':record['Fields'].pop()
 elif case=='omit-alias':e['field_values']['values']=[v for v in e['field_values']['values'] if v['name']!='cflow.srcaddr']
 source=a.out/(case+'-events.jsonl');source.write_bytes(('\n'.join(json.dumps(r) for r in mutated)+'\n').encode());output=a.out/(case+'-audit.json')
 r=subprocess.run([sys.executable,'-X','utf8',str(a.audit),str(a.oracle),str(source),str(output)],capture_output=True);assert r.returncode==1,(case,r.returncode,r.stderr.decode());d=json.loads(output.read_bytes());assert d['selected_differences']
 summary.append({'case':case,'audit_exit':r.returncode,'detected_checks':sorted(set(x['check'] for x in d['selected_differences'])),'differences':len(d['selected_differences'])});print(summary[-1],flush=True)
(a.out/'summary.json').write_bytes((json.dumps(summary,indent=2)+'\n').encode())