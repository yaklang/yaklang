"""Write aggregate closed GTPv2 evidence, without importing case/data outputs."""
from pathlib import Path
import hashlib,json,re
root=Path(__file__).parent;repo=Path(r'C:\Users\V\.codex\worktrees\shark-protocol-infrastructure\yaklang');dest=repo/'docs/exec-plans/shark-public-corpus-20261025'
source='df30fc4bd909d6489844f8e0d1cd0ab03826f35c'
def sha(p):return hashlib.sha256(Path(p).read_bytes()).hexdigest()
def read(p):return json.loads(Path(p).read_text('utf8'))
def ref(name):
 p=root/name;assert p.is_file();return dict(external_relative_name=name,SHA256=sha(p),copied=False)
native=read(root/'GTPv2-public-native-literal45-01/summary.json');cli=read(root/'GTPv2-public-CLI-literal45-01/summary.json');fuzz=read(root/'GTPv2-fuzz-df30-45-01/summary.json');small=read(root/'GTPv2-native-CLI-df30-actual45-01/summary.json');windows=read(root/'Windows-GTPv2-root-closure45-02/summary.json')
assert native['after_source']==cli['after_source']==fuzz['source']==small['producer_source']==windows['source']==source
assert native['differences']==cli['differences']==0 and windows['passed']and small['all_frozen_inputs_unchanged']
assert all(r['exit_code']==0 for r in fuzz['targets'])
fuzz_log=root/'GTPv2-fuzz-df30-45-01/FuzzGTPv2BoundedOccurrences.log';assert sha(fuzz_log)==fuzz['targets'][0]['log_SHA256'];fuzz_executions=max(map(int,re.findall(r'execs: (\d+)',fuzz_log.read_text('utf8'))))
contexts={r['name']:read(root/'GTPv2-native-CLI-df30-actual45-01'/r['name']/'verification.json')['counts']for r in small['contexts']}
direct=read(root/'GTPv2-direct-source-audit45-01/summary.json');assert direct['failures']==0
archive=repo/'common/yak/cmd/yakcmds/shark-cli/testdata/live-audit/gtpv2-control45/source-manifest.json';assert archive.is_file()
proof=dict(scope=__doc__,production_source=source,all_core_or_TOP30_complete=False,
 primary=dict(ETSI='TS129274 V18.6.0',PDF_SHA256='c9eefb4832b1549b7fdb9d3abdf9bf7afe4f7da611942ae6e3149e0a807d3be2',Wireshark='4.6.9'),
 original37_GTP=dict(port_hints=5538,UDP_extent_beyond_complete_IP=5535,other_bounded_hints_not_GTP=3,source_admitted_GTP=0,proof=ref('GTP-family-source43-closure01/summary.json')),
 official=dict(physical_frames=6,top_level_IEs=56,group93_child_IEs=8,message_type_counts={'32':1,'95':1,'133':2,'128':1,'129':1},reference_value_span_checks=292,reference_differences=0,source=ref('GTP-official-source43-02/summary.json'),before_actual=ref('GTPv2-official-before-cd7af45-01/summary.json')),
 direct=dict(source_cases=30,unique_physical_inputs=30,authored_scenarios=20,authored_physical_frames=24,counts=direct['counts'],proof=ref('GTPv2-direct-source-audit45-01/summary.json'),producer=ref('GTPv2-direct-actual45-01/producer.json'),frozen_plan=ref('GTPv2-API-source-plan43-01/manifest.json'),export_byte_identical_to_pre_receive_fix=True),
 native_CLI=dict(context_counts=contexts,physical_rows_each_API=54,unique_source_frames=30,modes_reuse_sources=True,registered_descriptors=30,Session_and_GetFields_checks_not_additive_wire_coverage=True,whole_unverified_metadata_not_certified=True,proof=ref('GTPv2-native-CLI-df30-actual45-01/summary.json'),frozen_plan=ref('GTPv2-native-CLI-source-plan44-01/manifest.json'),controller_selftest=ref('GTPv2-native-CLI-controller-selftest44-01/summary.json')),
 public_replay=dict(inputs=37,physical_records=383936,native_events=native['native_event_rows'],CLI_rows=cli['whole_rows'],whole_flow_files=native['whole_flow_files'],all_complete_exports_byte_identical=True,ID_mapping=False,native=ref('GTPv2-public-native-literal45-01/summary.json'),CLI=ref('GTPv2-public-CLI-literal45-01/summary.json')),
 tests=dict(base_source='f1068e2fbaa44d59f7ab82ee6109505bac5b3af1',full_5_packages={'pass':285612,'skip':24,'fail':0},full=ref('test-GTPv2-full-44-08/receipt.json'),latest_selected_IE_filters={'pass':59,'fail':0},selected=ref('test-GTPv2-IE-length-green45-03/receipt.json'),additional_IPv6_dual_address={'pass':1,'fail':0},IPv6=ref('test-GTPv2-IPv6-extension45-04/receipt.json'),original_receive_red=ref('test-GTPv2-IE-length-red45-01/receipt.json'),latest_not_claimed_as_new_full_5_package_run=True),
 fuzz=dict(targets=fuzz['targets'],executions=fuzz_executions,proof=ref('GTPv2-fuzz-df30-45-01/summary.json')),
 Windows=dict(cases=2,default_packet_cache=20000,default_history_mib=64,actual_native_cells_not_Terminal_pixels=True,entire_visible_sources_and_selected_ranges_exact=True,original_failed_IPC_run_preserved=True,only_helper_binding_changed=True,navigation_and_expected_wire_unchanged=True,root_viewed_actual_images=True,proof=ref('Windows-GTPv2-root-closure45-02/summary.json')),
 source_archive=dict(path=str(archive.relative_to(repo)).replace('\\','/'),SHA256=sha(archive),source_files=read(archive)['source_files'],source_bytes=read(archive)['source_bytes']),
 remaining=['mandatory IE/procedure conformance, NAS/subscriber/bearer/session ownership and peer authentication','unselected IE and non-EPC interface semantics','Recovery n>1 variable-length specification ambiguity: counter observed, tail explicitly unselected','more GTP-U, RSVP/Intserv, telecom and TOP30 advanced coverage'],captures_keylogs_full_cases_outputs_images_binaries_not_repository=True)
target=dest/'gtpv2-df30-summary.json';assert not target.exists();target.write_text(json.dumps(proof,indent=2)+'\n',encoding='utf8')
text=(root/'GTPv2-report-draft44.md').read_text('utf8')
text+='''

## 原生、CLI 与实际前后差别

旧版 `cd7af6` 与新版 `df30fc4` 均实际运行同一份官方六帧、相同关闭 embedded secret 的参数。旧 SDK 六帧都是 TURN ChannelData / context-required（没有 ChannelBind），旧 CLI 六行均为 UDP；新版六帧均为 decoded GTPv2-C，逐字段/范围证据和具体消息名已核对。不能将旧状态说成六个成功解码的 TURN 会话。

| 模式 | 物理行数 | 完整 GTPv2 | 显式诊断（仍显示 UDP） | 拒绝 GTP 身份 | private typed 值 |
|---|---:|---:|---:|---:|---:|
| 官方自动 | 6 | 6 | 0 | 0 | 422 |
| authored 自动 | 24 | 13 | 0 | 11（含1 carrier拒绝） | 1051 |
| authored 显式 | 24 | 13 | 10 | 1（carrier拒绝） | 1209 |

模式复用同30帧，不是54个独立样本。54条 SDK/CLI snapshot 桥及30个 descriptor(name/type/bits/origin)全过。官方 Session 与 GetFields 各307 literal/178 derived/70 names，是两份 API 检查，不能相加作614个独立 wire 叶。显式诊断保留 Session 部分字段；没有将 GetFields 未确认内容、opaque bytes 或未知 metadata 冒称完整语义。

## 接收长度与证据边界

ETSI §7.7.7、Table8.1、§8.4/8.5/8.8/8.22的复核修正：EBI 和 F-TEID 保留可证前缀及 owned 忽略扩展；F-TEID V4/V6至少一位，最小value为9/21/25；短字段、无地址类型和Cause非法n（仅允许2或6）有明确错误。Recovery表项称Variable Length而正文规定1 octet；n>1保留counter和owned未选尾并返回Context，未伪称规范必然要求Format。IPv6与双地址扩展有精确半开范围和独立所有权检查。

旧长短IE处理有实际失败执行（11个子例失败、1个子例通过，父测试也失败）；修后相关59项及额外IPv6/双地址1项全过。中间一次native测试误用了未发布的底层decoder，故私有alias尚未封存；随后改用真实auto/explicit捕获发布入口，没有降低预期。完整五包285612通过/24跳过属于f1068基线；不冒称最新df30又跑了一遍完整五包。

## 完整回放、Windows 与 fuzz

df30用新构建二进制实际跑37输入，307818 native完整对象、384518 CLI完整行及两份whole flow均与f1068逐字节相同；没有字段/ID规范化。它证明这组导出保留，不能证明所有未变解释都正确。

Windows默认20k/64MiB，两张实际页面来自官方frame2与authored双消息frame5，均为精确抽出的单包检查文件，不是新现场抓包或全大包缓存能力声明。Source0完整物理字节、Source1完整PDU、第二消息自身MessageType范围、鼠标/tabs/resize/CP936恢复与实际cells/attrs核验通过；root看过两张选中图。图片是实际console字符/颜色还原，不是Windows Terminal像素截图。

首次旧helper把WriteFile尚未写完的request读成空名而失败，原件保留；现helper忽略空request并在完整JSON关闭后发布完成ack。只改helper binding，原byte/name/range及导航均未改。闭合checker随后修正waitShot别名对ack的机械对应：必须匹配已完成candidate的精确JSON hash，不改内容预期或重新制造图片。

Fuzz实际运行和精确log/receipt/hash见聚合JSON。PCAP、完整facts/expected/exports、PDML、logs、cells/PNG与exe均外置。仓库仅有纯执行源码、来源依赖清单和聚合报告。

## 尚未完成

本阶段未证明全部IE、GTP-U、NAS/订户/承载状态或TOP30完整性。RSVP下一阶段已取得9帧/3种PDU、55对象的独立来源，39基础对象可核对，16 Intserv对象body仍opaque；这不是RSVP产品实现成果。全套hash、范围与逐例事实仍在外部保留。
'''
report=dest/'gtpv2-df30.md';assert not report.exists();report.write_text(text,encoding='utf8')
counts=dest/'current-counts.md';old=counts.read_text('utf8');counts.write_text('df30新增部分EPC GTPv2-C canonical/native/private字段与递进页面，官方6帧及20场景24帧独立核验、接收扩展长度修复、37输入整对象/行/flow保留和两张Windows页见[GTPv2报告](gtpv2-df30.md)。不是新TOP30全量统计，也未证明完整承载会话。\n\n'+old,encoding='utf8')
print(json.dumps(dict(report=str(report),summary=str(target),summary_SHA256=sha(target),source=source)))
