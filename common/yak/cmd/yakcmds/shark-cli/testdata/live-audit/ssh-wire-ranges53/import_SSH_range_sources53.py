"""Archive exact controller source only, never source facts or runtime exports."""
from pathlib import Path
import ast,hashlib,json,subprocess
P=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ssh-wire-ranges53'
assert not dest.exists()
sha=lambda p:hashlib.sha256(Path(p).read_bytes()).hexdigest()
head=subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()
pairs=[]
for directory,group in [('SSH-wire-ranges-source51-01','wire-plan'),('SSH-wire-ranges-API-CLI-source53-01','API-plan')]:
 for source in sorted((P/directory).glob('*.py')):pairs.append((source,Path(group)/source.name))
inventory=P/'SSH-ranges-public-preservation53-01/source-files53.json'
assert sha(inventory)=='47c205a893c6b771d27d4e08e0423e89ea3b56510490d36353c00249b19b2147'
for name in('inventory_old50_ssh53.py','ssh_old_wire53.py','ssh_public_ranges53_v02.py','summarize_public_ranges53.py'):
 pairs.append((P/'SSH-ranges-public-preservation53-01'/name,Path('public-preservation')/name))
for name in('run_SSH_ranges_native53.py','prepare_Windows_SSH_ranges53.py','run_Windows_SSH_ranges53.py','close_Windows_SSH_ranges53.py','close_Windows_SSH_ranges53_v02.py','close_Windows_SSH_ranges53_v03.py','close_Windows_SSH_ranges53_v04.py','run_fuzz_SSH_ranges53.py','run_current_label_census53.py','stage_SSH_wire_ranges53.py','import_SSH_range_sources53.py'):
 pairs.append((P/name,Path(name)))
assert len(pairs)==18 and len({str(v)for _,v in pairs})==len(pairs)
for source,_ in pairs:ast.parse(source.read_text('utf8'),filename=str(source))
dest.mkdir();rows=[];whitespace=[]
for source,relative in pairs:
 target=dest/relative;target.parent.mkdir(exist_ok=True,parents=True)
 with target.open('xb')as f:f.write(source.read_bytes())
 assert source.read_bytes()==target.read_bytes()
 rows.append(dict(path=relative.as_posix(),source=str(source),bytes=source.stat().st_size,SHA256=sha(source)))
 if source.read_bytes().replace(b'\r\n',b'\n').endswith(b'\n\n'):whitespace.append(relative.as_posix())
attributes='*.py -text whitespace=blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol\n*.json -text whitespace=blank-at-eol,blank-at-eof,space-before-tab,cr-at-eol\n'
for name in whitespace:attributes+=name+' whitespace=blank-at-eol,-blank-at-eof,space-before-tab,cr-at-eol\n'
(dest/'.gitattributes').write_text(attributes,encoding='utf8')
dependencies=[]
for archive in('ssh-identity49','ssh-role50','ssh-preview51','rsvp-control47'):
 m=dest.parent/archive/'source-manifest.json';assert m.is_file();dependencies.append(dict(path=str(m.relative_to(repo)).replace('\\','/'),SHA256=sha(m)))
manifest=dict(schema='ssh-owned-wire-range-audit-source-v1',scope='Only authored Python controllers and aggregate dependency/hash metadata; no PCAP, keylog, raw bytes, source rows, expected grids, SDK/CLI/PDML exports, runtime receipts, logs, binaries or images.',files=rows,files_count=len(rows),source_bytes=sum(x['bytes']for x in rows),AST_parse_passed=True,exact_original_bytes=True,dependencies=dependencies,source_wire_range_plan_SHA256='8739f8fed549047f117efbda6b38c0a8914f6d94cf2269a5e8c658fac28f6de3',source_API_range_plan_SHA256='30a5ee3060b3bb4678e6bfb355324a0ba3a3f404fffbaff1b402e127d9b6d436',public_preservation_plan_SHA256='8ecf8aaf15f14d940cbbd27647746ce0eeb55927241c60435fe8b0307886187e',historical_versions_preserved='API first prepare and Windows closure1/2/3 failures are controller assumptions, not rewrites of source bytes or facts; Windows closure4 passes with literal-selection-aware source cycling and fixed packet-row identity.',all_original_archived_sources_unmodified=True)
(dest/'source-manifest.json').write_text(json.dumps(manifest,indent=2)+'\n',encoding='utf8')
(dest/'README.md').write_text('SSH literal field-range audit controllers.\n\nThe manifest pins exact original source bytes and existing source-archive dependencies. Full captures, expected field grids, neutral facts, runtime exports, logs and images remain outside Git. Public preservation checks prove only that old interpretations were retained except for source-derived ranges; authored source49/50 checks provide independent field/range truth. Historical controller failures are retained and distinguished from product defects.\n\nThe subsequent single-tree display and sealed-line-oracle correction are separate implementation steps; this archive does not turn their pending checks into completed evidence.\n',encoding='utf8')
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=repo,text=True).strip()==head
receipt=dict(parent_HEAD=head,destination=str(dest),source_count=len(rows),source_bytes=manifest['source_bytes'],manifest_SHA256=sha(dest/'source-manifest.json'),all_copies_byte_identical=True,AST_parse_passed=True,Go_or_rules_changed=False,whitespace_exceptions=whitespace)
out=P/'SSH-range-source-import53-01.json';assert not out.exists();out.write_text(json.dumps(receipt,indent=2)+'\n',encoding='utf8')
print(json.dumps(receipt))
