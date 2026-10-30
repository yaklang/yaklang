# Yak Shark M1：常见协议基础设施与可复核证据

M1 固定已验证的解析、递进展示、字节来源和差分验证基础。协议功能基线是 `92620d72b0c8a1d62ce923e4e69cc1fad5d4c66d`；本阶段收尾只补证据归档、捕获样本存储与消费者兼容，以及这些检查发现的 Windows 路径问题。正在编写但尚未验收的 SMB 修改已单独保存，不进入 M1。

M1 的验收单位是明确的 profile、独立输入及检查合同。不是“所有核心协议完备”，也不是“所有输出与 Wireshark 全部一致”。历史回放保持相同输出，证明解释未回归，不能替代独立协议字段、重组或会话正确性。

## 已关闭的能力范围

| 能力 | 实现与边界 | 验收证据 |
|---|---|---|
| 原生协议入口 | [NativeProtocolProfiles](../../common/pcapx/pcaputil/protocol_registry.go) 给出每个传输、分帧和状态范围；[ProtocolCatalog](../../common/bin-parser/protocol_catalog.go) 保留 partial/未选布局；不能由端口提示或 YAML 存在推断完整支持 | 各 profile 的独立正反例及冻结语义回归；未关闭族单列 |
| TLS 与上层递进 | `tls` 的完整记录、已观察握手/版本和可见性；只有满足上下文和认证条件才投影 HTTP/2、gRPC、DoT/DoH；缺握手时不把 record legacy version 当作协商版本 | [初始 Top30 审计](shark-top30-correctness-wireshark-20261024.md)、[真实 TLS/QUIC](shark-public-corpus-20261025/tls-quic-fresh-d677.md)；测试密钥正确/缺失/错误分开 |
| QUIC 与 HTTP/3 | `quic`、`http3`、`doq` 的选定版本、CID/域隔离、认证数据及受限重组；中途流量或 QPACK 恢复不能凭字段名制造新字节来源 | [恢复与字段投影](shark-public-corpus-20261025/recovery-http3-7e022.md)；高级状态边界继续列入 M2 |
| SSH 初始明文 | `ssh` identification/KEXINIT、选定方法及 sender 角色；未知私名保持 opaque，NEWKEYS 后不推断明文；当前位置字段与携带的连接上下文分开 | [SSH 范围与单棵树](shark-public-corpus-20261025/ssh-ranges-926.md)，独立源、3 项 fuzz、真实 Windows 控制台 |
| 常见服务与控制协议 | DNS 家族、HTTP/升级、SSDP、BGP/AJP/RTMP、选定 SMB2/3、所列服务及网络控制 profile；范围以注册表和各报告为准 | [当前证据导航](shark-public-corpus-20261025/current-counts.md)；目录不把 partial 升为完整协议 |
| 路由与组播字段 | 选定 OSPFv2/v3、RSVP/Intserv、GTPv2-C、IGMP/MLD 等明确布局；未知扩展与不完整投影仍显式报告 | 上述导航中的逐 profile 报告、来源/完整性及 Windows 高亮检查 |
| 来源与 API | 物理帧、重组 PDU、解密/解码载荷、不透明流预览分开；字段范围属于其完整 PDU，不伪装成物理帧偏移；私有拥有值不受公开 DTO 修改影响 | 完整对象、来源引用、重复同值字段位置、跨进程数值折叠、拒绝结果和连接复用回归 |
| 差分方法 | 先冻结原字节与中立事实，再运行实际 SDK/CLI 和隔离的 TShark；逐包编号、长度、域、时钟及字段值/位置分开计数 | [最新大包标签普查](shark-public-corpus-20261025/label-cohorts-b586.md)，379,455 个物理帧；同名标签不等于字段通过 |

## 样本与执行配置

- 37 项公共输入的完整执行基线包含 307,819 个 native 事件、384,518 条 CLI 行和 2 份 flow 文件。53→54 为完整逐字节连续性检查；50→53 只接受提前冻结的 SSH 位置元数据增加，不做 blanket metadata stripping 或任意 ID 重映射。
- 大输入为 Wireshark/公共项目来源的 Weber、Sharon 等固定文件，实际文件、SHA、原始输出及逐帧计划保存在仓库外；两个大包共 379,455 个物理帧。完整报告区分不支持、保守拒绝、字段未映射、参考行为和真实错误。
- Windows 使用 Go 1.22.12、TShark 4.6.9、隔离配置、关闭名称解析并显式开启 TCP 乱序重组。无显式 keylog 的普查与有测试 keylog 的认证解密验收分开。默认 UI 缓存为 20,000 包/64 MiB；控制台编码 CP936。
- `92620d72b0` 的本地五包全量为 285,911 pass /24 skip /0 fail，SSH 专项 1,772 pass；四项精确源码 CI 均首轮成功。它们是功能基线证据，不代替本次归档最终 SHA 的 CI。
- Windows 图片来自实际控制台 cells/attributes 还原，不能称为 Windows Terminal 像素截图；已核对物理帧/PDU 全字节、来源切换、同值不同位置字段、无位置上下文、tabs/mouse/resize。

## 单一捕获存储

迁移前：36 个已跟踪 corpus ZIP，加 709-byte live PCAP 和 202,940-byte PCAPNG gzip，共 **38 个物理文件、18,907,231 字节**。迁移后：一个 **18,916,097-byte** [captures.zip](../../internal/trafficfixture/testdata/captures.zip)，SHA256 `0371e1304d6711497f697ea8a7622c707140e7e5378215d67bde7f0985bd8213`，低于 [GitHub 的 100 MiB 单文件限制](https://docs.github.com/en/repositories/working-with-files/managing-large-files/about-large-files-on-github)。

[完整清单](../../internal/trafficfixture/testdata/captures.manifest.json) 记录 38 项原仓库路径、长度、SHA256。固定排序、1980-01-01 时间、普通文件权限、ZIP Store、空额外元数据，三次构建结果相同。删除原检出文件前，已逐项确认 ZIP 内容与原 Git blob、工作区文件逐字节一致，无遗漏或重名。

旧 ZIP 作为中央 ZIP 的成员保留原始字节，避免改写版本化的历史批次和期望。旧索引 SHA256 `341f5398647d808f6b2edd869794ddd78a3668d577fcbcc5346c03c13fe0bff0` 不变：288 个逻辑批次、24,826 个全部成员、25,174 个路径别名。其中按 PCAP/PCAPNG/CAP 后缀计 11,990 个逻辑捕获成员、11,980 个唯一内容 hash、12,147 个捕获路径别名；再加两个原散装输入。物理 ZIP 成员数、历史版本中的样本数和唯一内容数不是同一计数。

`internal/trafficfixture` 统一读取、验 hash/CRC/边界并返回拥有字节；需要路径时写入调用者的临时目录。`ExportTools` 保留旧 ZIP/脚本路径于私有临时 workspace，使历史封存源码无需改字节；不把散装样本重新生成到源码目录。PCAPDB 的生成器要求仓库外输出目录。来源、许可、说明、期望及旧密码约定均保留，旧密码不是保密边界。

只读取迁移前已跟踪的明确白名单。此前下载的大 PCAP、本机真实抓包、敏感 keylog/密钥、完整字段/PDML/分析输出、日志、二进制和图片不进入此 ZIP。其他系统的 86 个 ZIP 不在本次迁移范围。

## 未关闭问题与交接

1. **SMB1→SMB2 探测停止**：5 个完整 SMB1 多协议 offer 导致请求方向探测停止，漏掉 107 个完整 Direct TCP 记录、121 个 SMB2 命令头。部分流量请求/响应分属 VLAN81/80；不能为追平参考而合并域或伪造请求关联。现有 SMB2 基础 profile 不意味着这条接续链已验收。
2. **SMB2 深字段与会话**：新恢复记录的 compound、literal IDs/继承上下文、所有选定 body/offset/变量区、通配协商后重新协商、重组/复用与 Windows 递进展示，逐项进入 M2。
3. **TLS 高级边界**：缺开头、丢包/乱序后恢复、双向状态、重协商/恢复、key update、无密钥/错误密钥与连接复用的组合矩阵未全部关闭；加密记录归属不等于版本或上层应用已确认。
4. **QUIC 高级边界**：CID/域、迁移/复用、加密级别与乱序丢失、密钥代际及 HTTP/3/QPACK 恢复的剩余组合继续核验；选定版本之外不称支持。
5. **其他字段、会话与 fuzz 差异**：保留 [当前报告](shark-public-corpus-20261025/current-counts.md) 和 [Top30 原始分布](shark-public-corpus-20261025/label-cohorts-b586.md) 中未映射/未实现/参考分歧；历史失败、当前通过与尚未运行分开，不能由测试条数推断全部协议正确。

M1 收尾的最终提交须完成受影响本地回归、必要大包回放、Windows 检查及同一 SHA 的全部必需 CI/专项流量 CI。验收结论和精确链接写入 PR5353 的最终交接记录。完成前不把 M1 标为已通过，也不从旧 SHA 的成功或 skipped 得出结论。全绿后，M2 从该提交单独建 `codex/` 分支，后续功能不再推入 M1 PR。

## 收尾本地验证记录

样本与 Windows 专项 `test-M1-archive55-03`：**24,947 pass /0 skip /0 fail**，
全部 run 有唯一 terminal；日志 SHA256 `24abd3dc1c7cb458ccc944fb9275f66092631f719d6b7cf014325acec8633643`。
先前 `55-02` 的中央存储完整性/pack/PCAPDB 路径/链接控制为 25,045 pass，
其 AI 工具临时数据库 cleanup 失败已在 55-03 修复，未删除断言或改记旧失败。
Windows 导出在写入前设置仅当前用户与 SYSTEM 的继承 DACL，实际根目录及 keylog
用二进制 SID/ACE 校验；Unix 保持 0700/0600。无符号链接创建权限时用测试私有 junction
执行相同拒绝断言，不跳过检查。

Python reader 17 项正反控制通过；此前 3 个 bool/float 冒充整数的红例保留，
补强了精确 JSON 整数类型。两个 Go 首轮失败是 Windows URI 与句柄清理，
另有一个执行器 namespace 覆盖误报，均分开记录，未改变样本或原始预期。

PCAPDB 的 24 个来源、3,368 包/722,388 expanded bytes 在仓库外重新生成，捕获 gzip
和全部 provenance 值与归档前完全相同。生成器用固定 OS=255 的 gzip header，修复
`gzip.compress(mtime=0)` 因平台只改变 header 第9字节而破坏可重复性的行为。

本节是提交前已关闭的本地证据。当前提交后的完整核心回归、新构建回放、Windows
新构建页面及最终 SHA CI 仍需完成，并在最终交接中记录，不提前标绿。
