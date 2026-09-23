# Windows 可采集的 20 组新协议流量

SPDX-License-Identifier: CC0-1.0

来源：本目录里的 pcapng 全部是为 yaklang PR #5013 新写的 Windows 实验流量，不是从 #5013 已有样本改名，也不是第三方 CTF/工控仓库里的抓包。作者放弃这些实验字节的版权（CC0-1.0），可以随仓库再分发。

这些是 #5013（分支 wip/optimize-protocol-parse，规划时的头 e3f689b2）协议语料里还没有综合会话的 20 个协议。对照的是该提交上全部 pcap/pcapng 文件名：socks5、finger、whois、gopher、dict、bjnp、zookeeper、mqtt-sn、turn、caldav、carddav、scgi、hessian、msgpack、clickhouse、bittorrent-dht、bits、consul、gearman、beanstalk 都没有对应捕获。已经有样本的协议（例如 srvloc、wsd、modbus、iec104）不放进这 20 组，避免把旧文件改名充数。

采集方式：在 Windows 上用本程序的协议状态机生成双方真正会发送的应用层字节，再封装成 Ethernet/IPv4/TCP 或 UDP 的 pcapng。地址用文档网段 192.0.2.10（客户端）和 192.0.2.20（服务端），TTL 128，TCP/UDP/IPv4 校验和按线路计算。每个 TCP 方向的载荷按 24 字节切开，所以 Wireshark 需要保持默认的 “Allow subdissector to reassemble TCP streams”。本机没有安装 tshark；PktMon 抓回环需要管理员权限，因此交付的是可重复的套接字字节封装，不是网卡驱动的原始 ETL。用 Wireshark 直接打开即可看到下面写的协议列和过滤器。

## 怎么验证

在解压后的目录执行 `go run .`（只依赖 Go 标准库）。程序读取 `captures/*.pcapng`，按协议把 TCP/UDP 重组后再解字段，把结果打印到标准输出。退出码 0 且输出与下面的 CRITICAL 块逐字相同，才算这一组验证结束。

下面任何一种都不算结束：只在 Wireshark 里看到有包、只对上了端口、README 里写了答案但程序没有从 pcapng 读出来、或者把 pcapng 删掉/截断/改一个应用层字节后程序仍然打印同一份 CRITICAL。

BEGIN CRITICAL
file=01-socks5.pcapng
protocol=socks5
version=5
auth_method=0
command=CONNECT
dst=127.0.0.1:80
relay_status=200
relay_body=ok

file=02-finger.pcapng
protocol=finger
user=alice
plan=Windows lab finger fixture for yaklang PR 5013.

file=03-whois.pcapng
protocol=whois
query=example.invalid
referral=whois.lab-invalid:43
id=LAB-5013-EXAMPLE

file=04-gopher.pcapng
protocol=gopher
menu_item=Lab files
menu_selector=/files
selector=/readme
body=yaklang lab gopher item

file=05-dict.pcapng
protocol=dict
client=lab-win/1.0
database=lab-words
word=protocol
definition=A lab definition used only as a parse fixture.

file=06-bjnp.pcapng
protocol=bjnp
discover=LAB-PRINTER
identity=SN:5013
job=job=7
job_reply=accepted
status=idle

file=07-zookeeper.pcapng
protocol=zookeeper
session_id=0x5013
children=lab,znode
path=/lab
value=winlab

file=08-mqttsn.pcapng
protocol=mqtt-sn
client_id=winlab
topic=lab/temp
topic_id=1
qos=1
payload=23.5

file=09-turn.pcapng
protocol=turn
relayed=192.0.2.55:50000
peer=198.51.100.8:3478
channel=0x4001
lifetime=600
payload=lab-turn/peer-ok

file=10-caldav.pcapng
protocol=caldav
displayname=Lab Calendar
uid=lab-evt-5013
summary=WinLab review
put=/cal/alice/default/lab-evt-5013.ics

file=11-carddav.pcapng
protocol=carddav
displayname=Lab Contacts
uid=lab-card-5013
fn=Ada Lab
tel=+1-555-0100

file=12-scgi.pcapng
protocol=scgi
request1=GET /lab/status
body1=scgi-ok-5013
request2=POST /lab/point
body2=point=7&value=42

file=13-hessian2.pcapng
protocol=hessian2
method1=readPoint
arg1=7
result1=42
method2=writePoint
arg2=7,42
result2=true

file=14-msgpack-rpc.pcapng
protocol=msgpack-rpc
status_result=ok-5013
set_method=lab.set
set_point=point
set_value=42
event_job=7f3a

file=15-clickhouse.pcapng
protocol=clickhouse
client=winlab-ch
user=winlab
database=lab
server=lab-clickhouse
revision=54401
timezone=UTC
ping=pong

file=16-bittorrent-dht.pcapng
protocol=bittorrent-dht
ping_id=winlab-node-5013!!!!
find_target=target-node-5013!!!!
info_hash=lab-infohash-5013!!!
announce_token=lab
announce_port=6881
nodes_len=26
node_id=lab-peer-node-5013!!
node_ip=198.51.100.9
node_port=6881

file=17-ms-bits.pcapng
protocol=ms-bits
session={LAB-SESSION-5013}
packets=Create-Session,Ping,Fragment,Close-Session
range=bytes 0-15/16
fragment=lab-bits-payload

file=18-consul.pcapng
protocol=consul
leader=192.0.2.20:8300
service=lab-win
tags=win,pcap
kv=lab/point
value=42

file=19-gearman.pcapng
protocol=gearman
function=lab.reverse
handle=H:lab:7
workload=winlab
result=balniw

file=20-beanstalkd.pcapng
protocol=beanstalkd
job_id=7
priority=1024
ttr=60
body=hello-winlab
deleted=true
END CRITICAL

## 01 SOCKS5

- 文件：`captures/01-socks5.pcapng`
- SHA-256：`c0681aa39ab129e7592428f9401e11d3ccaf54e135f171a19b4ff38b5fa77f20`
- 相对 #5013 的缺口：语料文件名没有 socks5。已有的是 SOCKS4 生成包，没有方法协商、CONNECT 和被中继的 HTTP。
- Wireshark / tshark：显示过滤器 `socks`，或 `tcp.port == 1080`。协议列 SOCKS。先看到 Version 5、No authentication、Connect 127.0.0.1:80，重组后同一条流里出现 HTTP `GET /health` 和 `200 OK`。
- 核心指标：版本 5、认证方法 0、命令 CONNECT、目的 127.0.0.1:80、中继 HTTP 状态与正文。
- 验证标准：`go run .` 打印的对应 `file=01-socks5.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有用户名口令认证、BIND、UDP ASSOCIATE，也没有对中继字节做 SOCKS 之外的解密。

## 02 Finger

- 文件：`captures/02-finger.pcapng`
- SHA-256：`21b0fe35842ccde164fa92afa1715896cf8aabc086f1ea811eca50d7712e879c`
- 相对 #5013 的缺口：没有 finger 捕获。
- Wireshark / tshark：过滤器 `finger` 或 `tcp.port == 79`。协议列 FINGER。请求行 `/W alice`，应答里有 Login 和 Plan。
- 核心指标：查询用户 alice，以及 Plan 那一行的完整句子。
- 验证标准：`go run .` 打印的对应 `file=02-finger.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有转发查询（user@host）和空用户列表。

## 03 WHOIS

- 文件：`captures/03-whois.pcapng`
- SHA-256：`02d3d5111057e4f2370b64ffebaea8124dd7a185019b53e4602a1bff2a384aed`
- 相对 #5013 的缺口：没有 whois 捕获。仓库里的 whois.yak 是脚本，不是报文。
- Wireshark / tshark：过滤器 `whois` 或 `tcp.port == 43`。Follow TCP Stream 能看到查询行和 ReferralServer。
- 核心指标：查询名、ReferralServer、Registry Domain ID。
- 验证标准：`go run .` 打印的对应 `file=03-whois.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 RWhois、没有国际化和 referral 之后的第二次查询。

## 04 Gopher

- 文件：`captures/04-gopher.pcapng`
- SHA-256：`fdb296c2a33d6994913cc3c5fe5f438f6a199e28b8bb14bb9692206d4e06c67d`
- 相对 #5013 的缺口：没有 gopher 捕获。
- Wireshark / tshark：过滤器 `gopher` 或 `tcp.port == 70`。两条 TCP：第一条请求空选择符，应答是菜单；第二条请求 `/readme`。
- 核心指标：菜单项 Lab files 的选择符 /files，以及 /readme 的正文。
- 验证标准：`go run .` 打印的对应 `file=04-gopher.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 Gopher+、二进制文件类型或搜索选择符。

## 05 DICT

- 文件：`captures/05-dict.pcapng`
- SHA-256：`f8167020c117bf59866681a1586df5cb11c2bf39e231730abf0ab218970db939`
- 相对 #5013 的缺口：没有 dict 协议捕获。
- Wireshark / tshark：过滤器 `dict` 或 `tcp.port == 2628`。能看到 CLIENT、SHOW DB、DEFINE、QUIT 以及 220/110/150/221 应答。
- 核心指标：客户端名、词库 lab-words、词条 protocol 和释义正文。
- 验证标准：`go run .` 打印的对应 `file=05-dict.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 AUTH、STRATEGY、MATCH 和管道化的多词查询。

## 06 BJNP

- 文件：`captures/06-bjnp.pcapng`
- SHA-256：`ad41277dc98207000c1f8d2cf688ec854312649faffb33d870578622f170cff7`
- 相对 #5013 的缺口：没有 bjnp 捕获。
- Wireshark / tshark：过滤器 `bjnp`，UDP 8611。Wireshark 字段：bjnp.id=BJNP，bjnp.type 为 Printer Command/Response，bjnp.code 依次是 Discover(1)、Get Printer Identity(0x30)、Print Job Details(0x10)、Get Printer Status(0x20)。bjnp.seq_no 是 4 字节，bjnp.session_id 是 2 字节，后面才是 bjnp.payload_len。Info 列类似 “Printer Command: Discover”。
- 核心指标：发现应答 LAB-PRINTER、身份 SN:5013、任务 job=7 的应答 accepted、状态 idle。
- 验证标准：`go run .` 打印的对应 `file=06-bjnp.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有真正的打印页数据、扫描任务和设备认证。

## 07 ZooKeeper

- 文件：`captures/07-zookeeper.pcapng`
- SHA-256：`e2f09c6315c71e6335d2cd884d3d5919c74177b6626a95a3a0c472b0f9857fd8`
- 相对 #5013 的缺口：没有 zookeeper 捕获。路线图里的 ZooKeeper 仍是 todo。
- Wireshark / tshark：过滤器 `zookeeper`，TCP 2181。先是 Connect（session 非 0），然后 Ping、GetChildren、GetData、CloseSession。
- 核心指标：session id、`/` 的子节点名、`/lab` 的数据。
- 验证标准：`go run .` 打印的对应 `file=07-zookeeper.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 SASL、多包 jute 事务、ACL 和 watcher 事件。

## 08 MQTT-SN

- 文件：`captures/08-mqttsn.pcapng`
- SHA-256：`d57651b77ca675deb298bc9f55337f64ef209a02eac87bf6c98bde7844a316ca`
- 相对 #5013 的缺口：只有 MQTT，没有 mqtt-sn / mqttsn。
- Wireshark / tshark：过滤器 `mqttsn`，UDP 1883。依次是 SEARCHGW、GWINFO、CONNECT、CONNACK、REGISTER、REGACK、PUBLISH QoS 1、PUBACK、DISCONNECT。客户端 ID 在 CONNECT 里。
- 核心指标：client id、主题名、topic id、QoS、PUBLISH 载荷。
- 验证标准：`go run .` 打印的对应 `file=08-mqttsn.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 WILL、睡眠流程、QoS 2 和预定义主题。

## 09 TURN

- 文件：`captures/09-turn.pcapng`
- SHA-256：`57a73c29c715c42b0663ad7e8e78b010add6f89df5914e93bfd43f762f6814bb`
- 相对 #5013 的缺口：有 STUN 样本，没有 Allocate/权限/通道数据这一组 TURN。
- Wireshark / tshark：过滤器 `turn` 或 `stun`，UDP 3478。Allocate Success 里有 XOR-RELAYED-ADDRESS 和 LIFETIME；随后 CreatePermission、ChannelBind，再是 ChannelData（首个半字 0x4000–0x7FFF，不是 STUN magic），最后 Refresh。没有 MESSAGE-INTEGRITY，Wireshark 不会把它标成已认证。
- 核心指标：中继地址、对端地址、通道号、lifetime、双向 ChannelData 载荷。
- 验证标准：`go run .` 打印的对应 `file=09-turn.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有长期凭证、IPv6 中继、Send/Data 指示和 TCP/TLS 传输。

## 10 CalDAV

- 文件：`captures/10-caldav.pcapng`
- SHA-256：`823ecccb82aa53b45a7b1d2b2157eb787d635dfdf8a7a594c21e52f9e959894d`
- 相对 #5013 的缺口：没有 caldav 捕获。HTTP 样本不能代替带 REPORT 和 text/calendar 的日历会话。
- Wireshark / tshark：过滤器 `http`，端口 8080。方法依次是 OPTIONS、PROPFIND、REPORT、PUT。207 的正文里有 displayname，calendar-data 里是 VEVENT。可用 `http.request.method == "REPORT"`。
- 核心指标：displayname、VEVENT 的 UID 和 SUMMARY、PUT 的路径。
- 验证标准：`go run .` 打印的对应 `file=10-caldav.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有调度（iTIP）、同步 token 和 CalDAV 权限。

## 11 CardDAV

- 文件：`captures/11-carddav.pcapng`
- SHA-256：`b5b655c93904baddae5ea18df4c8fca33741c4f7efb546d4de08ae0486de3976`
- 相对 #5013 的缺口：没有 carddav 捕获。
- Wireshark / tshark：过滤器 `http`，端口 8083。PROPFIND 与 addressbook-query REPORT，正文是 vCard。`http.content_type contains "vcard"` 能看到 PUT。
- 核心指标：通讯录 displayname、vCard 的 UID、FN、TEL。
- 验证标准：`go run .` 打印的对应 `file=11-carddav.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有多地址簿、照片和 vCard 4 的分组。

## 12 SCGI

- 文件：`captures/12-scgi.pcapng`
- SHA-256：`bb760229e822fe2fa8f169a0a00a993653b3865c73004d59bb5da07f2a3e6a4f`
- 相对 #5013 的缺口：没有 scgi 捕获。
- Wireshark / tshark：Wireshark 没有单独的 SCGI 解析器，协议列保持 TCP。过滤器 `tcp.port == 4000`。Follow TCP Stream：净字符串（长度、冒号、以 NUL 分隔的 CONTENT_LENGTH / REQUEST_URI，逗号结束），应答以 `Status:` 开头而不是 `HTTP/1.1`。
- 核心指标：两个请求的方法和 URI，GET 的应答正文，POST 的表单体。
- 验证标准：`go run .` 打印的对应 `file=12-scgi.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有多路复用以外的头部续行，也没有当作 FastCGI 来解。

## 13 Hessian 2

- 文件：`captures/13-hessian2.pcapng`
- SHA-256：`d08dae1640619ef065d981fb6b482d628795327420ce67dacb1f94f757397c2f`
- 相对 #5013 的缺口：没有 hessian 捕获。
- Wireshark / tshark：过滤器 `http`，端口 8082，Content-Type 是 application/x-hessian。`http.content_type contains "hessian"`。载荷以 `C`（call）或 `R`（reply）开头；方法名是 Hessian 2 的短字符串。
- 核心指标：readPoint(7)=42，writePoint(7,42)=true。
- 验证标准：`go run .` 打印的对应 `file=13-hessian2.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有对象定义、引用和 Hessian 1 的信封。

## 14 MessagePack-RPC

- 文件：`captures/14-msgpack-rpc.pcapng`
- SHA-256：`a54caafde804a66ab0658b9438d54ebf8aed3dad55f7d7b6fd3b35c2d18d1765`
- 相对 #5013 的缺口：没有 msgpack 捕获。
- Wireshark / tshark：没有默认解析器。过滤器 `tcp.port == 19850`。Follow TCP Stream 或把载荷当 MessagePack：请求是 4 元数组（0x94），type 0/1/2。第一条请求方法 lab.status。
- 核心指标：status 结果、lab.set 的参数、通知里的 job。
- 验证标准：`go run .` 打印的对应 `file=14-msgpack-rpc.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有扩展类型、流式分帧和错误对象的结构化字段。

## 15 ClickHouse Native

- 文件：`captures/15-clickhouse.pcapng`
- SHA-256：`b6a8a531159e55934ec0a0e2e9730debb5b6b9f2cea98c1a8fbe5fa112bde409`
- 相对 #5013 的缺口：没有 clickhouse 捕获，路线图该项仍是 todo。
- Wireshark / tshark：TCP 9000。Wireshark 4.x 的过滤器是 `clickhouse`。若协议列仍是 TCP，对该端口 Decode As → ClickHouse。客户端 Hello 的包类型是 varint 0，后面是客户端名、主次版本、revision、库名、用户、空密码；服务器 Hello 带回 revision 54401、时区 UTC 和显示名；随后各有一个类型 4 的 Ping/Pong。
- 核心指标：客户端名、用户、库、服务器名、revision、时区，以及 ping 得到 pong。
- 验证标准：`go run .` 打印的对应 `file=15-clickhouse.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 Query/Data/Block、压缩和带口令的认证。Hello+Ping 不算查询完成。

## 16 BitTorrent DHT

- 文件：`captures/16-bittorrent-dht.pcapng`
- SHA-256：`73d6d75249381dfc0f527e3920fbbc1544a7099c0a1e1aaeeed77da83cd984c1`
- 相对 #5013 的缺口：有 bittorrent 文件名，没有 bt-dht / dht。主协议捕获不是 KRPC。
- Wireshark / tshark：过滤器 `bt-dht`，UDP 6881。四组查询和应答：ping、find_node、get_peers、announce_peer，bencode 字典里 y=q 或 y=r。
- 核心指标：本节点 id、find_node 的 target、info_hash、announce 的 token 和端口。
- 验证标准：`go run .` 打印的对应 `file=16-bittorrent-dht.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 sample_infohashes、IPv6 nodes6 和令牌校验。

## 17 MS-BITS

- 文件：`captures/17-ms-bits.pcapng`
- SHA-256：`085a22413da7332c09cde747d57f914d506b6426e1ecf940b00d5018abf3e759`
- 相对 #5013 的缺口：没有 bits 捕获。
- Wireshark / tshark：过滤器 `http`，端口 8081。四个 POST 的 BITS-Packet-Type 分别是 Create-Session、Ping、Fragment、Close-Session。可用 `http contains "BITS-Packet-Type"`。应答里有 BITS-Session-Id。
- 核心指标：会话号、四个包类型、Content-Range 和 16 字节分片。
- 验证标准：`go run .` 打印的对应 `file=17-ms-bits.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有多段续传、取消和 BITS 的服务器证书认证。

## 18 Consul HTTP API

- 文件：`captures/18-consul.pcapng`
- SHA-256：`17b25d86aafb2b968e2bf846bcec79f5125c6054ac40474bfd35e8d0f7c3c8b8`
- 相对 #5013 的缺口：没有 consul 捕获。etcd 的 HTTP 样本不是 Consul 的 /v1 API。
- Wireshark / tshark：过滤器 `http`，端口 8500。`http.request.uri contains "/v1/"` 能看到 leader、catalog/services 和 kv。正文是 JSON。
- 核心指标：leader 地址、服务名和标签、KV 键和值。
- 验证标准：`go run .` 打印的对应 `file=18-consul.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 Consul 的 RPC/msgpack、gossip 和 ACL token。

## 19 Gearman

- 文件：`captures/19-gearman.pcapng`
- SHA-256：`26971ec97e0a280771fb46ea29180a703a42946e3e36832d4d1642d5788986bd`
- 相对 #5013 的缺口：没有 gearman 捕获。
- Wireshark / tshark：过滤器 `gearman`，TCP 4730。两条连接：worker 发 CAN_DO、GRAB_JOB、PRE_SLEEP、WORK_COMPLETE；client 发 SUBMIT_JOB。Magic 是 `\0REQ` / `\0RES`。Info 里能看到函数名 lab.reverse。
- 核心指标：函数名、job handle、workload 和 worker 返回的结果。
- 验证标准：`go run .` 打印的对应 `file=19-gearman.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有后台任务、唯一 ID 约束和异常包。

## 20 Beanstalkd

- 文件：`captures/20-beanstalkd.pcapng`
- SHA-256：`99d7beeb52ba15c2c7112b58036c20d1392606e5eac3c412d6c97613d786766d`
- 相对 #5013 的缺口：没有 beanstalk 捕获。
- Wireshark / tshark：没有专用解析器。过滤器 `tcp.port == 11300`。Follow TCP Stream 看到 `put 1024 0 60 12`、正文、`INSERTED 7`、`RESERVED 7 12`、`delete 7`、`DELETED`。
- 核心指标：job id、优先级、TTR 和正文，以及删除成功。
- 验证标准：`go run .` 打印的对应 `file=20-beanstalkd.pcapng` 段必须包含上面 CRITICAL 里该文件的每一行，并且这些值来自本 pcapng 的字段，而不是来自本段文字。
- 还不算做完：没有 bury、kick、tube 和超时重发。

