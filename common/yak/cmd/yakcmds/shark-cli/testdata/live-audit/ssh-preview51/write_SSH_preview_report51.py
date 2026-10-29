"""Archive exact UI51 controllers and closed aggregate caption/source evidence."""
from pathlib import Path
import ast,hashlib,json
root=Path(__file__).parent
repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang')
docs=repo/'docs/exec-plans/shark-public-corpus-20261025'
source='7fcbf0ac931c6881fd5a3baf4eba8d5c9c9889f8'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf-8-sig'))
def ref(p):return dict(external_proof=p,SHA256=sha(root/p))
closed=read(root/'Windows-SSH-preview-root-closure51-01/summary.json')
tests=read(root/'test-SSH-preview-green51-03/receipt.json')
audit=read(root/'SSH-role-native-7fcb51-01/independent-audit03/summary.json')
assert closed['passed']and closed['source']==source and closed['actual_console_cases']==3
assert tests['exit_code']==0 and tests['test_actions']=={'pass':895,'skip':2}and tests['all_runs_have_unique_terminal']
assert audit['passed']and audit['counts']['API_literal_occurrences']==645 and audit['counts']['private_SSH_occurrences']==243
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/ssh-preview51';assert not archive.exists();archive.mkdir()
names=['run_go_CLI51.py','stage_SSH_preview51.py','prepare_Windows_SSH_preview51.py','run_Windows_SSH_preview51.py',
 'close_Windows_SSH_preview51.py','import_SSH_role_sources50.py','import_SSH_role_sources50_v02.py','write_SSH_preview_report51.py']
files=[]
for name in names:
 p=root/name;raw=p.read_bytes();ast.parse(raw,filename=str(p));(archive/name).write_bytes(raw)
 assert sha(archive/name)==sha(p)
 files.append(dict(path=name,original_path=str(p),bytes=len(raw),SHA256=sha(p),language='Python'))
dependencies=[]
role=archive.parent/'ssh-role50/source-manifest.json';assert sha(role)=='2b52719496d379824626c6ec8a9c539aa94312f1f17ba6f03430e30bad3b02ba'
old=archive.parent/'rsvp-control47/source-manifest.json';assert sha(old)=='61e94c2263e9d725cd758599fba9cb86bba117e6b619bd50a86657e7fcd14cce'
for manifest,wanted in[(role,'run_Windows_SSH_role50.py'),(role,'run_SSH_role_native50.py'),(old,'run_go_temp.py'),(old,'run_build_temp.py'),(old,'verify_Windows_OSPFv3_36.py')]:
 inv=read(manifest);f=next(f for f in inv['files']if Path(f['path']).name==wanted)
 p=manifest.parent/f['path'];assert sha(p)==f['SHA256']
 dependencies.append(dict(path=p.relative_to(repo).as_posix(),SHA256=sha(p),manifest=manifest.relative_to(repo).as_posix(),manifest_SHA256=sha(manifest)))
inv=dict(production_source=source,scope=__doc__,source_files=len(files),source_bytes=sum(f['bytes']for f in files),files=files,
 existing_repository_dependencies=dependencies,exact_source_bytes_not_normalized=True,
 excludes=['packet captures/keylogs/raw data','whole source plans/expected/export files','runtime receipts/logs','images/cells/binaries','full RFC/vendor sources'],
 historical_failed_importer_retained=True,future_wire_range_ledger_not_included=True)
with(archive/'source-manifest.json').open('x',encoding='utf8')as f:json.dump(inv,f,indent=2);f.write('\n')
(archive/'.gitattributes').write_text('*.py -text\n*.json -text\n',encoding='utf8')
(archive/'README.md').write_text('''# SSH encrypted byte-source captions UI51

Eight exact historical Python controllers are archived with SHA256. This UI51
campaign is separate from the future wire-range proposal whose external folder
also carries a source51 label. No wire ranges are implemented by this change.

Three actual Windows console cases preserve full physical capture bytes while
distinguishing a refused complete message, a retained64/observed192 preview and
a fully retained48-byte opaque stream arrival. An observation ID does not prove
a complete SSH PDU. SDK/private pdu.id, Raw/Length and physical metadata stay intact.

The original role50 importer failed on an optional shared-dependency manifest
field; v02 resumes only hash-verified partial Python files without deleting or
moving them. These are controller/schema failures, not protocol failures.

Rehydrate original external paths or explicitly version a new path adapter.
Captures, keys, raw data, whole plans/expected/exports, receipts, images, logs,
binaries and downloaded primary sources remain external. The source manifest
inventories code, not captured traffic or a new execution.
''',encoding='utf8')
proof=dict(source=source,scope='SSH encrypted byte-source captions only; no backend grammar/SDK identity change.',
 all_core_or_TOP30_complete=False,actual_console=closed,
 tests=dict(package='common/yak/cmd/yakcmds/shark-cli',pass_count=895,skip_count=2,fail_count=0,
  test_models_JSON_roundtrip_not_full_worker_pipeline=True,receipt=ref('test-SSH-preview-green51-03/receipt.json'),
  source_index_bridge=ref('SSH-preview-stage51-01.json'),red_regression=ref('test-SSH-preview-red51-01/receipt.json')),
 SDK_CLI=dict(source=source,all_source50_trees_and_snapshots_passed=True,counts=audit['counts'],
  proof=ref('SSH-role-native-7fcb51-01/independent-audit03/summary.json')),
 public_suite_basis=dict(checked_source='26f01f4baa5fd39b88deadf901257a70bc5c1dfe',fresh_UI51_37_suite_not_claimed=True,
  unchanged_native_exporter_and_flow_binaries=closed['native_binary_bridges'],
  all115_frame_SDK_CLI_descriptors_exports_byte_identical=closed['closed_SDK_CLI_export_bridges'],
  native37=ref('SSH-role-public-native-bridge50-01/summary.json'),CLI37=ref('SSH-role-public-CLI-bridge50-01/summary.json')),
 old_frame_length_misread_was_not_product_corruption=True,
 old_caption_gap=ref('Windows-SSH-role-root-closure50-01/summary.json'),
 closure=ref('Windows-SSH-preview-root-closure51-01/summary.json'),
 source_archive=dict(source_files=len(files),source_bytes=inv['source_bytes'],manifest=archive.relative_to(repo).as_posix(),SHA256=sha(archive/'source-manifest.json')),
 CI='Consult exact latest PR checks; no future/superseded CI result is manufactured in this source checkpoint.',
 limits=['No authenticated SSH plaintext or SFTP decode is claimed.','Opaque bytes are synthetic controls, not new live SSH.',
  'SSH GetFields/typed/UI leaf wire ranges remain a separate future task.','Actual images reconstruct console cells/attributes, not Terminal pixels.'])
with(docs/'ssh-preview-7fcb-summary.json').open('x',encoding='utf8')as f:json.dump(proof,f,indent=2);f.write('\n')
report='''# SSH加密字节来源：UI 7fcb

7fcb修复把NEWKEYS后不透明流字节称为完整PDU的展示。只有既有的明确SSH加密诊断
选择这条路径；普通SSH、错误方向的malformed消息、TLS解密子消息和QUIC沿用原来源规则。
左侧用observation记录已保留的观察，私有pdu.id和查询身份保持不变。

## 实际Windows页面

同一115物理帧的人工来源跑了3个实际控制台案例；不是新增真实SSH连接。

| 情况 | 物理帧 | 实际捕获源 | 当前保留源 | 页面标题 |
|---|---:|---:|---:|---|
| 客户端错误发送reply |36|246B|完整192B消息封装|SSH · reassembled PDU，malformed，具体角色诊断|
| NEWKEYS后截余预览 |13|246B|64/192B|SSH · encrypted stream preview · 64/192 observed bytes|
| NEWKEYS后整段保留 |11|102B|48/48B|SSH · encrypted stream bytes · 48 observed bytes|

三种情况的完整原始捕获、保留源和切换后的字节逐一与独立来源核对。帧长度始终正确。
选择Expected Sender Role=server不制造字节高亮。加密两种情况均不再自称完整PDU，
48B全部保留也没有被当作已解码消息。原始115个缓存包、默认20000容量和64MiB历史
设置、CP936恢复、鼠标/标签/resize均检查。图像来自实际控制台cells/attributes，
不是Windows Terminal像素截图。原先64/64长度的视觉误读已纠正，不是产品数据损坏。

## 验证口径

CLI全包895通过、2跳过、0失败；新增回归先出现7个失败动作/6个通过动作，修复后全过。
其中JSON往返只证明DTO字段的序列化，真实页面由上面3个实际运行补充验证。
新版重新运行source50的SDK/CLI：645个API原语、243个SSH别名、12项描述符、
64个payload加7条FIN诊断的完整树及桥均通过。三个完整导出与26f逐字节一致。

新版events/flows二进制也与26f逐字节相同，后台Go/rules没有变化。37输入完整回放的
307819个native对象、384518条CLI记录和两份whole-flow连续性依据仍是26f闭合执行；
本报告不把它写成新做的7fcb大样本回放，也不把同字节当作全部协议正确性证明。
CI以PR当前精确提交的检查为准，不在报告中预填未来成功状态。

8个精确Python控制器及聚合摘要已归档。完整PCAP/keylog/raw、plans/expected、
导出/收据/日志、图像/二进制、完整RFC/vendor C仍在仓库外。SSH叶子范围、完整重协商、
主机/签名认证和TOP30完备仍未由本检查点证明。外部future wire-range source51账本
只是下一步准备，和本UI51修复是两个独立工作。
'''
with(docs/'ssh-preview-7fcb.md').open('x',encoding='utf8')as f:f.write(report)
counts=docs/'current-counts.md';counts.write_text('7fcb完成SSH加密字节来源的Windows三场景闭合：整段48B仍是opaque流，截余64/192明确preview，原始帧/角色诊断/来源切换不变。CLI895通过，后台exporter/flow二进制及115帧完整SDK/CLI导出与26f同字节；[实际页面与口径](ssh-preview-7fcb.md)。这不实现SSH叶子范围或声称TOP30完备。\n\n'+counts.read_text('utf8'),encoding='utf8')
counts_json=docs/'current-counts-summary.json';data=read(counts_json);assert 'ssh_preview_7fcb'not in data
data['ssh_preview_7fcb']=dict(source=source,report='ssh-preview-7fcb.md',summary='ssh-preview-7fcb-summary.json',summary_SHA256=sha(docs/'ssh-preview-7fcb-summary.json'),fresh_TOP30_label_census=False)
counts_json.write_text(json.dumps(data,indent=2,ensure_ascii=False)+'\n',encoding='utf8')
main=repo/'docs/exec-plans/shark-public-corpus-wireshark-20261025.md'
with main.open('a',encoding='utf8')as f:f.write('\n\n## SSH加密预览与来源页面\n\n7fcb将NEWKEYS后观察改称observation，明确encrypted stream bytes或保留/消费preview计数。三张Windows真实控制台与字节源闭合、895项CLI检查及不变的SDK/flow二进制和小来源完整导出见[报告](shark-public-corpus-20261025/ssh-preview-7fcb.md)。原始帧未损坏；完整消息和不透明流的来源层次分开。\n')
print(json.dumps(dict(source=source,source_files=len(files),source_bytes=inv['source_bytes'],archive_manifest_SHA256=sha(archive/'source-manifest.json'),report_summary_SHA256=sha(docs/'ssh-preview-7fcb-summary.json'))))
