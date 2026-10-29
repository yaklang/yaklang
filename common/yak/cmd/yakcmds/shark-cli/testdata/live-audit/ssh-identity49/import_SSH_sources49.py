"""Archive exact SSH audit code; captures, expected plans and outputs stay external."""
from pathlib import Path
import ast,hashlib,json

root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ssh-identity49'
assert not dest.exists();dest.mkdir()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
files={n:root/n for n in('run_SSH_before49.py','run_SSH_before_CLI49.py','inspect_SSH_before49.py',
 'run_SSH_native49.py','stage_SSH_kex49.py','run_Windows_SSH49.py','close_Windows_SSH49.py',
 'close_SSH_source_delta49.py','import_SSH_sources49.py')}
for group,folder,names in[
 ('source','SSH-source49-audit01',['build_ssh_source49.py','check_ssh_source49.py','close_ssh_source49.py']),
 ('native','SSH-SDK-CLI-source49-01',['ssh_source_API49.py','ssh_native_cli49.py']),
 ('native','SSH-SDK-CLI-source49-02',['ssh_native_cli49_v02.py']),
 ('native','SSH-SDK-CLI-source49-03',['ssh_native_cli49_v03.py']),
 ('reference','SSH-Wireshark-source49-01',['prepare_ssh_reference49.py','run_ssh_reference49.py',
  'compare_ssh_reference49.py','compare_ssh_reference49_v02.py','close_ssh_reference49.py','close_ssh_reference49_v02.py'])]:
 for name in names:files[group+'/'+name]=root/folder/name
files['windows/renderer.py']=root/'windows-identity-1MiB-5c88a-02/frozen-inputs/renderer.py'
records=[]
for name,path in files.items():
 raw=path.read_bytes();ast.parse(raw,filename=str(path));target=dest/name
 target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw)
 assert sha(target)==sha(path)
 records.append(dict(path=name,original_path=str(path),bytes=len(raw),SHA256=sha(path),language='Python'))
base=dest.parent
dependencies=[]
for name in('rsvp-control47/run_go_temp.py','rsvp-control47/run_build_temp.py','rsvp-control47/replay_public_resume.py',
 'public_native_literal_continuity.py','public_cli_literal_continuity.py','discovery/console/main_windows.go'):
 p=base/name;dependencies.append(dict(path=p.relative_to(repo).as_posix(),bytes=p.stat().st_size,SHA256=sha(p)))
proof=dict(scope=__doc__,production_source='cc9d84079d455a64b57365e143b9e5ee641a7997',source_files=len(records),
 source_bytes=sum(r['bytes']for r in records),files=records,existing_repository_dependencies=dependencies,
 source_plan_hashes={
  'SSH-source49-audit01/plan01/manifest.json':'37e004f4266ed3ce1bf3fd65c0df789ef2d16a11f0691713471ea8c69a2ab4b0',
  'SSH-SDK-CLI-source49-01/plan01/manifest.json':'6152f55c293920a466756dcc090e1b4d0627f8ca7379d33e21cb175c37c2d85d',
  'SSH-SDK-CLI-source49-02/plan02/manifest.json':'ee5e2f1d7daa26c859064f36e83aa5f25c2361e0f8ed8c56f9aae642928f0480',
  'SSH-SDK-CLI-source49-03/contract.json':'5de2a91f843b8c3871df54f4dfce1a50c9ff7a0157e84334f5ae7549670dc0c7'},
 excluded=['captures/keylogs','raw wire blobs','full source plans/expected facts','actual SDK/CLI/PDML/TSV outputs',
  'logs/proof receipts','images/cells/binaries','full RFC or vendor C source'],
 failed_versions_preserved=True,exact_source_bytes_not_normalized=True,
 scope_limits='Authored wire controls and exact selected SSH initial layouts, not new live peers, cryptographic authentication or all SSH states.')
for path,digest in proof['source_plan_hashes'].items():assert sha(root/path)==digest,path
with(dest/'source-manifest.json').open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
(dest/'.gitattributes').write_text('*.py -text\n*.json -text\n',encoding='utf8')
(dest/'README.md').write_text('''# SSH exact method identity audit49 source

Exact historical controller/oracle/renderer Python bytes are retained with SHA256.
They use external frozen plans and Windows tool paths. Source-plan hashes and
existing repository dependencies are recorded in source-manifest.json. Recreate
external plans with matching paths or use an isolated adapter; changing paths
creates a different execution and must receive a new receipt.

Native checker01 omitted source-known FIN diagnostics and existing connection
captions. Checker02 strictly adds11 complete diagnostic DTOs; checker03 checks
the existing connection annotation before applying the unchanged76-payload
oracle. Failed versions remain available. Reference v02 preserves the original
failed PDML empty-string/highlight and role-label closure checks with explicit
errata, without editing source facts or candidate values.

No captures, keylogs, raw blobs, full expected plans, native/CLI/PDML exports,
logs, proof JSON, images, binaries, full RFCs or Wireshark C are included. The
small source-manifest.json is a source inventory, not runtime output. This does
not certify all SSH algorithms, authentication, message sequencing or TOP30.
''',encoding='utf8')
assert len(records)==23
print(json.dumps(dict(source_files=len(records),source_bytes=proof['source_bytes'],dependencies=len(dependencies),
 manifest_SHA256=sha(dest/'source-manifest.json'))))
