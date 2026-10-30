# 企业流量分析测试集

本目录用于验证企业调查任务，不是协议解析器的通用语料库。

每个案例包含稳定 PCAP/PCAPNG、来源与 SHA256、调查任务、预期观察点和答案边界。公共样本答案只用于复核，不作为工具输入。

## corpus 归档与本地展开

为了避免在 Git 中逐个展开大型捕获和任务文件，远端将案例保存为
`common/bin-parser/testdata/corpus-batches/validation-enterprise-traffic-analysis-20261030.zip`，
并在同目录 `index.json` 中记录每个成员的 SHA256、大小和仓库路径 alias。

Go 测试不需要解压，统一通过 `internal/trafficfixture`：

- `ReadFile` 直接返回 hash-pinned 的只读字节副本。
- `ReadDir` 将归档 alias 与普通源文件合并。
- `Materialize` 仅当 API 必须接收文件名时，把单个 fixture 写入测试临时目录。

本地展开目录按既有开发方式保留，并通过 `.gitignore` 避免重复跟踪。只有在
更新 fixture 内容时才需要生成新 corpus batch；不要在本地修改后直接覆盖
已有 ZIP，也不要绕过 `index.json` 校验。

## 目录

| 目录 | 场景 | 规模 |
| --- | --- | --- |
| `redline-infostealer-2023-07` | 感染身份、HTTP 下载链、TLS SNI、未知端口 C2 | 2,497 包，约 1.4 MiB |
| `host-user-identification` | DHCP、NBNS、mDNS、Kerberos、LDAP、User-Agent | 5 个 PCAP，约 8.1 MiB |
| `https-keylog-dridex` | HTTPS 可见性和授权 keylog 解密 | 1,465 包，618 KiB + keylog |
| `lateral-movement-psexec` | SMB、DCE/RPC、Endpoint Mapper | 264 包，约 203 KiB |
| `dns-tunnel-c2` | DNS TXT 命令、分片下载和 C2 候选 | 301 包，约 35 KiB |
| `http-dga-c2` | HTTP DGA、URL 身份参数、POST 外传候选 | 28 包，约 10 KiB |
| `public-corpus-index` | 后续公开语料接入建议 | 不含大样本 |

## 快速运行

```sh
go build -o /tmp/pcap-inspect ./common/pcapx/cmd/pcap-inspect
go run ./internal/trafficfixture/cmd/corpus verify
```

本地仍有展开 fixture 时可直接传入路径：

```sh
/tmp/pcap-inspect \
  -read testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap \
  -workers 1 -protocol http -rows 20
```

只有 corpus ZIP 时，用现有 `corpus exec` 把 `@alias` 解析为私有临时文件：

```sh
go run ./internal/trafficfixture/cmd/corpus exec -- \
  /tmp/pcap-inspect \
  -read @testdata/enterprise-traffic-analysis/redline-infostealer-2023-07/capture.pcap \
  -workers 1 -protocol http -rows 20
```

```sh
/tmp/pcap-inspect \
  -read testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.pcap \
  -workers 1 -protocol http -rows 20

/tmp/pcap-inspect \
  -read testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.pcap \
  -tls-keylog testdata/enterprise-traffic-analysis/https-keylog-dridex/capture.keys \
  -workers 1 -protocol http -rows 20
```

第一条命令看不到解密后的 HTTPS；第二条应出现 `GET /invest_20.dll` 和 `POST /docs.php`。

## 测试层级

| 层级 | 验证内容 |
| --- | --- |
| L1 | 协议、字段、消息和包关联是否正确 |
| L2 | 能否从企业问题定位到证据 |
| L3 | Finding 是否包含事实、推断、未知、限制和证据 ID |

L1 通过不能替代 L2/L3。

## 安全边界

- 样本来自公开恶意流量训练集，只能在隔离环境分析。
- 不访问样本中的恶意域名、IP 或 C2。
- 不执行导出的文件或 TXT 中的 PowerShell。
- 不把样本凭据、Cookie、token、API key 直接送入模型。
- 不把公开答案、样本文件名或当前仍活跃的基础设施当作工具判断。

## 来源

- Palo Alto Networks Unit 42 Wireshark tutorials and quizzes
- Malware-Traffic-Analysis.net
- sbousseaden/PCAP-ATTACK

版权和再分发条件以各原始来源为准。每个案例的 `manifest.json` 记录下载地址和校验值。
