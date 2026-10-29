"""Archive exact audit/controller source only, never captures or proof data."""
from pathlib import Path
import ast,hashlib,json
root=Path(__file__).parent;repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/gtpv2-control45';assert not dest.exists()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
assert read(root/'GTPv2-public-native-literal45-01/summary.json')['differences']==0
assert read(root/'GTPv2-public-CLI-literal45-01/summary.json')['differences']==0
assert read(root/'Windows-GTPv2-root-closure45-02/summary.json')['passed']
assert read(root/'GTPv2-fuzz-df30-45-01/summary.json')['source']=='df30fc4bd909d6489844f8e0d1cd0ab03826f35c'
assert read(root/'GTPv2-native-CLI-df30-actual45-01/summary.json')['all_frozen_inputs_unchanged']
names=['census_GTP_family_source43.py','audit_GTP_parent_refusals43.py','audit_GTP_parent_refusals43_diagnostic01.py','audit_GTP_parent_refusals43_v02.py','finish_GTP_source43.py','finish_GTP_source43_v02.py','build_GTPv2_boundaries43_v03.py','audit_GTPv2_authored_reference43.py','extract_GTPv2_IE_names43.py','write_GTPv2_name_tables44.py','gtpv2_api_source_checker43_v02.py','gtpv2_native_cli_controller44_v02.py','selftest_GTPv2_controller44.py','build_GTPv2_guarded44.py','build_OSPFv2_guarded41.py','run_GTPv2_direct44.py','run_GTPv2_direct44_v02.py','run_GTPv2_direct45.py','prepare_Windows_GTPv2_44.py','prepare_Windows_GTPv2_helper45.py','build_console_guarded45.py','run_Windows_GTPv2_44.py','run_Windows_OSPFv2_42.py','verify_Windows_OSPFv3_36.py','close_Windows_GTPv2_45.py','close_Windows_GTPv2_45_v02.py','run_fuzz_GTPv2_44.py','run_fuzz_membership.py','run_go_temp.py','run_build_temp.py','replay_public_resume.py','import_GTPv2_source45.py']
names.append('run_GTPv2_official_before45.py')
names.append('write_GTPv2_report45.py')
sources=[(root/n,n)for n in names]
sources +=[(root/'gtpv2_direct_probe44.go','gtpv2_direct_probe44.go.txt')]
sources +=[(root/'OSPFv3-independent-source37-a82-01'/n,'source-reader/'+n)for n in('scan_source37.py','ospf_source_codec.py')]
sources +=[(Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')/n,'runner-dependencies/'+n)for n in('run_go_phase.py','run_build.py')]
dependencies=['common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_native_literal_continuity.py','common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_cli_literal_continuity.py','common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/vxlan_endpoint_projection_audit.py','common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/pptp_parent_source_audit.py','common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/console/main_windows.go']
records=[];dest.mkdir()
for src,relative in sources:
 data=src.read_bytes();assert src.suffix in('.py','.go')and len(data)<200000
 if src.suffix=='.py':ast.parse(data.decode('utf8'),filename=str(src))
 target=dest/relative;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data);assert sha(src)==sha(target)
 records.append(dict(path=relative,original_path=str(src),bytes=len(data),SHA256=sha(src),language='Go'if src.suffix=='.go'else'Python'))
(dest/'.gitattributes').write_text('*.py -text\n*.go.txt -text\n*.json -text\n*.md text eol=lf\n',encoding='utf8')
manifest=dict(scope=__doc__,production_sources=['f1068e2fbaa44d59f7ab82ee6109505bac5b3af1','df30fc4bd909d6489844f8e0d1cd0ab03826f35c'],files=records,source_files=len(records),source_bytes=sum(x['bytes']for x in records),existing_repository_dependencies=[dict(path=p,bytes=(repo/p).stat().st_size,SHA256=sha(repo/p))for p in dependencies],excluded=['captures/PCAP/PCAPNG/keylogs','complete per-case source facts/plans/expected objects/matrices','native and CLI exports/PDML/reference fields','proof JSONs/logs','console cells/images','binaries/primary PDF or RFC contents'],historical_failed_controller_sources_retained=True,Go_probe_text_preserves_compiled_bytes=True)
(dest/'source-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(directory=str(dest),files=len(records),source_bytes=manifest['source_bytes'],manifest_SHA256=sha(dest/'source-manifest.json'))))
