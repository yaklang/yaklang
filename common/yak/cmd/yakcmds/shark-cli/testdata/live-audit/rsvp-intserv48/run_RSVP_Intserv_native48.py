"""Execute source-frozen38 Intserv physical inputs with exact-source SDK/CLI."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
base=root/'run_RSVP_native_CLI46.py'
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
assert sha(archive)=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
expected=next(f['SHA256']for f in json.loads(archive.read_text('utf8'))['files']if f['path']==base.name)
assert sha(base)==expected
source=base.read_text('utf8')
changes={
 'official9+authored24':'official9+authored29',
 "root/'RSVP-native-CLI-source-plan46-01'":"root/'Intserv-native-CLI-source48-01/plan02'",
 "root/'rsvp_native_cli_checker46.py'":"root/'Intserv-native-CLI-source48-01/intserv_native_cli48_v02.py'",
 '1f291b0e4bf9ad0db09b12d1cd952f3fe69be41b40d9261598d96a63bdb1305c':'fc3a502c4eb4d7e8b54679901510b72afe9fa5af8ed6ad53c666d16b4e2e831b',
 '24124ac22523e73b72fa43191b8bf35505312b6f16cd76b36bf7bb03b6c3ae7e':'6ea649c4425b3f3c0771d020e809c4ed72d5ba8d5cd0110e007e6f3176421fae',
 'm.load(plan)':'m.load_plan(plan)',
 'm.verify_exports(':'m.verify(',
 'source_cases=33':'source_cases=38',
}
for old,new in changes.items():
 assert old in source,old
 source=source.replace(old,new)
exec(compile(source,str(Path(__file__)),'exec'),globals())
