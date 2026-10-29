"""Bind the closed dirty-tree direct49 execution to the committed product bytes."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
source='c04a60630ce2a3d0207aeca507e99dc535f8a4dd'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def git(*args):return subprocess.check_output(['git',*args],cwd=repo)
assert git('rev-parse','HEAD').decode().strip()==source and not git('status','--porcelain').strip()
build=root/'RSVP-Intserv-direct-build48-01';snapshot=read(build/'source-before.json')
assert git('rev-parse',source+'^').decode().strip()==snapshot['HEAD']
for path,digest in snapshot['files'].items():assert sha(repo/path)==digest,path
changed=git('diff-tree','--no-commit-id','--name-only','-r',source).decode().splitlines()
assert len(changed)==12 and all(p.endswith('.go')for p in changed)
for path in changed:assert hashlib.sha256(git('show',source+':'+path)).hexdigest()==snapshot['files'][path],path
actual=root/'RSVP-Intserv-direct-actual48-01';p=read(actual/'producer.json');check=read(actual/'independent-audit/summary.json')
assert p['source_snapshot_SHA256']==sha(build/'source-before.json')and p['source_unchanged']and p['exit_code']==0
for path,digest in p['inputs'].items():assert sha(path)==digest,path
assert check['export_SHA256']==p['export_SHA256']==sha(actual/'direct.jsonl')and check['passed']==check['evaluations']==49 and not check['failures']
native=root/'RSVP-Intserv-native-actual48-03/summary.json';windows=root/'Windows-Intserv-root-closure48-01/summary.json'
assert read(native)['source']==read(windows)['source']==source and read(native)['passed']and read(windows)['passed']
proof=dict(scope=__doc__,source=source,parent_HEAD=snapshot['HEAD'],all11928_wire_source_files_unchanged=True,exact_source_file_count=len(snapshot['files']),all12_changed_Git_blobs_equal_executed_source=True,changed_source_hashes={p:snapshot['files'][p]for p in changed},direct_evaluations=49,direct_producer_SHA256=sha(actual/'producer.json'),direct_audit_SHA256=sha(actual/'independent-audit/summary.json'),native38_summary_SHA256=sha(native),Windows_summary_SHA256=sha(windows),no_public_corpus_or_latest_full_or_CI_claim_yet=True)
out=root/'RSVP-Intserv-commit-bridge48-01.json';assert not out.exists();out.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(proof=str(out),SHA256=sha(out),source=source,direct49_bound=True,native38_and_Windows_bound=True)))
