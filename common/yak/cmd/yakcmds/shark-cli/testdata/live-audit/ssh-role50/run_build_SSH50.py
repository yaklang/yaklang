"""Source-frozen build with one CPU/one-GiB beside the bounded full tests."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
base=root/'run_build_temp.py'
expected=next(x['SHA256']for x in json.loads(archive.read_text('utf8'))['files']if x['path']==base.name)
assert hashlib.sha256(base.read_bytes()).hexdigest()==expected
source=base.read_text('utf8')
call="exec(compile(source,str(Path(__file__)),'exec'),globals())"
assert source.count(call)==1
source=source.replace(call,"assert source.count(\"GOMAXPROCS='3',GOMEMLIMIT='2GiB'\")==1\nsource=source.replace(\"GOMAXPROCS='3',GOMEMLIMIT='2GiB'\",\"GOMAXPROCS='1',GOMEMLIMIT='1GiB'\")\n"+call)
exec(compile(source,str(Path(__file__)),'exec'),globals())
