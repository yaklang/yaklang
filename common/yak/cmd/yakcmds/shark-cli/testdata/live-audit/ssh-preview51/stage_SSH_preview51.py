"""Stage only the UI sources matching the closed895-pass CLI source snapshot."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
receipt=json.loads((root/'test-SSH-preview-green51-03/receipt.json').read_text('utf8'))
assert receipt['exit_code']==0 and receipt['test_actions']=={'pass':895,'skip':2}and receipt['all_runs_have_unique_terminal']
expected=['common/yak/cmd/yakcmds/shark-cli/protocol_events.go','common/yak/cmd/yakcmds/shark-cli/ssh_stream_source_test.go']
hashes={p.replace('\\','/'):s for p,s in receipt['source']['Go_working_files'].items()}
assert set(hashes)==set(expected)
parent=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip();assert parent==receipt['source']['parent_HEAD']
for p,h in hashes.items():assert hashlib.sha256((repo/p).read_bytes()).hexdigest()==h
subprocess.run(['git','add','--',*expected],cwd=repo,check=True)
assert sorted(subprocess.check_output(['git','diff','--cached','--name-only'],cwd=repo,text=True).splitlines())==sorted(expected)
subprocess.run(['git','diff','--cached','--check'],cwd=repo,check=True)
for p,h in hashes.items():assert hashlib.sha256(subprocess.check_output(['git','show',':'+p],cwd=repo)).hexdigest()==h,p
proof=dict(parent_HEAD=parent,staged_files=hashes,all_staged_bytes_equal_executed_source=True,
 tests=dict(pass_count=895,skip_count=2,receipt_SHA256=hashlib.sha256((root/'test-SSH-preview-green51-03/receipt.json').read_bytes()).hexdigest(),log_SHA256=receipt['log_SHA256']),
 scope='Byte-source captions only; physical capture/SDK/private protocol identities unchanged.')
with(root/'SSH-preview-stage51-01.json').open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
print(json.dumps(proof))
