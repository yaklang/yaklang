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
| **经典 PROTOCOL_RDP**（XP/2003） | `SAVE_SESSION_INFO`（0x26）且 `AuthOK()`，**或** FontMap 后再发 Deactivate All / 二次 Demand Active（会话切换），或 ncrack 成功绘图订单 | FontMap **不是**成功。FAILED_XP 绘图订单 / EOF / 复位 / 超时无正向信号 = 失败。大位图不是成功 |

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
| `xp-classic-no-0x26` | 真机 XP：成功不发 0x26，FontMap 后 Deactivate All | **通过** | **通过** FAILED_XP |

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
| XP SP3 x86 KVM `192.168.3.218:13391` `rdpuser` / `RdpPass123!` | PROTOCOL_RDP 5.1 RC4 | **通过** Ok+Finished（FontMap 后二次 Demand Active，无 0x26） | **通过** ~0.5s `rdp logon failed: xp logon dialog`，不 Finished |

XP 真机说明：

- X.224 CC 无协商、License `STATUS_VALID_CLIENT`、FontMap 后进入图形。
- **0x26 不是唯一依据，这台 XP 成功路径根本不发。** 解密 PDU 里没有 `17 00 … 26`。
- 成败都会先推同一批墙纸位图，**大位图不能当成功**，**超时无失败也不能当成功**。
- 失败：ncrack `FAILED_XP` 绘图订单，约 0.5s。
- 成功正向信号：FontMap 之后的 **Deactivate All / 二次 Demand Active**（登录后会话切换）。0x26 若出现仍立即成功。
- 已关 `INFO_LOGONERRORS` / fast-path；Domain 空；`INFO_AUTOLOGON|INFO_LOGONNOTIFY`。LOGONNOTIFY 没让这台 XP 发出 0x26。

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
5. **经典成功判定**：0x26 是充分条件。真机 XP 用 FontMap 后的会话切换（Deactivate All / 二次 Demand Active）作为正向成功；失败靠绘图订单。超时不是成功。

## 测试入口

```bash
go test ./common/utils/bruteutils/ -count=1 -timeout 90s -run 'RDP|ClassicRDP|NLA|CredSSP'
go test ./common/utils/bruteutils/grdp/protocol/t125/gcc/ ./common/utils/bruteutils/grdp/protocol/x224/ ./common/utils/bruteutils/grdp/protocol/nla/ ./common/utils/bruteutils/grdp/protocol/sec/ ./common/utils/bruteutils/grdp/protocol/pdu/
```
