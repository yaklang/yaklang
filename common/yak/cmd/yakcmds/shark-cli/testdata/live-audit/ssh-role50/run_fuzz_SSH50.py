"""Two bounded SSH role/private-refusal fuzz targets on committed frozen source."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
assert hashlib.sha256(archive.read_bytes()).hexdigest()=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
base=root/'run_fuzz_membership.py'
assert hashlib.sha256(base.read_bytes()).hexdigest()==next(x['SHA256']for x in json.loads(archive.read_text('utf8'))['files']if x['path']==base.name)
source=base.read_text('utf8')
old="for pkg,name in [('./common/bin-parser/parser/stream_parser','FuzzMulticastMembershipWireBounds')]:"
assert source.count(old)==1
source=source.replace(old,"for pkg,name in [('./common/pcapx/pcaputil','FuzzSSHClientRoleNegotiationIsolation'),('./common/pcapx/pcaputil','FuzzSSHNativeRefusalFieldIsolation')]:")
assert source.count("'40s'")==1 and source.count("GOMEMLIMIT='2GiB'")==1
source=source.replace("'40s'","'60s'").replace("GOMEMLIMIT='2GiB'","GOMEMLIMIT='1GiB'")
exec(compile(source,str(Path(__file__)),'exec'),globals())
