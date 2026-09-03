# RDP 爆破：协议栈与正反判定

本文件记录 yaklang RDP 爆破（`rdpAuth.BrutePass`）已经验证过的行为。
**通过的就写通过；没打到的不写成通过。**

核心目标：正确区分 **Ok（账密对）** 与 **失败（继续字典）**，
不得把「会话建起来了」当成密码正确。

## 判定规则

| 路径 | 成功 (Ok+Finished) | 失败 (Ok=false, 不 Finished) |
|---|---|---|
| **NLA / CredSSP**（Win7+） | CredSSP 走完（服务端 PubKeyAuth + 客户端 AuthInfo）即 `nla-ok` | `errorCode` / TLS `access denied` / 认证阶段掉线 → `CredSSPError` |
| **SSL**（xrdp 等无 NLA） | FontMap `ready`（无后续 logon PDU） | 目标不可达才 Finished；xrdp 错密码往往只停在图形登录框 |
| **经典 PROTOCOL_RDP**（XP/2003） | `SAVE_SESSION_INFO` 且 `AuthOK()`（INFOTYPE_LOGON / LONG / PLAINNOTIFY） | FontMap `ready` **不是**成功。无 0x26、随后 EOF/超时/复位/登录失败对话框 = 认证失败 |

FontMap 只表示连接序列完成。XP 在 AUTOLOGON 失败时同样会画登录界面。

生产实现：`common/utils/bruteutils/rdp.go` + `grdp/`。
NLA 在 CredSSP 完成后短路，不再开 MCS 图形会话。

## 已验证矩阵

### Mock（每次 `go test` 都跑，来自真机成功路径）

入口：`TestRDPMockMatrixFromLive`。

| 用例 | 模拟的真机经验 | 正确账密 | 错密码 / 未知用户 |
|---|---|---|---|
| `win7-credssp-v2-tls-drop` | Win7：Authenticate 后拆 TLS，无 errorCode | **通过** Ok+Finished | **通过** EOF/拆链 → 认证失败，不 Finished |
| `win11-credssp-v6-errorcode` | Win11：`STATUS_LOGON_FAILURE` | **通过** | **通过** errorCode，不 Finished |
| `credssp-ber-long-form` | 边缘：TSRequest 长度 `82 00 xx`（BER 合法、DER 非法） | **通过** | **通过** errorCode |
| `xp-classic-save-session-info` | XP：0x26 = 成功；失败对话框订单 = 失败 | **通过** 0x26 | **通过** FAILED_XP 绘图订单，不 Finished |

另有 CredSSP v2/v6 Unicode、字典命中、BER 单测 `TestPadBERLongFormRoundTrip` / `TestReadTSRequestBERLeadingZeros`。

经典 mock：`rdp_classic_test.go`。无 X.224 协商 → MCS（EncryptionMethod=0）→
Client Info AUTOLOGON → License `STATUS_VALID_CLIENT` → Demand Active →
Synchronize / Cooperate / Granted / FontMap → 仅正确密码发 0x26。

### 可控真机

| 目标 | 协议 | 正确账密 | 错密码 / 未知用户 |
|---|---|---|---|
| Win7 `127.0.0.1:13390` `rdpuser` / `RdpPass123!` | NLA CredSSP | **通过** Ok+Finished ~0.2–1.5s | **通过** TLS `access denied`，非 Ok，不 Finished ~100ms |
| Win11 `127.0.0.1:13389` `rdpuser` / `RdpPass123!` | NLA CredSSP | **通过** Ok+Finished ~0.3–1.3s | **通过** `STATUS_LOGON_FAILURE`，非 Ok，不 Finished ~200ms |
| xrdp SSL 容器 | PROTOCOL_SSL | 正确账密可达会话 | 错密码无协议级失败信号（已知限制，见 `rdp_real_test.go`） |
| XP SP3 `127.0.0.1:13391` `Administrator` / `RdpPass123!` | PROTOCOL_RDP 5.1 RC4 | 握手完整（RC4、License VALID_CLIENT、FontMap）。**正确账密本镜像不发 0x26**（只推位图），不把 FontMap 当成功。协议成功路径以 mock 的 0x26 为准。 | **未知用户会画登录失败对话框** → `rdp logon failed: xp logon dialog`，不 Finished。错密码同样不 Finished。 |

XP 真机说明（未写成通过）：

- X.224 CC 无协商、SC_SECURITY `EncryptionMethod=2`、Client Info 走 RC4，连接序列完整。
- 已关 `INFO_LOGONERRORS` / fast-path（RDP 5.1 不理解这些，会导致 AUTOLOGON 失效）。
- 已设空 Domain、`INFO_AUTOLOGON|INFO_LOGONNOTIFY`。
- 判定保持「必须 0x26」：宁可超时失败，也不把 FontMap 当成功（早期假阳性）。

跑真机：

```bash
YAK_BRUTE_RDP_ADDR=127.0.0.1:13390 YAK_BRUTE_RDP_USER=rdpuser YAK_BRUTE_RDP_PASS='RdpPass123!' \
  go test ./common/utils/bruteutils/ -count=1 -timeout 90s -v -run TestRDPWindowsLiveBrute
```

调试：`YAK_RDP_GLOG=1`（INFO）或 `YAK_RDP_GLOG=debug`；经典 mock 用 `YAK_RDP_CLASSIC_DEBUG=1`。

## 协议要点

1. **NLA**：CredSSP v6 + SHA256 pubkey binding（CVE-2018-0886）；TLS 1.2（TLS 1.3 会破坏 CredSSP）；TSRequest 兼容 DER，以及边缘情况下的 BER 长长度。
2. **经典 RDP**：无 `rdpNegData` 或 `SSL_NOT_ALLOWED_BY_SERVER` 都回落到 `PROTOCOL_RDP`。
3. **PDU 成帧**：按 Share Control Header `TotalLength` 切 PDU；慢路径 0x02/0x1b 跳过该 PDU 而不是整段流。
4. **本地账户**：Domain 必须空；填 IP 时 XP 不会按用户名 AUTOLOGON。

## 测试入口

```bash
go test ./common/utils/bruteutils/ -count=1 -timeout 90s -run 'RDP|ClassicRDP|NLA|CredSSP'
go test ./common/utils/bruteutils/grdp/protocol/t125/gcc/ ./common/utils/bruteutils/grdp/protocol/x224/ ./common/utils/bruteutils/grdp/protocol/nla/ ./common/utils/bruteutils/grdp/protocol/sec/ ./common/utils/bruteutils/grdp/protocol/pdu/
```
