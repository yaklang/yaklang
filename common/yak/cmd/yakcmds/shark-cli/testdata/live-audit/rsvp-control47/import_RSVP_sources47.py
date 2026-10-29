"""Archive exact RSVP audit source and hashes; exclude captures, plans, expected rows and runtime proof data."""
from pathlib import Path
import argparse,ast,hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
ap=argparse.ArgumentParser(description=__doc__)
ap.add_argument('--extra',action='append',default=[])
a=ap.parse_args()
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/rsvp-control47'
assert not dest.exists()
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
source='55b29ade06b2201e8be8e40b25920226038df2e1'
assert read(root/'RSVP-native-CLI-55b-actual47-01/summary.json')['source']==source
assert read(root/'Windows-RSVP-root-closure47-01/summary.json')['passed']
assert read(root/'RSVP-fuzz-55b-47-01/summary.json')['source']==source
names=[
 'census_RSVP_source45.py','census_RSVP_source45_v02.py',
 'download_RSVP_primary45.py','inspect_RSVP_official45.py','build_RSVP_baseline_source45.py',
 'rsvp_boundary_source45.py','build_RSVP_boundaries45.py','audit_RSVP_partial_owners46.py',
 'rsvp_direct_api_checker46.py','rsvp_direct_api_checker46_v02.py','bridge_RSVP_source_plans46.py',
 'build_RSVP_guarded46.py','build_OSPFv2_guarded41.py','run_RSVP_direct46.py','run_RSVP_direct46_v02.py',
 'rsvp_native_cli_checker46.py','run_RSVP_native_CLI46.py','run_RSVP_official_before46.py',
 'audit_RSVP_parent_sources46.py','audit_RSVP_parent_sources46_v02.py','audit_RSVP_parent_sources46_v03.py',
 'rsvp_parent_diagnostic_source47.py','rsvp_parent_diagnostic_source47_v02.py',
 'prepare_Windows_RSVP46.py','prepare_Windows_RSVP_navigation46.py','prepare_Windows_RSVP_navigation46_v02.py',
 'run_Windows_RSVP46.py','run_Windows_OSPFv2_42.py','verify_Windows_OSPFv3_36.py','close_Windows_RSVP47.py',
 'run_fuzz_RSVP46.py','run_fuzz_membership.py','run_go_temp.py','run_build_temp.py','replay_public_resume.py',
 'download_RSVP_Intserv_primary46.py','rsvp_intserv_source46.py','build_RSVP_Intserv_source46.py',
 'import_RSVP_sources47.py',
]
for name in a.extra:
 assert Path(name).name==name and Path(name).suffix=='.py'and name not in names
 names.append(name)
sources=[(root/n,n)for n in names]
sources.append((root/'rsvp_direct_probe46.go','rsvp_direct_probe46.go.txt'))
sources.extend((Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')/n,'runner-dependencies/'+n)for n in('run_go_phase.py','run_build.py'))
sources.extend((root/'OSPFv3-independent-source37-a82-01'/n,'source-reader/'+n)for n in('scan_source37.py','ospf_source_codec.py'))
dependencies=[
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_native_literal_continuity.py',
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/public_cli_literal_continuity.py',
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/known_pdu_reference_paths_v2.py',
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/known_cli_pdu_reference_paths.py',
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/vxlan_endpoint_projection_audit.py',
 'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/discovery/console/main_windows.go',
]
records=[]
for src,relative in sources:
 assert src.is_file()and src.suffix in('.py','.go')
 data=src.read_bytes();assert len(data)<200000
 if src.suffix=='.py':ast.parse(data.decode('utf8'),filename=str(src))
 records.append(dict(path=relative,original_path=str(src),bytes=len(data),SHA256=sha(src),language='Go'if src.suffix=='.go'else'Python'))
assert len({r['path']for r in records})==len(records)
dest.mkdir()
for (src,relative),record in zip(sources,records):
 target=dest/relative;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(src.read_bytes())
 assert sha(src)==sha(target)==record['SHA256']
(dest/'.gitattributes').write_text('*.py -text whitespace=cr-at-eol\n*.go.txt -text whitespace=cr-at-eol\n*.json -text whitespace=cr-at-eol\n*.md text eol=lf\n',encoding='utf8')
manifest=dict(scope=__doc__,production_sources=['4dceaf339e49dd235918664ec38c827a0f20cf30',source],source_files=len(records),source_bytes=sum(r['bytes']for r in records),files=records,existing_repository_dependencies=[dict(path=p,bytes=(repo/p).stat().st_size,SHA256=sha(repo/p))for p in dependencies],external_frozen_manifest_hashes={n:sha(root/n)for n in['RSVP-direct-API-source-plan46-02/manifest.json','RSVP-native-CLI-source-plan46-01/manifest.json','RSVP-parent-diagnostic-source47-02/manifest.json']},Intserv_is_source_audit_only_not_product_implementation=True,excluded=['PCAP/PCAPNG/keylogs','full source facts/plans/expected rows/matrices','native/CLI exports/PDML/reference outputs','proof JSONs/logs','console cells/images/binaries','RFC/PDF primary text'],Go_probe_stored_as_text_preserves_original_bytes=True)
(dest/'source-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
(dest/'README.md').write_text('''# RSVP source audit controllers

This directory contains source only. The source manifest binds exact executed
bytes, existing repository dependencies and external frozen source contracts.
No capture, keylog, full per-case expectation, runtime export or screenshot is
included. The Go probe is stored as `.go.txt` for byte-for-byte provenance.

Controllers expect their original external proof directory and downloaded
inputs. Reconstruct those paths outside the checkout before executing them;
the manifest is a dependency record, not a bundled runnable capture corpus.
The existing OSPFv3 capture reader and guarded build/test runner dependencies
are retained as source. Windows images are reconstructed from actual console
cells and attributes, not Windows Terminal pixels. Navigation revisions keep
the independently frozen field names, values and ranges unchanged.

The direct codec, native/CLI and parent-carrier audits have different scopes.
An IP46 declaration does not authenticate an RSVP sender or prove an installed
reservation. Intserv controllers here are an independent source audit for a
later parser stage; their presence does not imply product support. Complete
primary specifications and all proof data remain external.
''',encoding='utf8')
print(json.dumps(dict(directory=str(dest),source_files=len(records),source_bytes=manifest['source_bytes'],manifest_SHA256=sha(dest/'source-manifest.json'))))
