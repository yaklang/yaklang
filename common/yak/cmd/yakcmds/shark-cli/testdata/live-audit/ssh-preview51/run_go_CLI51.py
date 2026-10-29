"""Freeze source and run the CLI package only for byte-source presentation changes."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
manifest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
assert hashlib.sha256(manifest.read_bytes()).hexdigest()=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
base=root/'run_go_temp.py'
assert hashlib.sha256(base.read_bytes()).hexdigest()==next(x['SHA256']for x in json.loads(manifest.read_text('utf8'))['files']if x['path']==base.name)
source=base.read_text('utf8');call="exec(compile(source,str(Path(__file__)),'exec'),globals())"
assert source.count(call)==1
before="'./common/pcapx/pcaputil','./common/yak/cmd/yakcmds/shark-cli','./common/bin-parser','./common/bin-parser/rules','./common/bin-parser/parser/stream_parser'"
inject='assert source.count('+repr(before)+')==1\nsource=source.replace('+repr(before)+',"\'./common/yak/cmd/yakcmds/shark-cli\'")\n'
inject+="assert source.count(\"GOMAXPROCS='3',GOMEMLIMIT='2GiB'\")==1\nsource=source.replace(\"GOMAXPROCS='3',GOMEMLIMIT='2GiB'\",\"GOMAXPROCS='2',GOMEMLIMIT='1GiB'\")\n"
source=source.replace(call,inject+call)
exec(compile(source,str(Path(__file__)),'exec'),globals())
