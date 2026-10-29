"""Reuse full-source Windows closure for the two independently selected Intserv fields."""
from pathlib import Path
import hashlib,json
root=Path(__file__).parent
base=root/'close_Windows_RSVP47.py'
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
manifest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47/source-manifest.json'
assert hashlib.sha256(manifest.read_bytes()).hexdigest()=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
expected=next(x['SHA256']for x in json.loads(manifest.read_text('utf8'))['files']if x['path']==base.name)
assert hashlib.sha256(base.read_bytes()).hexdigest()==expected
source=base.read_text('utf8')
changes={
 "for key in('actual','out')":"for key in('actual','out','plan')",
 "old=root/'Windows-RSVP-source-plan46-01';plan=root/'Windows-RSVP-source-plan46-03'":"old=root/'Windows-Intserv-source-plan48-01';plan=a.plan",
 "and read(plan/'plan.json')['only_navigation_changed']":"",
 'only_navigation_changed=True':'only_navigation_changed=plan!=old',
 'old_navigation_failures_preserved=True':'old_navigation_failures_preserved=plan!=old',
}
for old,new in changes.items():
 assert old in source,old
 source=source.replace(old,new)
exec(compile(source,str(Path(__file__)),'exec'),globals())
