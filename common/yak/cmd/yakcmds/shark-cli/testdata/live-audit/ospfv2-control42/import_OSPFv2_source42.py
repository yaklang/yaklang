"""Import exact executed source only; never captures, matrices or proof outputs."""
from pathlib import Path
import ast,hashlib,json,shutil,subprocess
root=Path(__file__).parent;repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ospfv2-control42';assert not dest.exists()
assert (root/'runtime-public-native-42/summary.json').exists()and(root/'runtime-public-CLI-42/summary.json').exists()and(root/'OSPFv2-fuzz-cd7af-42-01/summary.json').exists()
assert json.loads((root/'OSPFv2-whole-cd7af-42-02/summary.json').read_text())['unplanned_differences']==0
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
root_names=['ospfv2_source_census39.py','freeze_OSPFv2_direct_plan41.py','verify_OSPFv2_direct_fields41.py','build_OSPFv2_review_regressions40.py','audit_OSPFv2_source_contract41.py','ospfv2_source_contract41.py','selfcheck_OSPFv2_source_contract41.py','ospfv2_whole_bridge41.py','ospfv2_whole_bridge41_v02.py','fix_OSPFv2_bridge_schema42.py','build_OSPFv2_guarded41.py','run_OSPFv2_direct41.py','prepare_Windows_OSPFv2_42.py','prepare_Windows_OSPFv2_crypto_navigation42.py','run_Windows_OSPFv2_42.py','close_Windows_OSPFv2_42.py','run_fuzz_OSPFv2_42.py','run_fuzz_membership.py','run_build_temp.py','run_go_temp.py','replay_public_resume.py','verify_Windows_OSPFv3_36.py','import_OSPFv2_source42.py']
sources=[(root/n,n)for n in root_names]
sources += [(p,'source39/'+p.name)for p in sorted((root/'OSPFv2-independent-source39-01').glob('*.py'))]
sources += [(Path(r'C:\tmp\yak-shark-TLS-metadata-20261028')/n,'runner-dependencies/'+n)for n in('run_go_phase.py','run_build.py')]
sources += [(root/'ospfv2_direct_probe41.go','ospfv2_direct_probe41.go.txt')]
dest.mkdir();records=[]
for src,relative in sources:
 data=src.read_bytes()
 if src.suffix=='.py':ast.parse(data.decode('utf8'),filename=str(src))
 assert src.suffix in('.py','.go')and len(data)<200000
 target=dest/relative;target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data);assert sha(target)==sha(src)
 records.append(dict(path=relative,original_path=str(src),bytes=len(data),SHA256=sha(src),source_language='Go'if src.suffix=='.go'else'Python'))
(dest/'.gitattributes').write_text('*.py -text\n*.go.txt -text\n*.json text eol=lf\n*.md text eol=lf\n',encoding='utf8')
manifest=dict(scope=__doc__,production_source='cd7af6a67e998ed1a6466a11f2b37c66fad742a8',files=records,source_files=len(records),source_bytes=sum(r['bytes']for r in records),excluded=['PCAP/PCAPNG','keylogs','complete per-case plans/matrices/source facts/expected objects','native/CLI exports and ID ledgers','PDML/XML/TSV','console cells/images','binaries','logs','executed proof JSONs'],Go_probe_preserved_as_text='Exact compiled external helper bytes, .go.txt intentionally avoids adding a new package or changing its executed SHA')
(dest/'source-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
print(json.dumps(dict(directory=str(dest),files=len(records),source_bytes=manifest['source_bytes'],manifest_SHA256=sha(dest/'source-manifest.json'))))
