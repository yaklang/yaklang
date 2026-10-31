# SMB 工作检查点：按用户要求搁置

2026-10-31 用户明确要求“不分析 SMB 的协议，记录 SMB 协议的缺口，然后跳过 SMB”。
本记录冻结已有改动及证据，不再把 SMB 补齐作为后续其他协议工作的前置条件。
[草稿 PR #5380](https://github.com/yaklang/yaklang/pull/5380) 以尚未合并的
[#5379](https://github.com/yaklang/yaklang/pull/5379) 为基线；不宣称本批已经完整验收。

## 已保存的实现与实际验证

代码提交 `76e0bd408eba99a8a989deaef048330fa1ea40f3` 包含：

- TREE_CONNECT_ANDX WC4/WC3、DELETE WC1/WC0 四个有限 canonical/native 入口、
  协议目录、规则归档、typed 字段、Unicode/raw NUL 视图及 literal 范围。
- NEGOTIATE/SESSION_SETUP/TREE_CONNECT/DELETE 共用 PID32/MID/request-direction
  owner；跨 command 复用、实际 lineage 退休、UID/TID 关联、预算及 modern-carrier 隔离。
- SMB 残包的字段查询拒绝、SMB2 已接纳字段与公开缓存的独立所有权，以及 SMB2
  framing/resource/普通 header 错误的私有拒绝。无效 NextCommand 不能再落回宽松旧语法。

最后相关回归 `test-M2-SMB-authority-full126-01`：735 pass /0 skip /0 fail，
覆盖 pcaputil、canonical parser 子包、CLI 的 SMB/CIFS 测试及普通 fuzz seeds。
执行时 HEAD 为6e9983a8，精确 dirty Go/规则/ZIP/工作区输入由 receipt 哈希固定，随后提交为76e0bd4；
不能把 receipt 的 source 字段改写成后来提交。stdout SHA256：
`0f6f5fbbc139e169092aaa65c38f484db0ac8faf9ecb7e5cdac46987e78b7d58`。
新增23个父测试/156个实际 terminal 已列为后续 CI 覆盖清单，尚不是最终 CI 通过证书。

以下是真实的较早构建证据，不冒充76e0bd4的最终重跑：

| 验证 | 代码及实际结果 |
|---|---|
| 原始完整2096帧 SDK | ea8c6c5；四条原23–26记录、89源literal/view在fields/session双副本178行，1821断言，0diff/0gap，两对请求响应关联通过 |
| Windows四页 | ea8c6c5；109个实际native console状态，Header→Body→context、完整物理/PDU、精确高亮、mouse/tabs、80/180列、CP936恢复通过 |
| 大捕获三种导出 | ea8c6c5；42223事件、完整SDK/flow/CLI均与TLS91检查点逐字节一致；这只保持基线，不代表全部Wireshark语义正确 |
| 两项90s插桩fuzz | 6e9983a8；wire110942次/43新覆盖、state27821次/120新覆盖，均0fail；早于最终SMB2修改 |

Windows图像来自实际控制台cells/attributes，未证明Terminal像素、字体、DPI或IME全部行为。
较早109/113运行的QA失败保留：左侧`#23`误匹配右侧pane标题；新增独立console工具修正定位，
并完整识别6/8位offset。并非协议raw字节错误。

## 保留的失败

- fuzz118在约78秒发现 incomplete 被错误当成 typed body error；继续追查后，精确
  red119-02 证明公开缓存能把残包字段查询伪装成成功。原失败 corpus 全留仓库外，最小合成
  输入已作内联回归种子；typed 错误的类别和 plain incomplete 的完整拒绝文字分别保留。
- red122 的两个SMB2用例先在完整消息正控失败：公开协议名/缓存改写替代了原字段。
  修复私有消息标记及公开/私有树别名，green123两个用例通过。
- red125三叶及父均失败：invalid-next-command 被旧语法误解析；framing/resource
  拒绝被公开缓存覆盖。修复后全部纳入full126通过，未删除旧断言。
- 最终76e0bd4插桩fuzz127已构建；用户要求停止SMB后，核实本任务PID/二进制路径并终止。
  exit4294967295记录为用户取消，不能算fuzz通过或新的已分类产品错误；第二目标未启动。

外部证据根：`C:/tmp/yak-shark-ESP-SSH-UI-20261028`。source82/95、runtime115、Windows116、
delta118、red119/122/125、full126、fuzz118/121/127及用户取消receipt保持原值。
原大捕获、完整导出、raw PDU、keylog、密钥和图片均未入库。

## 尚未关闭的SMB缺口

| ID | 缺口及当前证据 | 当前处置 |
|---|---|---|
| M2-04 | 严格Weber checker04仍1400 coverage gaps；已映射值错误0，但full-selected不通过。独立QI映射392项后仍1008（961未发表、47范围未映射）；typed正文/其余命令不完备 | 搁置 |
| M2-05 | 高级compound缺前驱晚到、异步身份及完整文件/tree生命周期仍未全部验收；现有有限TCP重传/乱序/永久洞/新SYN/接口/VLAN矩阵不替代它们 | 搁置 |
| M2-36.01 | 当前四profile仅有限成功布局；其他WC、错误响应、active AndX/secondary fragment、非ASCII OEM等未完整接入 | 搁置 |
| M2-36.02 | 原16条中23–26已接入正文；另外12条的完整正文、状态和连接复用未完成 | 搁置 |
| M2-36.03 | 27–30 NT_CREATE/WRITE只有独立source110的119项事实/32门合同；source117四入口/8测试补丁仅在仓库外，未应用/编译，未接native/FID | 搁置 |
| M2-36.04 | 31–38的TRANSACTION2、SET_INFORMATION2、QUERY_INFORMATION2共8条正文未接入；完整tree/FID及handle复用未证明 | 搁置 |
| M2-36.05 | 76e0bd4最终fuzz取消、最终SDK/大捕获/Windows重建复验未完成；不得用较早构建证据替代 | 搁置 |
| M2-36.06 | 最终说明提交的完整CI及artifact覆盖尚待执行；PR保持draft，CI成功也不替代上一行运行缺口 | 未验收 |
| UI | Packet Name摘要仍有重复；其余SMB页面及全部Windows环境未覆盖 | 搁置 |

不将SMB签名、认证、会话建立或文件操作真实成功由字段/标签推断为已验证。
M2-07的大捕获工作继续处理其他协议，其SMB未关闭部分按本记录保留。
后续非SMB分支从已全绿的8ed202b62d56ceaedf8a00cea0806090daeed193开始，
避免依赖尚未验收的SMB改动。M1 #5353的边界不变，不合入本批SMB工作。
