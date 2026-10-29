"""Stage only SSH method-identity changes bound to the closed396-test source."""
from pathlib import Path
import hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
def git(*args):return subprocess.check_output(['git',*args],cwd=repo)
def sha(w):return hashlib.sha256(w).hexdigest()
proof=json.loads((root/'test-SSH-exact-name-green49-03/receipt.json').read_text('utf8'))
assert proof['exit_code']==0 and proof['test_actions']=={'pass':396}
expected={Path(path).as_posix():digest for path,digest in proof['source']['Go_working_files'].items()}
assert len(expected)==2 and set(expected)=={'common/pcapx/pcaputil/bin_parser_ssh.go','common/pcapx/pcaputil/protocol_ssh_kex_identity_test.go'}
assert not git('diff','--cached','--name-only').strip()
changed=set(git('diff','--name-only').decode().splitlines())|set(git('ls-files','--others','--exclude-standard').decode().splitlines())
assert changed==set(expected),changed
for path,digest in expected.items():
 assert sha((repo/path).read_bytes())==digest
 subprocess.run(['git','add','--',path],cwd=repo,check=True)
 assert sha(git('show',':'+path))==digest
subprocess.run(['git','diff','--cached','--check'],cwd=repo,check=True)
print(json.dumps(dict(staged_files=2,index_blobs_equal_closed_test_source=True,passed_tests=396)))
