"""Bind docs/audit-only e41 checkpoint to the existing production source baseline."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).resolve().parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
before='5c88aed7aa9ee8c7438aca30a6bfaa114fff85d9';after='e41da9c4401f9f97108daa2b99d3f463aa52a32d'
out=root/'source-equivalence01';out.mkdir(exist_ok=False)
commands=[]
def run(args):
 p=subprocess.run(['git',*args],cwd=repo,stdout=subprocess.PIPE,stderr=subprocess.PIPE)
 commands.append(dict(arguments=['git',*args],exit_code=p.returncode,stdout=p.stdout.decode('utf8'),stderr=p.stderr.decode('utf8')))
 assert p.returncode==0;return p.stdout.decode('utf8')
changed=run(['diff','--name-only','-z',before,after]).split('\0');assert changed[-1]=='';changed.pop()
prefixes=['common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/','docs/']
assert changed and all(any(p.startswith(prefix) for prefix in prefixes) for p in changed)
trees={}
for path in ['common/pcapx/pcaputil','common/bin-parser']:
 a=run(['rev-parse',before+':'+path]).strip();b=run(['rev-parse',after+':'+path]).strip();assert a==b
 trees[path]=dict(before_tree=a,source_tree=b,equal=True)
sha=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
logs=out/'git-evidence.json';logs.write_text(json.dumps(commands,indent=2)+'\n',encoding='utf8')
value=dict(scope=__doc__,baseline_source=before,source=after,production_trees=trees,
 changed_files=changed,allowed_nonproduction_change_prefixes=prefixes,all_changes_are_documentation_or_standalone_audit_sources=True,
 production_code_equivalent_for_native_stats_API_probe=True,baseline_source_metadata_not_rewritten=True,
 git_evidence_file=str(logs),git_evidence_SHA256=sha(logs),auditor_SHA256=sha(Path(__file__)))
p=out/'equivalence.json';p.write_text(json.dumps(value,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(equivalence=str(p),SHA256=sha(p))))
