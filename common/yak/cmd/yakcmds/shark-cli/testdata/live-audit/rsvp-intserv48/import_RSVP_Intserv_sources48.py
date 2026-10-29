"""Archive exact Intserv audit source only; never plans, captures or outputs."""
from pathlib import Path
import ast,hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
out=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-intserv48'
assert not out.exists();out.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
files={name:root/name for name in(
 'rsvp_intserv_API48.py','run_RSVP_Intserv_direct48.py','prepare_Windows_Intserv48.py',
 'run_RSVP_Intserv_native48.py','run_RSVP_Intserv_native48_v02.py','close_Windows_Intserv48.py',
 'close_RSVP_Intserv_commit48.py','run_fuzz_Intserv48.py','stage_RSVP_Intserv48.py',
 'import_RSVP_Intserv_sources48.py')}
for name in('intserv_neutral_source48.py','build_intserv_source48.py'):
 files['source/'+name]=root/'Intserv-source48-01'/name
for name in('intserv_native_schema48.py','intserv_native_cli48.py','intserv_native_cli48_v02.py'):
 files['native/'+name]=root/'Intserv-native-CLI-source48-01'/name
for name in('prepare_mapping48.py','prepare_mapping48_v02.py','run_reference48.py','compare_reference48.py','compare_reference48_v02.py','close_reference48.py'):
 files['reference/'+name]=root/'Intserv-Wireshark-source48-01'/name
rows=[]
for relative,path in files.items():
 raw=path.read_bytes();ast.parse(raw,filename=str(path))
 dest=out/relative;dest.parent.mkdir(parents=True,exist_ok=True);dest.write_bytes(raw)
 assert sha(dest)==sha(path)
 rows.append(dict(path=relative,original_path=str(path),bytes=len(raw),SHA256=sha(path),language='Python'))
old=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47'
manifest=old/'source-manifest.json'
assert sha(manifest)=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
old_names=['rsvp_intserv_source46.py','build_RSVP_Intserv_source46.py','rsvp_boundary_source45.py','rsvp_native_cli_checker46.py','run_RSVP_native_CLI46.py','run_fuzz_RSVP46.py','run_fuzz_membership.py','run_Windows_RSVP46.py','run_Windows_OSPFv2_42.py','verify_Windows_OSPFv3_36.py','close_Windows_RSVP47.py','run_go_temp.py','run_build_temp.py','replay_public_resume.py','build_RSVP_guarded46.py','build_OSPFv2_guarded41.py','rsvp_direct_probe46.go.txt']
dependencies=[]
for name in old_names:
 f=next(x for x in read(manifest)['files']if x['path']==name);path=old/name
 assert sha(path)==f['SHA256']
 dependencies.append(dict(path=path.relative_to(repo).as_posix(),bytes=f['bytes'],SHA256=f['SHA256']))
for name in('public_native_literal_continuity.py','public_cli_literal_continuity.py','discovery/console/main_windows.go'):
 path=old.parent/name
 dependencies.append(dict(path=path.relative_to(repo).as_posix(),bytes=path.stat().st_size,SHA256=sha(path)))
proof=dict(scope=__doc__,production_source='c04a60630ce2a3d0207aeca507e99dc535f8a4dd',source_files=len(rows),source_bytes=sum(x['bytes']for x in rows),files=rows,existing_repository_dependencies=dependencies,prior_source_manifest_SHA256=sha(manifest),source_plan_hashes={
 'Intserv-source48-01/frozen-plan01/manifest.json':'1b57dd45ed6ea226d62569a1f9c6f06981bc85bef2210d115e30d68fc3e7ad66',
 'RSVP-Intserv-API-plan48-03/manifest.json':'1ee14f677769da754b5dcd0b37bfd9e30dc2e0cdfce50791696b5021d2e73227',
 'Intserv-native-CLI-source48-01/plan02/manifest.json':'fc3a502c4eb4d7e8b54679901510b72afe9fa5af8ed6ad53c666d16b4e2e831b',
 'Intserv-Wireshark-source48-01/plan02/manifest.json':'07f9652a93da40a5cd768251e82cdc8c50e79462ca124f14004cbed2175ae6c7'},
 excluded=['captures/keylogs','full source facts/plans/expected','native/CLI/PDML/TSV outputs','logs/proof JSONs','images/cells/binaries','full RFC text'],
 failed_source_versions_preserved=True,exact_raw_source_bytes_not_normalized=True)
for path,digest in proof['source_plan_hashes'].items():assert sha(root/path)==digest,path
(out/'source-manifest.json').write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
(out/'.gitattributes').write_text('*.py -text whitespace=cr-at-eol\n*.json -text whitespace=cr-at-eol\n*.md text eol=lf\n',encoding='utf8')
(out/'README.md').write_text('''# RSVP Intserv source audit

These are the exact executed source-only models, adapters and checkers for
production `c04a60630ce2a3d0207aeca507e99dc535f8a4dd`. The manifest binds each
original source byte sequence, the existing RSVP helpers and the externally
frozen source plans. Preserve the directory structure when restoring the
external provenance workspace used by the controllers.

Inputs, complete plans/expected facts, authored wire files, PCAPs, keylogs,
runtime exports, PDML, proof data, logs, images and binaries stay external.
Acquire the official input from the linked report and verify its hash; replay
controllers require the corresponding external provenance workspace and tool
receipts. This archive is not a self-contained capture corpus.

The initial native runner stopped before execution while the CLI build receipt
was absent, then its first complete invocation exposed the new checker argument
order mismatch. Version02 corrects that invocation, without changing source
values. Reference mapping01 preparation and comparator01 classification errors
are also preserved. The separate source48 model and its original source46
observations were never rewritten to match candidate or reference outputs.

See the Intserv report for direct49 versus default38 scope, literal/derived
counts, full-record preservation, precise Windows source/highlight proof and
Wireshark display differences. No complete RSVP procedure, Guaranteed-service
implementation, authentication or installed reservation is claimed.
''',encoding='utf8')
assert len(rows)==21
print(json.dumps(dict(archive=str(out),source_files=len(rows),source_bytes=proof['source_bytes'],dependencies=len(dependencies),manifest_SHA256=sha(out/'source-manifest.json'))))
