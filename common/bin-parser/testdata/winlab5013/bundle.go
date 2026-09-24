package main

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type capSpec struct {
	File       string
	Title      string
	Gap        string
	Shark      string
	Metrics    string
	Unfinished string
	Build      func(*lab)
	Parse      func([]Frame) (string, error)
}

type zipBundle struct {
	Name  string
	Title string
	Intro string
	Caps  []capSpec
}

func allBundles() []zipBundle {
	return []zipBundle{bundle20(), bundleICS(), bundlePower(), bundleBlue(), bundleFTP(), bundleDNS()}
}

func specByFile(name string) (capSpec, bool) {
	for _, b := range allBundles() {
		for _, c := range b.Caps {
			if c.File == name {
				return c, true
			}
		}
	}
	return capSpec{}, false
}

func verifyNamed(name string, pcap []byte) (string, error) {
	if len(pcap) == 0 {
		return "", fmt.Errorf("%s: capture missing", name)
	}
	spec, ok := specByFile(name)
	if !ok {
		return "", fmt.Errorf("unknown capture %s", name)
	}
	frames, err := parsePcapng(pcap)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	body, err := spec.Parse(frames)
	if err != nil {
		return "", fmt.Errorf("%s: %w", name, err)
	}
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	return "file=" + name + "\n" + body, nil
}

func verifyDir(dir string) (string, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	var names []string
	for _, e := range ents {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".pcapng") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return "", fmt.Errorf("no pcapng in %s", dir)
	}
	var b strings.Builder
	for i, name := range names {
		raw, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return "", err
		}
		out, err := verifyNamed(name, raw)
		if err != nil {
			return "", err
		}
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(out)
	}
	return b.String(), nil
}

func buildCapture(spec capSpec) []byte { return pcapOf(spec.Build) }

func bundleCritical(b zipBundle) (map[string][]byte, string, error) {
	pcaps := map[string][]byte{}
	var text strings.Builder
	for i, c := range b.Caps {
		raw := buildCapture(c)
		pcaps[c.File] = raw
		out, err := verifyNamed(c.File, raw)
		if err != nil {
			return nil, "", err
		}
		if i > 0 {
			text.WriteByte('\n')
		}
		text.WriteString(out)
	}
	return pcaps, text.String(), nil
}

func renderREADME(b zipBundle, pcaps map[string][]byte, critical string) string {
	var doc strings.Builder
	doc.WriteString("# " + b.Title + "\n\n")
	doc.WriteString("SPDX-License-Identifier: CC0-1.0\n\n")
	doc.WriteString("来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。\n\n")
	doc.WriteString(b.Intro)
	doc.WriteString("\n\n## 怎么验证\n\n")
	doc.WriteString("在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。\n\n")
	doc.WriteString("下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。\n\n")
	doc.WriteString("BEGIN CRITICAL\n")
	doc.WriteString(critical)
	if !strings.HasSuffix(critical, "\n") {
		doc.WriteByte('\n')
	}
	doc.WriteString("END CRITICAL\n\n")
	for _, c := range b.Caps {
		sum := sha256.Sum256(pcaps[c.File])
		fmt.Fprintf(&doc, "## %s\n\n", c.Title)
		fmt.Fprintf(&doc, "- 文件：`captures/%s`\n", c.File)
		fmt.Fprintf(&doc, "- SHA-256：`%x`\n", sum)
		fmt.Fprintf(&doc, "- 相对 #5013 的缺口：%s\n", c.Gap)
		fmt.Fprintf(&doc, "- Wireshark / tshark：%s\n", c.Shark)
		fmt.Fprintf(&doc, "- 核心指标：%s\n", c.Metrics)
		fmt.Fprintf(&doc, "- 验证标准：`go run .` 打印的对应 `file=%s` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。\n", c.File)
		fmt.Fprintf(&doc, "- 还不算做完：%s\n\n", c.Unfinished)
	}
	return doc.String()
}

func zipBytes(b zipBundle) ([]byte, error) {
	pcaps, critical, err := bundleCritical(b)
	if err != nil {
		return nil, err
	}
	readme := renderREADME(b, pcaps, critical)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	when := time.Date(2026, 9, 23, 0, 0, 0, 0, time.UTC)
	add := func(name string, data []byte) error {
		h := &zip.FileHeader{Name: name, Method: zip.Deflate, Modified: when}
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	}
	if err := add("README.md", []byte(readme)); err != nil {
		return nil, err
	}
	if err := add("go.mod", []byte("module winlabverify\n\ngo 1.20\n")); err != nil {
		return nil, err
	}
	srcs, err := sourceFiles()
	if err != nil {
		return nil, err
	}
	for _, s := range srcs {
		if err := add(s.name, s.data); err != nil {
			return nil, err
		}
	}
	names := make([]string, 0, len(pcaps))
	for n := range pcaps {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if err := add("captures/"+n, pcaps[n]); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type srcFile struct {
	name string
	data []byte
}

func sourceFiles() ([]srcFile, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	// Tests and `go run` may start in a temp copy. Prefer the directory of this source when present.
	if exe, err := os.Executable(); err == nil {
		_ = exe
	}
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range ents {
		n := e.Name()
		if strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
			names = append(names, n)
		}
	}
	if len(names) == 0 {
		return nil, fmt.Errorf("no verifier sources in %s", dir)
	}
	sort.Strings(names)
	var out []srcFile
	for _, n := range names {
		b, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		out = append(out, srcFile{name: n, data: b})
	}
	return out, nil
}

func writeZips(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	// Pack from the source directory so go files are found even if -pack is invoked elsewhere.
	if err := os.Chdir(sourceDir(wd)); err != nil {
		return err
	}
	defer os.Chdir(wd)
	for _, b := range allBundles() {
		raw, err := zipBytes(b)
		if err != nil {
			return fmt.Errorf("%s: %w", b.Name, err)
		}
		if len(raw) >= 100*1024*1024 {
			return fmt.Errorf("%s is %d bytes", b.Name, len(raw))
		}
		if err := os.WriteFile(filepath.Join(dir, b.Name), raw, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func sourceDir(wd string) string {
	if _, err := os.Stat(filepath.Join(wd, "bundle.go")); err == nil {
		return wd
	}
	return wd
}

func readZipREADME(z *zip.Reader) (string, error) {
	f, err := findZip(z, "README.md")
	if err != nil {
		return "", err
	}
	r, err := f.Open()
	if err != nil {
		return "", err
	}
	defer r.Close()
	b, err := io.ReadAll(r)
	return string(b), err
}

func findZip(z *zip.Reader, name string) (*zip.File, error) {
	for _, f := range z.File {
		if f.Name == name {
			return f, nil
		}
	}
	return nil, fmt.Errorf("zip missing %s", name)
}

func criticalFromREADME(readme string) (string, error) {
	const a = "BEGIN CRITICAL\n"
	const b = "END CRITICAL"
	i := strings.Index(readme, a)
	j := strings.Index(readme, b)
	if i < 0 || j < 0 || j < i {
		return "", fmt.Errorf("critical block missing")
	}
	return readme[i+len(a) : j], nil
}

func bundle20() zipBundle {
	return zipBundle{
		Name:  "win-protocols-20.zip",
		Title: "Windows 可采集的 20 组新协议流量",
		Intro: strings.TrimSpace(`
这些是 #5013（分支 wip/optimize-protocol-parse，规划时的头 e3f689b2）协议语料里还没有综合会话的 20 个协议。对照的是该提交上全部 pcap/pcapng 文件名：socks5、finger、whois、gopher、dict、bjnp、zookeeper、mqtt-sn、turn、caldav、carddav、scgi、hessian、msgpack、clickhouse、bittorrent-dht、bits、consul、gearman、beanstalk 都没有对应捕获。已经有样本的协议（例如 srvloc、wsd、modbus、iec104）不放进这 20 组，避免把旧文件改名充数。

采集方式：在 Windows 上用本程序的协议状态机生成双方真正会发送的应用层字节，再封装成 Ethernet/IPv4/TCP 或 UDP 的 pcapng。地址用文档网段 192.0.2.10（客户端）和 192.0.2.20（服务端），TTL 128，TCP/UDP/IPv4 校验和按线路计算。每个 TCP 方向的载荷按 24 字节切开，所以 Wireshark 需要保持默认的 “Allow subdissector to reassemble TCP streams”。本机没有安装 tshark；PktMon 抓回环需要管理员权限，因此交付的是可重复的套接字字节封装，不是网卡驱动的原始 ETL。用 Wireshark 直接打开即可看到下面写的协议列和过滤器。
`),
		Caps: []capSpec{
			{File: "01-socks5.pcapng", Title: "01 SOCKS5", Gap: "语料文件名没有 socks5。已有的是 SOCKS4 生成包，没有方法协商、CONNECT 和被中继的 HTTP。", Shark: "显示过滤器 `socks`，或 `tcp.port == 1080`。协议列 SOCKS。先看到 Version 5、No authentication、Connect 127.0.0.1:80，重组后同一条流里出现 HTTP `GET /health` 和 `200 OK`。", Metrics: "版本 5、认证方法 0、命令 CONNECT、目的 127.0.0.1:80、中继 HTTP 状态与正文。", Unfinished: "没有用户名口令认证、BIND、UDP ASSOCIATE，也没有对中继字节做 SOCKS 之外的解密。", Build: buildSOCKS, Parse: parseSOCKS},
			{File: "02-finger.pcapng", Title: "02 Finger", Gap: "没有 finger 捕获。", Shark: "过滤器 `finger` 或 `tcp.port == 79`。协议列 FINGER。请求行 `/W alice`，应答里有 Login 和 Plan。", Metrics: "查询用户 alice，以及 Plan 那一行的完整句子。", Unfinished: "没有转发查询（user@host）和空用户列表。", Build: buildFinger, Parse: parseFinger},
			{File: "03-whois.pcapng", Title: "03 WHOIS", Gap: "没有 whois 捕获。仓库里的 whois.yak 是脚本，不是报文。", Shark: "过滤器 `whois` 或 `tcp.port == 43`。Follow TCP Stream 能看到查询行和 ReferralServer。", Metrics: "查询名、ReferralServer、Registry Domain ID。", Unfinished: "没有 RWhois、没有国际化和 referral 之后的第二次查询。", Build: buildWhois, Parse: parseWhois},
			{File: "04-gopher.pcapng", Title: "04 Gopher", Gap: "没有 gopher 捕获。", Shark: "过滤器 `gopher` 或 `tcp.port == 70`。两条 TCP：第一条请求空选择符，应答是菜单；第二条请求 `/readme`。", Metrics: "菜单项 Lab files 的选择符 /files，以及 /readme 的正文。", Unfinished: "没有 Gopher+、二进制文件类型或搜索选择符。", Build: buildGopher, Parse: parseGopher},
			{File: "05-dict.pcapng", Title: "05 DICT", Gap: "没有 dict 协议捕获。", Shark: "过滤器 `dict` 或 `tcp.port == 2628`。能看到 CLIENT、SHOW DB、DEFINE、QUIT 以及 220/110/150/221 应答。", Metrics: "客户端名、词库 lab-words、词条 protocol 和释义正文。", Unfinished: "没有 AUTH、STRATEGY、MATCH 和管道化的多词查询。", Build: buildDICT, Parse: parseDICT},
			{File: "06-bjnp.pcapng", Title: "06 BJNP", Gap: "没有 bjnp 捕获。", Shark: "过滤器 `bjnp`，UDP 8611。Wireshark 字段：bjnp.id=BJNP，bjnp.type 为 Printer Command/Response，bjnp.code 依次是 Discover(1)、Get Printer Identity(0x30)、Print Job Details(0x10)、Get Printer Status(0x20)。bjnp.seq_no 是 4 字节，bjnp.session_id 是 2 字节，后面才是 bjnp.payload_len。Info 列类似 “Printer Command: Discover”。", Metrics: "发现应答 LAB-PRINTER、身份 SN:5013、任务 job=7 的应答 accepted、状态 idle。", Unfinished: "没有真正的打印页数据、扫描任务和设备认证。", Build: buildBJNP, Parse: parseBJNP},
			{File: "07-zookeeper.pcapng", Title: "07 ZooKeeper", Gap: "没有 zookeeper 捕获。路线图里的 ZooKeeper 仍是 todo。", Shark: "过滤器 `zookeeper`，TCP 2181。先是 Connect（session 非 0），然后 Ping、GetChildren、GetData、CloseSession。", Metrics: "session id、`/` 的子节点名、`/lab` 的数据。", Unfinished: "没有 SASL、多包 jute 事务、ACL 和 watcher 事件。", Build: buildZK, Parse: parseZK},
			{File: "08-mqttsn.pcapng", Title: "08 MQTT-SN", Gap: "只有 MQTT，没有 mqtt-sn / mqttsn。", Shark: "过滤器 `mqttsn`，UDP 1883。依次是 SEARCHGW、GWINFO、CONNECT、CONNACK、REGISTER、REGACK、PUBLISH QoS 1、PUBACK、DISCONNECT。客户端 ID 在 CONNECT 里。", Metrics: "client id、主题名、topic id、QoS、PUBLISH 载荷。", Unfinished: "没有 WILL、睡眠流程、QoS 2 和预定义主题。", Build: buildMQTTSN, Parse: parseMQTTSN},
			{File: "09-turn.pcapng", Title: "09 TURN", Gap: "有 STUN 样本，没有 Allocate/权限/通道数据这一组 TURN。", Shark: "过滤器 `turn` 或 `stun`，UDP 3478。Allocate Success 里有 XOR-RELAYED-ADDRESS 和 LIFETIME；随后 CreatePermission、ChannelBind，再是 ChannelData（首个半字 0x4000–0x7FFF，不是 STUN magic），最后 Refresh。没有 MESSAGE-INTEGRITY，Wireshark 不会把它标成已认证。", Metrics: "中继地址、对端地址、通道号、lifetime、双向 ChannelData 载荷。", Unfinished: "没有长期凭证、IPv6 中继、Send/Data 指示和 TCP/TLS 传输。", Build: buildTURN, Parse: parseTURN},
			{File: "10-caldav.pcapng", Title: "10 CalDAV", Gap: "没有 caldav 捕获。HTTP 样本不能代替带 REPORT 和 text/calendar 的日历会话。", Shark: "过滤器 `http`，端口 8080。方法依次是 OPTIONS、PROPFIND、REPORT、PUT。207 的正文里有 displayname，calendar-data 里是 VEVENT。可用 `http.request.method == \"REPORT\"`。", Metrics: "displayname、VEVENT 的 UID 和 SUMMARY、PUT 的路径。", Unfinished: "没有调度（iTIP）、同步 token 和 CalDAV 权限。", Build: buildCalDAV, Parse: parseCalDAV},
			{File: "11-carddav.pcapng", Title: "11 CardDAV", Gap: "没有 carddav 捕获。", Shark: "过滤器 `http`，端口 8083。PROPFIND 与 addressbook-query REPORT，正文是 vCard。`http.content_type contains \"vcard\"` 能看到 PUT。", Metrics: "通讯录 displayname、vCard 的 UID、FN、TEL。", Unfinished: "没有多地址簿、照片和 vCard 4 的分组。", Build: buildCardDAV, Parse: parseCardDAV},
			{File: "12-scgi.pcapng", Title: "12 SCGI", Gap: "没有 scgi 捕获。", Shark: "Wireshark 没有单独的 SCGI 解析器，协议列保持 TCP。过滤器 `tcp.port == 4000`。Follow TCP Stream：净字符串（长度、冒号、以 NUL 分隔的 CONTENT_LENGTH / REQUEST_URI，逗号结束），应答以 `Status:` 开头而不是 `HTTP/1.1`。", Metrics: "两个请求的方法和 URI，GET 的应答正文，POST 的表单体。", Unfinished: "没有多路复用以外的头部续行，也没有当作 FastCGI 来解。", Build: buildSCGI, Parse: parseSCGI},
			{File: "13-hessian2.pcapng", Title: "13 Hessian 2", Gap: "没有 hessian 捕获。", Shark: "过滤器 `http`，端口 8082，Content-Type 是 application/x-hessian。`http.content_type contains \"hessian\"`。载荷以 `C`（call）或 `R`（reply）开头；方法名是 Hessian 2 的短字符串。", Metrics: "readPoint(7)=42，writePoint(7,42)=true。", Unfinished: "没有对象定义、引用和 Hessian 1 的信封。", Build: buildHessian, Parse: parseHessian},
			{File: "14-msgpack-rpc.pcapng", Title: "14 MessagePack-RPC", Gap: "没有 msgpack 捕获。", Shark: "没有默认解析器。过滤器 `tcp.port == 19850`。Follow TCP Stream 或把载荷当 MessagePack：请求是 4 元数组（0x94），type 0/1/2。第一条请求方法 lab.status。", Metrics: "status 结果、lab.set 的参数、通知里的 job。", Unfinished: "没有扩展类型、流式分帧和错误对象的结构化字段。", Build: buildMsgpack, Parse: parseMsgpack},
			{File: "15-clickhouse.pcapng", Title: "15 ClickHouse Native", Gap: "没有 clickhouse 捕获，路线图该项仍是 todo。", Shark: "TCP 9000。Wireshark 4.x 的过滤器是 `clickhouse`。若协议列仍是 TCP，对该端口 Decode As → ClickHouse。客户端 Hello 的包类型是 varint 0，后面是客户端名、主次版本、revision、库名、用户、空密码；服务器 Hello 带回 revision 54401、时区 UTC 和显示名；随后各有一个类型 4 的 Ping/Pong。", Metrics: "客户端名、用户、库、服务器名、revision、时区，以及 ping 得到 pong。", Unfinished: "没有 Query/Data/Block、压缩和带口令的认证。Hello+Ping 不算查询完成。", Build: buildClickHouse, Parse: parseClickHouse},
			{File: "16-bittorrent-dht.pcapng", Title: "16 BitTorrent DHT", Gap: "有 bittorrent 文件名，没有 bt-dht / dht。主协议捕获不是 KRPC。", Shark: "过滤器 `bt-dht`，UDP 6881。四组查询和应答：ping、find_node、get_peers、announce_peer，bencode 字典里 y=q 或 y=r。", Metrics: "本节点 id、find_node 的 target、info_hash、announce 的 token 和端口。", Unfinished: "没有 sample_infohashes、IPv6 nodes6 和令牌校验。", Build: buildDHT, Parse: parseDHT},
			{File: "17-ms-bits.pcapng", Title: "17 MS-BITS", Gap: "没有 bits 捕获。", Shark: "过滤器 `http`，端口 8081。四个 POST 的 BITS-Packet-Type 分别是 Create-Session、Ping、Fragment、Close-Session。可用 `http contains \"BITS-Packet-Type\"`。应答里有 BITS-Session-Id。", Metrics: "会话号、四个包类型、Content-Range 和 16 字节分片。", Unfinished: "没有多段续传、取消和 BITS 的服务器证书认证。", Build: buildBITS, Parse: parseBITS},
			{File: "18-consul.pcapng", Title: "18 Consul HTTP API", Gap: "没有 consul 捕获。etcd 的 HTTP 样本不是 Consul 的 /v1 API。", Shark: "过滤器 `http`，端口 8500。`http.request.uri contains \"/v1/\"` 能看到 leader、catalog/services 和 kv。正文是 JSON。", Metrics: "leader 地址、服务名和标签、KV 键和值。", Unfinished: "没有 Consul 的 RPC/msgpack、gossip 和 ACL token。", Build: buildConsul, Parse: parseConsul},
			{File: "19-gearman.pcapng", Title: "19 Gearman", Gap: "没有 gearman 捕获。", Shark: "过滤器 `gearman`，TCP 4730。两条连接：worker 发 CAN_DO、GRAB_JOB、PRE_SLEEP、WORK_COMPLETE；client 发 SUBMIT_JOB。Magic 是 `\\0REQ` / `\\0RES`。Info 里能看到函数名 lab.reverse。", Metrics: "函数名、job handle、workload 和 worker 返回的结果。", Unfinished: "没有后台任务、唯一 ID 约束和异常包。", Build: buildGearman, Parse: parseGearman},
			{File: "20-beanstalkd.pcapng", Title: "20 Beanstalkd", Gap: "没有 beanstalk 捕获。", Shark: "没有专用解析器。过滤器 `tcp.port == 11300`。Follow TCP Stream 看到 `put 1024 0 60 12`、正文、`INSERTED 7`、`RESERVED 7 12`、`delete 7`、`DELETED`。", Metrics: "job id、优先级、TTR 和正文，以及删除成功。", Unfinished: "没有 bury、kick、tube 和超时重发。", Build: buildBeanstalk, Parse: parseBeanstalk},
		},
	}
}

func bundleICS() zipBundle {
	return zipBundle{
		Name:  "topic-ics.zip",
		Title: "工控流量专题",
		Intro: strings.TrimSpace(`
这是新做的工控实验会话，不是把 #5013 里已有的 modbus-read-holding.pcap（645 字节，只有读保持寄存器）、ndpi 里的 bacnet，或已有的 enip/cip 文件重新打包。

要算“解完”，必须从 pcapng 里读出命令、点号和数值：Modbus 的功能码、地址、寄存器/线圈/异常码；BACnet 的设备实例、对象、present-value；EtherNet/IP 的会话号和 CIP 标签值。只认出 TCP 502 或 UDP 47808 不算结束。用 Wireshark 打开时应按下面每个文件的过滤器能看到这些字段。
`),
		Caps: []capSpec{
			{File: "ics-01-modbus.pcapng", Title: "Modbus TCP", Gap: "已有样本只覆盖读保持寄存器。这里补了写单个寄存器、读线圈和非法地址异常。", Shark: "过滤器 `modbus` 或 `mbtcp`，TCP 502。事务 1 是功能码 3，事务 2 是功能码 6，事务 3 是功能码 1，事务 4 的功能码是 0x83（异常）。", Metrics: "单元、功能码、地址、寄存器值、线圈、异常码。", Unfinished: "没有文件记录、设备标识（0x2B）和串口 RTU。", Build: buildModbus, Parse: parseModbus},
			{File: "ics-02-bacnet.pcapng", Title: "BACnet/IP", Gap: "已有 bacnet 文件名不是这一组 Who-Is / I-Am / ReadProperty / WriteProperty。", Shark: "过滤器 `bacnet` 或 `bacapp`，UDP 47808。BVLC 0x81。Who-Is 限定设备 5013，I-Am 带 device 对象，随后是 analog-value,7 的 present-value 读和写。", Metrics: "设备实例、对象标识、读出的 present-value、写入的 present-value。", Unfinished: "没有 COV、分段确认和 BACnet/SC。", Build: buildBACnet, Parse: parseBACnet},
			{File: "ics-03-enip-cip.pcapng", Title: "EtherNet/IP CIP", Gap: "语料里虽有 enip/cip 文件，但不是会话 0x5013、标签 Speed 的这次读 1500 / 写 1510。本文件的 SHA-256 与那些文件不同。", Shark: "过滤器 `enip` 和 `cip`，TCP 44818。RegisterSession 命令 0x0065，SendRRData 命令 0x006F。CIP 服务 0x4C 读标签、0x4D 写标签，路径是 ANSI 符号段 0x91，类型 DINT（0x00C4）。", Metrics: "会话号、标签名、读出的 DINT、写入的 DINT。", Unfinished: "没有连接管理器的隐式 I/O、PCCC 和加密 CIP Security。", Build: buildCIP, Parse: parseCIP},
		},
	}
}

func bundlePower() zipBundle {
	return zipBundle{
		Name:  "topic-power.zip",
		Title: "电力行业专题流量",
		Intro: strings.TrimSpace(`
电力专题是新的实验帧，不复制 #5013 里 718 字节的 iec104-startdt-interrogation.pcap、621 字节的 goose-dataset.pcap，也不复制 c37118-cmd.pcap。

解完的标准是读出报文里真实存在的量：IEC 104 的 ASDU 类型、公共地址、IOA 和值；GOOSE 的 goID、gocbRef、数据集和状态变化；C37.118 的 IDCODE 与相量分量。只看到 2404 端口或 0x88B8 以太类型不算结束。用 Wireshark 打开时，下面写的过滤器必须能定位到这些字段。
`),
		Caps: []capSpec{
			{File: "power-01-iec104.pcapng", Title: "IEC 60870-5-104", Gap: "已有样本没有同时给出 IOA 2001 的单点与 IOA 1001 的短浮点，也没有激活终止。", Shark: "过滤器 `iec104` 或 `104apci`，TCP 2404。U 帧 STARTDT act/con，然后 I 帧：C_IC_NA_1（100）总召唤、M_SP_NA_1（1）、M_ME_NC_1（13）、再次 C_IC_NA_1 且 COT=10，最后 S 帧确认。公共地址和 IOA 按 104 常见的 2 字节 COT、2 字节 CA、3 字节 IOA，小端。", Metrics: "CA、单点 IOA 与 SPI、测量 IOA 与浮点、总召唤激活终止。", Unfinished: "没有对时、遥控选择/执行和文件传输。", Build: buildIEC104, Parse: parseIEC104},
			{File: "power-02-goose.pcapng", Title: "IEC 61850 GOOSE", Gap: "已有 goose-dataset.pcap 不是 goID LAB-GOOSE-1 从 stNum 5 到 6 的状态变化。", Shark: "以太类型 0x88b8，目的 MAC 01:0c:cd:01:00:01。过滤器 `goose`。APPID 0x3000。两帧的 stNum/布尔量不同。gocbRef、datSet、goID 在 goosePDU 的 context 标记 0、2、3。", Metrics: "gocbRef、goID、datSet、两帧的 stNum 和布尔状态。", Unfinished: "没有 GOOSE 订阅超时、密钥和采样值 SV。", Build: buildGOOSE, Parse: parseGOOSE},
			{File: "power-03-c37118.pcapng", Title: "IEEE C37.118 同步相量", Gap: "已有 c37118-cmd.pcap 是命令帧场景，没有这一帧里带名字的浮点相量。", Shark: "过滤器 `synphasor`，TCP 4712。先是命令帧（启动传输，CMD=2），再是 config2（站名 LAB-PMU，相量名 V1，50 Hz），然后数据帧。CRC 是 init 0xFFFF、多项式 0x1021、无最终异或，覆盖除 CHK 外的整帧。若 Wireshark 的 CRC 专家信息与实现差一个变体，字段仍应按 IDCODE 和相量解析；本验证以同一算法重算 CRC，不符则失败。", Metrics: "IDCODE、相量名、直角坐标实部/虚部、频率偏差、额定频率。", Unfinished: "没有 config3、多 PMU 和分相模拟量。", Build: buildC37, Parse: parseC37},
		},
	}
}

func bundleBlue() zipBundle {
	return zipBundle{
		Name:  "topic-blueteam.zip",
		Title: "蓝队应急流量处置（含挖矿会话）",
		Intro: strings.TrimSpace(`
这是实验用的应急分析样本，不是可用的矿工程序，也没有连接任何真实矿池。Stratum 只在 192.0.2.20:3333 上交换 JSON 行；域名 lab-pool.invalid 和 update.lab-invalid.test 都是保留在文档里的名字。授权参数里的口令是单个字母 x，验证输出不打印它。

处置这些流量时，解完的标准是从报文里抽出矿池、矿工、任务号，以及实验室信标的 Host、URI、User-Agent。不包含恶意家族归因、磁盘取证或遏制步骤；那些不算本包的完成条件。Wireshark 里用下面的过滤器和 Follow TCP Stream 能看到同一批字段。
`),
		Caps: []capSpec{
			{File: "blue-01-stratum.pcapng", Title: "Stratum 矿池会话", Gap: "语料里没有 stratum 或 mining 捕获。", Shark: "先是 DNS `lab-pool.invalid` A 记录，过滤器 `dns.qry.name == \"lab-pool.invalid\"`。随后 TCP 3333，Follow TCP Stream 是一行一个 JSON：mining.subscribe、mining.authorize、mining.notify、mining.submit。Wireshark 通常把 3333 显示成 TCP；用 `tcp.port == 3333 && tcp contains \"mining.notify\"` 能定位任务。", Metrics: "池域名、A 记录、端口、矿机 agent、矿工名、notify 的 job id。", Unfinished: "没有真实 share 校验、TLS 矿池和矿机进程树。", Build: buildStratum, Parse: parseStratum},
			{File: "blue-02-beacon.pcapng", Title: "实验室 HTTP 信标", Gap: "不是已有 HTTP 语料的改名；Host 和 URI 是这组 IOC。", Shark: "过滤器 `http`，端口 80。`http.host == \"update.lab-invalid.test\"`，请求行含 `/beacon`，User-Agent 为 lab-beacon/5013。", Metrics: "Host、URI、User-Agent、应答正文里的 job 标记。", Unfinished: "没有 JA3、加密信标和主机侧进程关联。", Build: buildBeacon, Parse: parseBeacon},
		},
	}
}

func bundleFTP() zipBundle {
	return zipBundle{
		Name:  "ctf-ftp-reassembly.zip",
		Title: "CTF 专题一：FTP 被动模式重组",
		Intro: strings.TrimSpace(`
原创题目，CC0-1.0。#5013 里已经有 FTP 样本，本题不是那份样本，也不从外部 CTF 包拷贝。旗标拆在数据连接的多个 TCP 段里；横幅里的 flag{not-the-ftp-flag} 是干扰项。

完成标准：从 PASV 给出的数据端口把 RETR 的文件重组出来，打印真正的 flag。只在控制连接里搜到 flag{ 不算。Wireshark 里控制连接和数据连接要分开看。
`),
		Caps: []capSpec{
			{File: "ctf-01-ftp.pcapng", Title: "FTP 重组题", Gap: "原创挑战流量，不是已有 FTP 夹具。", Shark: "过滤器 `ftp` 看控制连接（21），`ftp.request.command == \"RETR\"`。227 应答给出数据端口。数据连接本身是裸文件字节，协议列是 TCP；要对 PASV 端口做 Follow TCP Stream 才能看到完整旗标。", Metrics: "文件名、数据端口、重组后的 flag。", Unfinished: "没有 TLS（FTPS）和多文件。干扰旗标不是答案。", Build: buildFTP, Parse: parseFTP},
		},
	}
}

func bundleDNS() zipBundle {
	return zipBundle{
		Name:  "ctf-dns-labels.zip",
		Title: "CTF 专题二：DNS TXT 分段",
		Intro: strings.TrimSpace(`
原创题目，CC0-1.0。不是第三方 CTF 流量包。旗标拆在三个 TXT 回答里，decoy.flag.lab-invalid 的 flag{not-this} 要丢掉。完整旗标在文件里不是连续字节。

完成标准：按 c1、c2、c3 的查询名顺序拼接 TXT，得到 flag。对整份 pcap 搜 flag{ 会同时看到干扰项，那不算解出来。Wireshark 的 DNS 解析树里，三条 TXT 是分开的资源记录。
`),
		Caps: []capSpec{
			{File: "ctf-01-dns.pcapng", Title: "DNS TXT 拼接题", Gap: "原创挑战；已有 DNS 样本不含这四条 TXT。", Shark: "过滤器 `dns`，UDP 53。`dns.qry.name contains \"flag.lab-invalid\"`。每个回答是一条 TXT。c1/c2/c3 才参与拼接，decoy 只作为负例出现在包里。", Metrics: "三条查询名、被忽略的 TXT、拼接后的 flag。", Unfinished: "没有 DNSSEC 校验和 TCP DNS。干扰 TXT 不是答案。", Build: buildDNSCTF, Parse: parseDNSCTF},
		},
	}
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-pack" {
		dir := "zips"
		if len(os.Args) > 2 {
			dir = os.Args[2]
		}
		if err := writeZips(dir); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	dir := "captures"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	out, err := verifyDir(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}
