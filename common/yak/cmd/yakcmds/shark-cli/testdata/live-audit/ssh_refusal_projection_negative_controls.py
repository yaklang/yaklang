"""Challenge the one-record SSH refusal projection proof without changing wire/reference witnesses."""
import argparse,copy,hashlib,json,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for name in ['before','after','manifest','oracle','out']:ap.add_argument('--'+name,type=Path,required=True)
a=ap.parse_args()
for path in vars(a).values():assert not any((p/'.git').exists()for p in[path.resolve(),*path.resolve().parents])
a.out.mkdir(exist_ok=False,parents=True);before=[json.loads(l)for l in a.before.open()];after=[json.loads(l)for l in a.after.open()];assert len(before)==len(after);index=next(i for i,(b,n)in enumerate(zip(before,after))if b!=n);assert after[index]['fields']is None and after[index].get('fields_error')
def run(name,mutate,positive=False):
 d=a.out/name;d.mkdir();rows=copy.deepcopy(after);mutate(rows);path=d/'after.jsonl';path.write_text(''.join(json.dumps(e,sort_keys=True,separators=(', ',': '))+'\n'for e in rows));args=[sys.executable,'-X','utf8',str(Path(__file__).with_name('ssh_refusal_projection_audit.py')),'--before',str(a.before),'--after',str(path),'--manifest',str(a.manifest),'--oracle',str(a.oracle),'--out',str(d/'proof.json')];r=subprocess.run(args,capture_output=True,timeout=30);(d/'audit.log').write_bytes(r.stdout+r.stderr);assert (r.returncode==0)==positive,(name,r.returncode,(r.stdout+r.stderr).decode('utf8','replace')[-600:])
def restore(rows):rows[index]=copy.deepcopy(before[index])
controls={
 'generic-success-restored':restore,
 'empty-map-is-not-refusal':lambda n:n[index].__setitem__('fields',{}),
 'missing-query-error':lambda n:n[index].pop('fields_error'),
 'wrong-query-error':lambda n:n[index].__setitem__('fields_error','accepted'),
 'decoded-status':lambda n:n[index].__setitem__('status','decoded'),
 'cleared-original-error':lambda n:n[index].__setitem__('error',''),
 'invented-completion':lambda n:n[index]['source_bytes'].__setitem__('CompletedIn',copy.deepcopy(n[index]['source_bytes']['PacketRefs'][0])),
 'wrong-contributor':lambda n:n[index]['source_bytes']['PacketRefs'][0].__setitem__('Number',99999),
 'wrong-domain':lambda n:n[index]['domain'].__setitem__('Section',1),
 'wrong-id':lambda n:n[index].__setitem__('id',n[index]['id']+1),
 'wrong-flow':lambda n:n[index].__setitem__('flow',n[index]['flow']+1),
 'wrong-value-type':lambda n:n[index].__setitem__('flow',str(n[index]['flow'])),
 'drop-row':lambda n:n.pop(),
 'duplicate-row':lambda n:n.append(copy.deepcopy(n[-1])),
 'row-order':lambda n:n.reverse(),
}
run('positive-format-and-key-order',lambda n:None,True)
for name,mutate in controls.items():run(name,mutate)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest();result={'scope':__doc__,'positive_format_controls':1,'records':len(after),'negative_controls_rejected':list(controls),'before_sha256':sha(a.before),'after_sha256':sha(a.after),'manifest_sha256':sha(a.manifest),'oracle_sha256':sha(a.oracle),'wire_and_reference_witnesses_unchanged':True};(a.out/'summary.json').write_text(json.dumps(result,indent=2)+'\n');print(json.dumps(result))
