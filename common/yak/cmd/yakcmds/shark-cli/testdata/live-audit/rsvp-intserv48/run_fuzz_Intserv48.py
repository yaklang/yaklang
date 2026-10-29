"""Reuse the RSVP fuzz target with a1GiB limit beside the1GiB CLI replay."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
base=root/'run_fuzz_RSVP46.py'
manifest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
assert hashlib.sha256(manifest.read_bytes()).hexdigest()=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
assert hashlib.sha256(base.read_bytes()).hexdigest()==next(f['SHA256']for f in json.loads(manifest.read_text('utf8'))['files']if f['path']==base.name)
source=base.read_text('utf8')
old="exec(compile(source,str(Path(__file__)),'exec'),globals())"
assert source.count(old)==1
source=source.replace(old,"source=source.replace(\"GOMEMLIMIT='2GiB'\",\"GOMEMLIMIT='1GiB'\")\n"+old)
exec(compile(source,str(Path(__file__)),'exec'),globals())
