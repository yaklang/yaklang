"""Stage only the12 validated product/test sources; bind them to direct49 build."""
from pathlib import Path
import hashlib,json,subprocess
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
root=Path(__file__).parent
files=['common/bin-parser/parser/stream_parser/'+name for name in('rsvp_value.go','rsvp_value_test.go','rsvp_intserv_value.go','rsvp_intserv_value_test.go')]+['common/bin-parser/'+name for name in('protocol_catalog.go','rsvp_observed_fields_test.go')]+['common/pcapx/pcaputil/'+name for name in('protocol_rsvp.go','protocol_rsvp_fields.go','protocol_rsvp_test.go')]+['common/yak/cmd/yakcmds/shark-cli/'+name for name in('protocol_events.go','rsvp_presentation.go','rsvp_presentation_test.go')]
def git(*args):return subprocess.check_output(['git',*args],cwd=repo)
def sha(w):return hashlib.sha256(w).hexdigest()
snapshot=json.loads((root/'RSVP-Intserv-direct-build48-01/source-before.json').read_text('utf8'))
assert not git('diff','--cached','--name-only').strip()
changed=set(git('diff','--name-only').decode().splitlines())|set(git('ls-files','--others','--exclude-standard').decode().splitlines())
assert changed==set(files),(changed-set(files),set(files)-changed)
for path in files:
 assert sha((repo/path).read_bytes())==snapshot['files'][path],path
 subprocess.run(['git','add','--',path],cwd=repo,check=True)
 assert sha(git('show',':'+path))==snapshot['files'][path],path
subprocess.run(['git','diff','--cached','--check'],cwd=repo,check=True)
assert set(git('diff','--cached','--name-only').decode().splitlines())==set(files)
print(json.dumps(dict(files=len(files),all_index_blobs_equal_executed_source=True,direct_evaluations=49,no_capture_or_rule_archive_changes=True)))
