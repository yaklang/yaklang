"""Independent literal identity scope guard controls; no generated protocol facts."""
import argparse,copy,hashlib,json
from pathlib import Path
from bridge import validate_literal_identity_files,rows,valid_id

def sha(p):return hashlib.sha256(p.read_bytes()).hexdigest()
def main():
 p=argparse.ArgumentParser()
 for n in ('plan','before','after','out'):p.add_argument('--'+n,type=Path,required=True)
 a=p.parse_args();a.out.mkdir(exist_ok=False);positive=[];negative=[]
 for c in json.loads((a.plan/'cases.json').read_text()):
  if c['case']in('weber','sharon'):continue
  old=(a.before/c['relative_producer']).with_name('events.jsonl');new=(a.after/c['relative_producer']).with_name('events.jsonl')
  seen_old=set();seen_new=set();old_ids=[valid_id(e,seen_old)for e in rows(old)];new_ids=[valid_id(e,seen_new)for e in rows(new)]
  assert len(old_ids)==len(new_ids)==c['old_rows'];mapping=dict(zip(old_ids,new_ids));validate_literal_identity_files(old,new,mapping)
  positive.append({'case':c['case'],'rows':len(old_ids),'whole_export_SHA256':sha(old)})
 before=a.out/'authored-old.jsonl';after=a.out/'authored-new.jsonl';data=b'{"protocol":"http3","id":1,"transaction":13}\n';before.write_bytes(data);after.write_bytes(data)
 validate_literal_identity_files(before,after,{1:1});positive.append({'case':'unknown-transaction-protocol-noop','authored_control':True})
 def rejects(name,mapping,content=data):
  after.write_bytes(content)
  try:validate_literal_identity_files(before,after,mapping)
  except AssertionError:negative.append(name);return
  raise AssertionError('negative control accepted '+name)
 rejects('nonidentity-ID',{1:2});rejects('bool-key',{True:1});rejects('bool-value',{1:True});rejects('float-key',{1.0:1});rejects('float-value',{1:1.0})
 rejects('unchanged-ID-mutated-protocol',{1:1},data.replace(b'http3',b'http2'));rejects('unchanged-ID-mutated-wire-integer',{1:1},data.replace(b'13',b'14'));rejects('same-semantics-different-bytes',{1:1},data.replace(b':',b': '))
 result={'scope':__doc__,'positive_controls':positive,'negative_controls':negative,'bridge_SHA256':sha(Path(__file__).with_name('bridge.py')),'auditor_SHA256':sha(Path(__file__))};f=a.out/'summary.json';f.write_text(json.dumps(result,indent=2)+'\n');print(json.dumps({'summary':str(f),'SHA256':sha(f),'positive':len(positive),'negative':len(negative)}))
if __name__=='__main__':main()
