"""Require scope-audit failure for corrupted QUIC authentication/source evidence."""
import argparse,copy,json,re,subprocess,sys
from pathlib import Path
ap=argparse.ArgumentParser(description=__doc__)
for n in ['capture','before','after','out']:ap.add_argument('--'+n,required=True,type=Path)
a=ap.parse_args();a.out.mkdir(parents=True,exist_ok=True)
assert not any((p/'.git').exists()for p in[a.out.resolve(),*a.out.resolve().parents])
base=[json.loads(s)for s in a.after.read_text(encoding='utf-8-sig').splitlines()];audit=Path(__file__).with_name('quic_admission_scope_audit.py')
def run(ep,out):return subprocess.run([sys.executable,'-X','utf8',str(audit),'--capture',str(a.capture),'--before',str(a.before),'--after',str(ep),'--out',str(out)],capture_output=True)
r=run(a.after,a.out/'positive.json');assert r.returncode==0,r.stderr.decode('utf8','replace')
results=[]
for kind in['false-authentication','missing-scope','offset','length','raw-byte','flow','domain','missing-event','unrelated-summary']:
 es=copy.deepcopy(base);e=next(e for e in es if e['admission']=='unparsed-datagram-remainder')
 if kind=='false-authentication':e['admission']='authenticated-wire-and-observed-CID'
 elif kind=='missing-scope':e['summary']='quic'
 elif kind=='offset':e['summary']=re.sub(r'offset=(\d+)',lambda m:'offset='+str(int(m[1])+1),e['summary'])
 elif kind=='length':
  e['summary']=re.sub(r'(\d+) bytes',lambda m:str(int(m[1])+1)+' bytes',e['summary'])
 elif kind=='raw-byte':e['raw']='AA=='
 elif kind=='flow':e['flow']+=1000
 elif kind=='domain':e['domain']['Section']+=1
 elif kind=='missing-event':es.remove(e)
 else:next(x for x in es if x['admission']=='authenticated-wire-and-observed-CID')['summary']='invented'
 ep=a.out/(kind+'.jsonl');ep.write_text(''.join(json.dumps(x)+'\n'for x in es),encoding='utf8');r=run(ep,a.out/(kind+'-unexpected.json'));assert r.returncode!=0 and b'AssertionError'in r.stderr,(kind,r.stderr.decode('utf8','replace'));results.append({'corruption':kind,'detected':'rejected'})
(a.out/'summary.json').write_text(json.dumps({'controls':len(results),'results':results},indent=2)+'\n',encoding='utf8');print('All',len(results),'scope corruptions rejected')
