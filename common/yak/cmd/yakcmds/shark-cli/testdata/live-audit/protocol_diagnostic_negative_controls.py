"""Ensure diagnostic-only export auditing rejects unrelated or invented changes."""
import argparse,copy,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in['before','after','events','out']:ap.add_argument('--'+n,type=Path,required=True)
a=ap.parse_args()
for p in vars(a).values():assert not any((q/'.git').exists()for q in[p.resolve(),*p.resolve().parents])
a.out.mkdir(parents=True,exist_ok=False);original=[json.loads(l)for l in a.after.open(encoding='utf-8-sig')];old={v['id']:v for e in[json.loads(l)for l in a.before.open(encoding='utf-8-sig')]for v in e.get('pdus',[])};audit=Path(__file__).with_name('protocol_diagnostic_export_invariants.py')
def fresh(es):return next((e,v)for e in es for v in e.get('pdus',[])if v.get('error')and'error'not in old[v['id']])
controls=[('invented-error',lambda es:fresh(es)[1].__setitem__('error','invented reason')),('invented-expert',lambda es:fresh(es)[1].__setitem__('expert','Decoded')),('invented-summary',lambda es:fresh(es)[1].__setitem__('summary','invented observation')),('missing-diagnostic',lambda es:fresh(es)[1].pop('error')),('changed-PDU-status',lambda es:fresh(es)[1].__setitem__('status','decoded')),('changed-PDU-ID',lambda es:fresh(es)[1].__setitem__('id',999999)),('changed-row-flow',lambda es:fresh(es)[0].__setitem__('conversation_id',999999)),('changed-row-protocol',lambda es:fresh(es)[0].__setitem__('protocol','TCP')),('changed-row-summary',lambda es:fresh(es)[0].__setitem__('info','rewritten summary')),('invented-raw-bytes',lambda es:fresh(es)[1].__setitem__('raw','AAAA')),('changed-typed-value',lambda es:fresh(es)[1]['field_values']['values'].append({'name':'invented.field','value':1,'origin':'derived'})),('extra-physical-row',lambda es:es.append(copy.deepcopy(es[-1])))]
results=[]
for name,mutate in [('baseline',None),*controls]:
 d=a.out/name;d.mkdir();es=copy.deepcopy(original)
 if mutate:mutate(es)
 ep=d/'CLI.jsonl';ep.write_text(''.join(json.dumps(v)+'\n'for v in es),encoding='utf8');r=subprocess.run([sys.executable,'-X','utf8',str(audit),'--before',str(a.before),'--after',str(ep),'--events',str(a.events),'--out',str(d/'audit.json')],capture_output=True,timeout=60);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert(r.returncode==0)==(name=='baseline'),(name,(r.stdout+r.stderr).decode('utf8','replace')[-700:]);results.append({'control':name,'accepted':r.returncode==0})
out={'scope':__doc__,'baseline_passed':True,'rejected_corruptions':len(controls),'controls':results};(a.out/'summary.json').write_text(json.dumps(out,indent=2)+'\n',encoding='utf8');print(json.dumps(out))
