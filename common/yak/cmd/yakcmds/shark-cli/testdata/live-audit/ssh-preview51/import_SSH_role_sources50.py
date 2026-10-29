"""Import reviewed aggregate role50 evidence and exact Python source; no packet artifacts."""
from pathlib import Path
import ast,hashlib,json,subprocess
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
stage=root/'SSH-role-report-stage50-03'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
assert sha(stage/'staging-manifest.json')=='97407c64cd5b44e95e57889e1573e3368b9f316b0daf19509c5ddff4a3aea75e'
assert sha(stage/'report.md')=='ee4511d9a0b6f99592ea021148475d02df92f5f3c4cea3580fe3d0cffbbce7e9'
assert sha(stage/'aggregate-summary.json')=='181d3107387c55e35d0060b52feb4229b27c073abd5c0882f67d819d138a8649'
manifest=stage/'source-archive/source-manifest.json'
assert sha(manifest)=='2b52719496d379824626c6ec8a9c539aa94312f1f17ba6f03430e30bad3b02ba'
inv=read(manifest);assert inv['source_files']==23 and inv['source_bytes']==194175
dest=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ssh-role50'
assert not dest.exists();dest.mkdir()
for f in inv['files']:
 relative=Path(f['path']);assert not relative.is_absolute()and'..'not in relative.parts and relative.suffix=='.py'
 source=stage/'source-archive'/relative;original=Path(f['original_path']);raw=source.read_bytes()
 assert sha(source)==sha(original)==f['SHA256']and len(raw)==f['bytes']
 ast.parse(raw,filename=str(source));target=dest/relative;assert target.resolve().is_relative_to(dest.resolve())
 target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(raw);assert sha(target)==f['SHA256']
for d in inv['existing_repository_dependencies']:
 assert sha(repo/d['path'])==d['SHA256']
 assert sha(repo/d['manifest_path'])==d['manifest_SHA256']
(dest/'source-manifest.json').write_bytes(manifest.read_bytes())
(dest/'.gitattributes').write_text('*.py -text\n*.json -text\n',encoding='utf8')
(dest/'README.md').write_text('''# SSH observed method roles source50

These23 exact historical Python sources retain failed and corrected source,
reference and SDK/CLI schema versions. The source manifest records SHA256,
original external paths and existing repository dependencies. Rehydrating the
original external layout or writing an explicitly versioned path adapter is
required; archived sources are not an implicit new execution.

The115-frame source is authored, not a live SSH session. Four positive role
contradictions are independent of prior-message absence. Source50 bad flows end
with FIN; subsequent valid-message recovery is a separate Go regression proof.
The64-byte post-NEWKEYS preview has logical Length192. Its hash covers only the
retained prefix; full-tail source proof comes from the external capture input.

No PCAP/keylog/raw data, whole expected plans, runtime exports, receipts, image,
log, binary, downloaded RFC/vendor source or future wire-range ledger is stored
here. The small source-manifest.json inventories source code only. The old26f
preview caption is a preserved UI issue; UI51 fixes and actual runs are separate.
''',encoding='utf8')
docs=repo/'docs/exec-plans/shark-public-corpus-20261025'
for name,filename in[('report.md','ssh-role-26f.md'),('aggregate-summary.json','ssh-role-26f-summary.json')]:
 target=docs/filename;assert not target.exists();target.write_bytes((stage/name).read_bytes());assert sha(target)==sha(stage/name)
counts=docs/'current-counts.md';old=counts.read_text('utf8');assert not old.startswith('26f')
counts.write_text('26f按已观察TCP角色拒绝SSH错误方向的方法消息，保留SSH身份/具体诊断并允许后续完整合法消息恢复。独立115物理帧、645字段、37输入完整输出、285825全量、两项fuzz和四项CI见[角色报告](ssh-role-26f.md)。旧加密预览标题缺陷单列，未夸大成物理帧损坏；UI51另行验证。\n\n'+old,encoding='utf8')
counts_json=docs/'current-counts-summary.json';data=read(counts_json);assert 'ssh_role_26f'not in data
data['ssh_role_26f']=dict(source=inv['production_source'],report='ssh-role-26f.md',summary='ssh-role-26f-summary.json',
 summary_SHA256=sha(docs/'ssh-role-26f-summary.json'),source_manifest_SHA256=sha(dest/'source-manifest.json'),fresh_TOP30_label_census=False)
counts_json.write_text(json.dumps(data,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
main=repo/'docs/exec-plans/shark-public-corpus-wireshark-20261025.md'
with main.open('a',encoding='utf8')as f:f.write('\n\n## SSH已观察发送端角色与恢复\n\n26f修复DH/ECDH/GEX错误方向的字段和主机密钥误解析。独立115物理帧/64payload加7FIN诊断、source49兼容、两项fuzz、全量285825及四项CI已闭合。source50的bad流只有FIN，后续合法消息恢复由单独144项拆包正反控验证；旧预览标题保留为已知UI差别，见[报告](shark-public-corpus-20261025/ssh-role-26f.md)。\n')
proof=dict(production_source=inv['production_source'],source_files=23,source_bytes=194175,
 source_manifest_SHA256=sha(dest/'source-manifest.json'),staging_manifest_SHA256=sha(stage/'staging-manifest.json'),
 reports_byte_identical_to_reviewed_stage=True,source_files_byte_identical_to_original=True,no_packet_artifacts_imported=True)
with(root/'SSH-role-import50-01.json').open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
print(json.dumps(proof))
