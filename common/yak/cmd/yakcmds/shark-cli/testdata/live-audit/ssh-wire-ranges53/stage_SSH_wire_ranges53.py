"""Stage exactly the source snapshot that passed the 548-action SSH run."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
test=root/'test-SSH-wire-ranges53-05/receipt.json'
r=json.loads(test.read_text('utf8'))
assert r['exit_code']==0 and r['test_actions']=={'pass':548} and r['all_runs_have_unique_terminal']
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==r['source']['parent_HEAD']
paths=r['source']['Go_working_files'];assert len(paths)==7 and all(p.endswith('.go')for p in paths)
sha=lambda b:hashlib.sha256(b).hexdigest()
for p,h in paths.items():assert sha((repo/p).read_bytes())==h,p
assert not subprocess.check_output(['git','diff','--cached','--name-only'],cwd=repo)
subprocess.run(['git','add','--',*paths],cwd=repo,check=True,stdout=subprocess.DEVNULL,stderr=subprocess.DEVNULL)
indexed=subprocess.check_output(['git','diff','--cached','--name-only'],cwd=repo,text=True).splitlines()
assert set(indexed)=={p.replace('\\','/')for p in paths}
for p,h in paths.items():assert sha(subprocess.check_output(['git','show',':'+p.replace('\\','/')],cwd=repo))==h,p
with(root/'SSH-wire-ranges-stage53-diff-check.log').open('xb')as log:
 q=subprocess.run(['git','diff','--cached','--check'],cwd=repo,stdout=log,stderr=subprocess.STDOUT)
assert q.returncode==0
proof=dict(parent=r['source']['parent_HEAD'],test_receipt_SHA256=sha(test.read_bytes()),test_log_SHA256=r['log_SHA256'],test_actions=r['test_actions'],staged_files=paths,all_index_bytes_equal_tested_working_source=True,only_Go_files=True)
out=root/'SSH-wire-ranges-stage53-01.json';assert not out.exists();out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(staged_files=len(paths),test_pass=548,proof_SHA256=sha(out.read_bytes()))))
