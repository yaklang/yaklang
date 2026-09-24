# netstackvm 使用手册

`netstackvm` 在进程内运行独立的 gVisor 协议栈。新代码优先使用 `NewNetworkVM`：它把网络模式、地址、DNS、出站策略和生命周期放在同一个入口中。原有 channel VM 和分步 TCP 探测接口仍然保留。

## 1. 选择网络模式

- **`NetworkShared`：共享主机连接。** VM 使用私有 IPv4 地址，通过用户态网关调用主机的 TCP/UDP 套接字出站。不创建系统网卡、不修改主机路由，适合 HTTP 客户端和普通应用连接。
- **`NetworkBridged`：独立二层身份。** VM 使用自己的 MAC，通过调用方提供的链路进入网络，可以配置静态 IPv4，也可以运行 DHCP。适合已有 TAP 或其他以太网帧设备的场景。
- **`NetworkIsolated`：隔离网络。** VM 没有自动出站通道。使用 `PacketEndpoint()` 接入进程内虚拟网络或自定义报文处理逻辑，适合测试和协议实验。省略 `Mode` 时采用此模式。
- **`NetworkPCAP`：静默观察主机网卡。** 使用 `Device` 指定接口，通过 `OnPacket` 接收抓包结果。所有注入都被禁止，包括协议栈自动回复和手工 TCP RST。

当前高层配置、拨号和监听接口覆盖 **IPv4**。共享网关转发 **TCP/UDP**，不提供任意 IP 协议透传、ICMP 出站代理或自动端口映射。VM 内监听不会自动变成主机上的监听端口。

桥接模式不会自动创建 TAP、配置操作系统网桥或修改物理网卡。pcap 旁路模式也不会因为填写了网卡名而变成桥接模式。

## 2. 准备运行环境

在 Yaklang 仓库根目录使用项目声明的 Go 工具链及依赖。本文示例可以保存到临时 `.go` 文件，再从仓库根目录执行 `go run /path/to/example.go`。

本包依赖仓库的 pcap 相关构建环境。共享和隔离模式运行时不需要打开主机抓包设备，但编译整个包仍需满足这些依赖。pcap 模式还需要系统抓包支持和打开目标设备的权限；桥接设备的准备方式由操作系统及设备实现决定。

## 3. 共享连接：HTTP 完整示例

下面程序使用 VM 的私有地址和显式配置的 DNS 访问给定 URL。运行时会产生正常的 DNS 和 HTTP/HTTPS 流量；请使用你实际可达的 DNS 服务器和目标地址。

```go
package main

import (
    "context"
    "fmt"
    "io"
    "log"
    "net/http"
    "net/netip"
    "os"
    "time"

    "github.com/yaklang/yaklang/common/netstackvm"
)

func main() {
    if len(os.Args) != 3 {
        log.Fatal("usage: go run example.go <DNS IPv4> <URL>")
    }
    dns, err := netip.ParseAddr(os.Args[1])
    if err != nil {
        log.Fatal(err)
    }
    if err := run(dns, os.Args[2]); err != nil {
        log.Fatal(err)
    }
}

func run(dns netip.Addr, url string) error {
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    vm, err := netstackvm.NewNetworkVM(ctx, netstackvm.NetworkVMConfig{
        Mode:    netstackvm.NetworkShared,
        Address: netip.MustParsePrefix("10.90.0.2/24"),
        DNS:     []netip.Addr{dns},
    })
    if err != nil {
        return err
    }
    defer vm.Close()

    transport := &http.Transport{DialContext: vm.DialContext}
    defer transport.CloseIdleConnections()
    client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
    req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return err
    }
    response, err := client.Do(req)
    if err != nil {
        return err
    }
    defer response.Body.Close()
    fmt.Println(response.Status)
    _, err = io.Copy(os.Stdout, response.Body)
    return err
}
```

共享模式不要设置 `Gateway`、`Link`、`Device` 或 `DHCP`；网关和内部链路由 VM 自己管理。外部服务看到的是主机正常出站连接的地址，而不是 VM 的 `10.90.0.2`。

### 出站策略

`NetworkVMConfig.HostDialContext` 只适用于共享模式。它接收实际要建立的主机连接，可以检查目的地址、拒绝连接，或接入自己的拨号器。下面函数可直接赋给该字段：

```go
package examples

import (
    "context"
    "fmt"
    "net"
    "time"
)

func HostDialPolicy(ctx context.Context, network, address string) (net.Conn, error) {
    _, port, err := net.SplitHostPort(address)
    if err != nil {
        return nil, err
    }
    if port == "25" {
        return nil, fmt.Errorf("SMTP destination is disabled: %s", address)
    }
    return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, network, address)
}
```

这是目的地址/端口策略，不是域名或 HTTP 内容过滤器。域名已经在 VM 内解析，DNS 连接本身也经过该拨号策略。自定义拨号器必须响应 `ctx` 取消，否则可能阻塞 VM 关闭。

当前共享网关最多同时处理 256 个 TCP/UDP 会话。UDP 转发保留数据报边界，单向读取等待超过 30 秒会结束该会话；这些值目前不是公开配置项。

## 4. pcap：静默观察完整示例

下面程序接收接口名，例如 macOS 的 `lo0` 或实际网卡名。它只累计捕获到的 TCP 包数量，并在收到 Ctrl-C 时退出。

```go
package main

import (
    "context"
    "fmt"
    "log"
    "os"
    "os/signal"
    "sync/atomic"
    "time"

    "github.com/gopacket/gopacket"
    "github.com/gopacket/gopacket/layers"
    "github.com/yaklang/yaklang/common/netstackvm"
)

func main() {
    if len(os.Args) != 2 {
        log.Fatal("usage: go run capture.go <interface name>")
    }
    if err := capture(os.Args[1]); err != nil {
        log.Fatal(err)
    }
}

func capture(device string) error {
    ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
    defer stop()
    var packets atomic.Uint64
    vm, err := netstackvm.NewNetworkVM(ctx, netstackvm.NetworkVMConfig{
        Mode:   netstackvm.NetworkPCAP,
        Device: device,
        OnPacket: func(packet gopacket.Packet) {
            if packet.Layer(layers.LayerTypeTCP) != nil {
                packets.Add(1)
            }
        },
    })
    if err != nil {
        return err
    }
    defer vm.Close()
    ticker := time.NewTicker(time.Second)
    defer ticker.Stop()
    for {
        select {
        case <-ctx.Done():
            return nil
        case <-ticker.C:
            fmt.Printf("captured TCP packets: %d\n", packets.Load())
        }
    }
}
```

使用要求与行为：

- 只配置 `Device` 和可选的 `OnPacket`；不要同时配置地址、网关、DNS、DHCP、主动链路或主机拨号策略。
- `OnPacket` 在收包路径同步执行，必须快速返回。耗时分析应通过有界队列交给其他 goroutine；队列满时自行决定丢弃策略。不要在回调中同步调用 `vm.Close()`。
- `DialContext`、`ListenTCP`、`ListenUDP` 返回 `ErrPassiveNetwork`，可用 `errors.Is` 判断。
- 即使直接使用 `Stack()` 触发协议栈发包，最终写包入口仍会丢弃报文。底层写操作返回成功不代表网卡实际发送了报文。
- 新 `NetworkPCAP` 模式没有解除静默限制的选项。它提供报文观察，不会把抓到的任意 TCP 会话自动转成可读写的 `net.Conn`。

## 5. 桥接：独立 MAC 和 DHCP

先由调用方准备一个 `io.ReadWriteCloser` 设备：每次 `Read` 返回一整帧以太网报文，每次 `Write` 写出一整帧，`Close` 必须解除阻塞中的读写。裸 TCP 字节流和只提供 IP 包的 TUN 设备不能直接当作这个设备使用。

下面是接收已准备设备的完整函数，**不包含操作系统 TAP 创建步骤**：

```go
package examples

import (
    "context"
    "io"
    "net"
    "time"

    "github.com/yaklang/yaklang/common/netstackvm"
)

func OpenDHCPVM(ctx context.Context, device io.ReadWriteCloser) (*netstackvm.NetworkVM, error) {
    // 为每台 VM 分配不同的本地管理单播 MAC。
    mac := net.HardwareAddr{0x02, 0x00, 0x00, 0x00, 0x90, 0x02}
    link, err := netstackvm.NewEthernetLink(ctx, device, mac, 1500)
    if err != nil {
        device.Close()
        return nil, err
    }
    vm, err := netstackvm.NewNetworkVM(ctx, netstackvm.NetworkVMConfig{
        Mode: netstackvm.NetworkBridged,
        Link: link,
        DHCP: true,
    })
    if err != nil {
        link.Close()
        return nil, err
    }
    readyCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
    defer cancel()
    if err := vm.WaitReady(readyCtx); err != nil {
        vm.Close()
        return nil, err
    }
    return vm, nil // 调用方负责 defer vm.Close()。
}
```

创建成功后，通过 `Address()`、`Gateway()` 和 `DNSServers()` 读取租约结果。默认网关取自 DHCP Router 选项，不使用 DHCP Server 地址代替。租约撤销后会清除地址、路由和 DNS；`WaitReady` 此时会报告没有当前租约，而不是继续使用旧地址。

`NewNetworkVM` 不等待 DHCP 完成。`WaitReady` 只检查初始化/当前地址状态，不是互联网连通性探针；某次等待超时也不会自行关闭 VM 或停止 DHCP。上面的示例在等待失败时显式关闭 VM。

使用静态地址时，把配置改为 `DHCP: false`，设置 `Address`（如 `netip.MustParsePrefix("192.168.50.20/24")`），按需设置同子网且不同于 VM 地址的 `Gateway`，以及 `DNS`。这些地址必须由你根据实际网络分配，示例地址不能直接作为现场配置。

DHCP 模式不能同时设置静态 `Address`、`Gateway` 或 `DNS`。`NewEthernetLink` 保留 VM 的 MAC，不替换为主机 MAC；上游设备及网络必须允许这个独立身份接入。

## 6. 隔离网络与底层报文

用 `NetworkIsolated` 和静态 `Address` 创建 VM 后，默认不会把报文发送到主机网卡。未提供自定义 `Link` 时，`PacketEndpoint()` 返回 channel endpoint：

- `ReadContext(ctx)` 取出 VM 发出的报文。
- `InjectInbound(protocol, pkt)` 注入对端报文，IPv4 使用 `header.IPv4ProtocolNumber`。
- channel 中的报文是网络层报文，不带以太网头；`EthernetLink` 对接的则是完整以太网帧。
- 读取到的 `*stack.PacketBuffer` 由调用方 `DecRef()`；新建入站报文注入后也要释放调用方持有的引用。
- 同一个出站队列应由一个转发循环消费，避免多个消费者抢包。共享模式内部已经消费其队列，不暴露 `PacketEndpoint()`；桥接和 pcap 模式也返回 `nil`。

双向连接两个 channel VM、TCP/UDP 收发的可运行参考见 [network_vm_test.go](network_vm_test.go) 中的 `TestNetworkVMIsolatedTCPUDP`。真实以太网帧、ARP、TCP 和 ICMP 的参考见 [ethernet_link_test.go](ethernet_link_test.go)。

如果只需要一个现成的双 channel TCP 实验环境，也可以使用旧接口 `NewChannelNetStackVirtualMachineEntry("10.0.0.1")` / `NewChannelNetStackVirtualMachineEntry("10.0.0.2")`，再调用 `BridgeChannelNetStacks(ctx, a, b)`。该辅助接口接收旧的 entry 类型，不接收新的 `NetworkVM`。

`Stack()` 用于原始协议、路由和其他 gVisor 能力。直接修改协议栈时，应自行维护与 VM 地址、路由和生命周期的一致性。

## 7. 拨号、监听、DNS 和关闭

- `DialContext(ctx, network, address)`：支持 `tcp`、`tcp4`、`udp`、`udp4`，返回 `net.Conn`。地址使用 `IP:port` 或 `hostname:port`，目标端口必须在 1–65535。
- `ListenTCP("10.90.0.2:8080")`：在 VM 内监听，返回 `net.Listener`。
- `ListenUDP("10.90.0.2:5353")`：在 VM 内监听，返回 `net.PacketConn`。使用 `ReadFrom` / `WriteTo` 保留数据报语义。
- 监听地址必须是 IPv4 字面量，不能使用主机名。共享模式没有自动入站端口映射，因此这些监听不会自动对宿主机或局域网开放。
- 域名通过静态 `DNS` 或 DHCP 下发的 DNS 解析，当前实现使用列表中的第一台服务器，没有多服务器故障切换。未配置 DNS 时，IP 字面量仍可拨号，主机名拨号会报错，不会回退到主机 DNS。
- 拨号上下文主要约束建连过程；连接建立后，读写超时请使用 `SetDeadline`、`SetReadDeadline`、`SetWriteDeadline` 或上层客户端超时。UDP 建连成功不代表对端可达。
- 创建 VM 使用生命周期上下文；单次请求使用单独的超时上下文，避免请求结束时顺带取消整台 VM。
- 成功创建后始终 `defer vm.Close()`。关闭会取消后台任务并释放 VM 持有的链路、协议栈和共享连接；重复关闭是允许的。
- 传入的 `Link` 在配置校验通过后由 VM 接管，后续初始化失败也会关闭它。若构造函数在校验阶段失败，调用方仍应清理提前创建的链路。桥接示例对所有失败路径都做了清理。

## 8. 分步 TCP 握手和挥手

`TCPProbe` 支持 channel 双虚拟机的完整分步握手，以及下节的真实网卡半开探测。原有 `NetStackVirtualMachineEntry.StartTCPProbe` 使用 channel 后端，不能直接接收 pcap entry；它不是 `NetworkVM` 的方法。

操作顺序：

1. 用 `NewChannelNetStackVirtualMachineEntry` 创建两个同子网 entry，并在服务端 `ListenTCP`、启动 `Accept`。
2. 客户端调用 `StartTCPProbe(ctx, target, serverEntry)`，成功后 `defer probe.Close()`。
3. 依次调用 `ProbeSYN()` → `ReceiveSYNACK()` → `ProbeACK()`。
4. 握手完成后可用 `probe.Conn()` 取得连接，或用 `probe.Write()` 发送载荷。
5. 关闭时调用 `SendFIN()`；服务端应用需要配合关闭写方向/连接，再调用 `ReceivePeerClose()` → `SendFinalACK()`。

步骤返回的 `TCPSegment` 包含真实地址、端口、序号、确认号和 flags。跳过步骤会返回错误。`ReceivePeerClose` 返回对端 ACK 和 FIN 两个报文结果。

不要同时在同一对 entry 上运行 `BridgeChannelNetStacks` 和 `StartTCPProbe`，否则两者会竞争出站队列，破坏分步控制。完整的连接、载荷和挥手示例见 [tcp_probe_test.go](tcp_probe_test.go) 的 `TestTCPProbeHandshakePayloadAndClose`。

### 真实网卡半开探测与重试

真实网卡使用 `OpenHalfOpenSYN` 创建**主动探测会话**，再调用会话的 `StartTCPProbe`。返回的仍是 `*TCPProbe`，与 channel 后端共用步骤、响应校验和重试控制；该后端的 `ProbeACK` 固定返回 `ErrHalfOpenACK`。`NetworkPCAP` 的静默策略保持不变。

下面是可编译的单目标示例。`device`、`source`、`gateway` 必须与实际接口、地址及路由一致；同网段或回环目标可不提供网关。本 API 当前只接受 IPv4 字面量目标。

```go
package examples

import (
    "context"
    "fmt"
    "net"
    "time"

    "github.com/yaklang/yaklang/common/netstackvm"
)

func HalfOpen(ctx context.Context, device, source, gateway, target string) error {
    iface, err := net.InterfaceByName(device)
    if err != nil { return err }
    session, err := netstackvm.OpenHalfOpenSYN(ctx, netstackvm.HalfOpenSYNConfig{
        Iface: iface,
        SourceIP: net.ParseIP(source),
        Gateway: net.ParseIP(gateway),
        MaxInFlight: 256,
        PacketsPerSecond: 1000,
    })
    if err != nil { return err }
    defer session.Close()

    probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
    defer cancel()
    probe, err := session.StartTCPProbe(probeCtx, target,
        netstackvm.WithSYNRetry(netstackvm.SYNRetryPolicy{
            MaxAttempts: 3, // 首次发送也计算在内
            SendTimeout: 3*time.Second,
            ResponseTimeout: 500*time.Millisecond,
            MaxResponseTimeout: 2*time.Second,
        }))
    if err != nil { return err }
    defer probe.Close()
    if _, err = probe.ProbeSYNContext(probeCtx); err != nil { return err }
    synAck, err := probe.ReceiveSYNACKContext(probeCtx)
    if err != nil { return err }
    fmt.Printf("open: %s:%d\n", synAck.RemoteIP, synAck.RemotePort)
    return nil // 不调用 ProbeACK；Close 不会注入 ACK/FIN/RST。
}
```

`StartTCPProbe` 的 `ctx` 控制探测生命周期；`ProbeSYNContext`、`ReceiveSYNACKContext` 还可以接收各步骤的更短期限。等待并发名额、报文生成、速率配额、设备写入队列、响应及重试退避都响应取消。未完成的原生 libpcap 写入没有强制取消接口：已经进入该调用的操作必须等它返回，不能保证此处硬实时取消。会话关闭会取消探测并等待底层资源释放；调用方负责等待自己的 worker 退出。

原来的 `ProbeSYN()`、`ReceiveSYNACK()` 仍可使用，采用创建 probe 时的生命周期 context。channel 后端默认只尝试一次，以保持原有分步握手行为；真实网卡后端默认总计尝试 3 次。两者都可通过 `WithSYNRetry` 调整。

**如何区分结果：** 使用 `errors.Is` 检查返回错误。

- 无错误：收到与接口、四元组、本次 SYN 序号匹配的 SYN-ACK。在已确认不代答的网络路径上可报告 open；相关性校验不能识别透明代理伪造的远端响应。
- `ErrProbeRefused`：收到匹配的 RST+ACK；是一次明确的拒绝响应。
- `ErrProbeNoResponse`：尝试预算耗尽，未收到有效响应；可能丢包、被过滤或目标不可达，**不能据此判定 closed**。
- `ErrUnverifiedSYNTransport`：所选非回环接口使用 raw-IP 或 point-to-point 传输，默认拒绝创建主动会话，避免代理 SYN-ACK 被直接解释为真实端口开放。
- `ErrProbeSend`：本地发送/生成失败，保留底层错误；不能解释为目标无响应。
- `context.Canceled` / `context.DeadlineExceeded`：调用者或生命周期取消/超时。

报文还需通过 flags、长度、分片检查。物理接口校验 IPv4/TCP 校验和；已知 loopback 接口允许宿主栈的校验和卸载占位值，例如 macOS `lo0`。SYN-ACK 和 RST 都不注入 gVisor。来源、目标、端口、ACK 不匹配的报文，以及旧探测的迟到响应不能直接生成结果；生成 SYN 时也核对 endpoint 自身的 ISN，防止队列中的旧 SYN 被误认成本次发送。

**TUN / 透明代理误报：** Windows 实机的 sing-tun `tun0`（Npcap DLT 12）会对未监听端口返回匹配的 SYN-ACK，而远端没有接受连接。这种代答同样可以通过四元组、ACK、校验和检查；重试无法解决，也不能通过发送第三次 ACK“验证”而仍称为半开扫描。

因此会话默认只接纳 Ethernet 和已识别的主机 loopback；其他链路或 point-to-point 接口在启用注入之前返回 `ErrUnverifiedSYNTransport`。`synscanx` 沿用该默认策略，不会退回旧发包器或偷偷完成握手。路由选中代理 TUN 时，应显式选择物理接口及对应的源 IP/网关。`HalfOpenSYNConfig.AllowUnverifiedTransport: true` 仅为已核实路径的低层调用者保留；启用后必须自行解释结果，不能直接承诺远端 open。Ethernet 上也可能存在透明 SYN 代理，此策略不是响应来源的密码学认证。需要应用连通性证据时另做完整连接，并把结果与半开探测区分。

**突发流量与算法：** “写成功”仅表示 libpcap 接受报文，不保证网卡、网络或对端收到。gVisor 的有界输出队列、pcap 每订阅者的 1000 包接收队列、内核捕获缓冲和远端设备都可能丢包。扩大队列不是可靠性保证。

- 默认最多 256 个未完成 probe，名额保持到 `Close`/生命周期结束；超出后 `StartTCPProbe` 阻塞并可被 ctx 取消。
- 会话默认最多 1000 个 SYN/秒，burst 为 1；首次发送和重试共同受限，防止超时后的重试突发。ARP 邻居解析不计入这个 SYN 配额。
- 同一 probe 的重试重发完全相同的 SYN，保留四元组和 ISN，不重新分配源端口、不重建连接。gVisor 自己生成的重传和关闭报文不能直接出网。
- 第 k 次尝试的响应窗口为 `min(MaxResponseTimeout, ResponseTimeout × 2^(k-1) × (1+U[0,0.2]))`。默认约为 0.5–0.6 秒、1–1.2 秒、2 秒。响应窗口从本地写入成功后开始。
- 写入失败也消耗同一份 `MaxAttempts`，重发前按同样的退避公式等待；每次发送另受 `SendTimeout` 限制。总期限由 probe/步骤 ctx 收紧。不是“写入重试 3 次再叠加网络重试 3 次”。
- 这是有上限的退避策略，不是自适应 RTT 估计器；没有有限次数重试可以保证不漏报。提高重试次数会增加扫描时间和对端负载；应按网络时延设置响应窗口。

### 同步单次调用与上层批量并发

不需要分步处理时，使用类似 `DialContext` 的同步调用：

```go
probeCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
defer cancel()
synAck, err := session.ProbeSYN(probeCtx, "192.0.2.20:443")
// 返回时已完成响应等待/重试，并释放 probe 和并发名额。
// err == nil 表示匹配的 SYN-ACK；它没有完成三次握手，也不返回 net.Conn。
```

`ProbeSYN` 内部顺序执行 `StartTCPProbe` → `ProbeSYNContext` → `ReceiveSYNACKContext`，在所有返回路径上关闭 probe。返回错误保留上面的分类；只有最终结果，不存在“提交成功但结果稍后经回调返回”的歧义。单个 context 超时不会取消同会话的其他 probe，关闭 session 则取消全部 probe。

批量并发由调用层控制，每个 worker 同步执行一次探测，每个目标有独立 context。下面的完整函数使用固定 worker 数量，按输入顺序返回结果；某个目标失败不取消其他目标，批量 context 取消则终止全部等待。

```go
package examples

import (
    "context"
    "sync"
    "time"

    "github.com/yaklang/yaklang/common/netstackvm"
)

type ProbeResult struct {
    Target string
    SYNACK netstackvm.TCPSegment
    Err error
}

func ProbeBatch(ctx context.Context, session *netstackvm.HalfOpenSYN,
    targets []string, concurrency int, perTargetTimeout time.Duration) []ProbeResult {
    if concurrency < 1 { concurrency = 1 }
    if concurrency > len(targets) { concurrency = len(targets) }
    results := make([]ProbeResult, len(targets))
    jobs := make(chan int)
    var wg sync.WaitGroup
    for i := 0; i < concurrency; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for index := range jobs {
                target := targets[index]
                probeCtx, cancel := context.WithTimeout(ctx, perTargetTimeout)
                // 简单调用可直接用 session.ProbeSYN；这里展示上层管理分步 API。
                ack, err := probeOne(probeCtx, session, target)
                cancel() // 在本轮立即释放 timer，不能 defer 到 worker 退出。
                results[index] = ProbeResult{Target: target, SYNACK: ack, Err: err}
            }
        }()
    }
    for index, target := range targets {
        if ctx.Err() != nil {
            results[index] = ProbeResult{Target: target, Err: ctx.Err()}
            continue
        }
        select {
        case jobs <- index:
        case <-ctx.Done():
            results[index] = ProbeResult{Target: target, Err: ctx.Err()}
        }
    }
    close(jobs)
    wg.Wait()
    return results
}

func probeOne(ctx context.Context, session *netstackvm.HalfOpenSYN, target string) (netstackvm.TCPSegment, error) {
    probe, err := session.StartTCPProbe(ctx, target)
    if err != nil { return netstackvm.TCPSegment{}, err }
    defer probe.Close()
    if _, err = probe.ProbeSYNContext(ctx); err != nil { return netstackvm.TCPSegment{}, err }
    return probe.ReceiveSYNACKContext(ctx)
}
```

半开会话已移除 `HalfOpenSYNConfig.OnOpen` / `OnResult`、`Emit` 和 `Wait`。旧调用迁移为 `ProbeSYN` 直接取结果，或自行组织 worker 并执行上述 probe 步骤；调用方等待自己的 worker 结束，再关闭 session。分步 API 仍需 `defer probe.Close()`，没有 `Wait` 替调用方释放遗忘关闭的 probe。

`synscanx` 的 TCP 批量路径使用上层固定 worker（默认 256），每个目标默认独立 15 秒期限，可通过 Go 选项 `WithTCPProbeConcurrency(n)` / `WithTCPProbeTimeout(duration)` 调整。每个 worker 直接创建 probe、发送、等待结果并关闭，生产结束后等待 worker 全部退出，再关闭扫描结果流。纯 TCP 扫描不再额外睡眠 `WithWaiting` 的时间；混合 UDP 扫描仍保留 UDP 响应窗口。扫描器既有结果 channel / 兼容回调只在扫描器层处理，netstackvm 不存储或调用它们。混合 UDP 扫描的额外抓包器不会绕过 TCP 校验；主动会话初始化失败会明确返回错误。

主动会话借用宿主机 IP，gVisor 的端口绑定并不等于预留了宿主机操作系统的端口；宿主栈仍可能发送自己的 RST。本接口约束的是本进程注入行为，不能承诺与宿主所有现有连接完全隔离。需要独立网络身份时应使用桥接 VM 的独立 IP/MAC，而不是把静默 pcap 当成独立虚拟机。

## 9. 旧接口迁移

`NewNetStackVirtualMachineEntry`、`NewPCAPEndpoint` 和 `NewSystemNetStackVM` 现在默认禁止 pcap 注入。旧 entry 的主动拨号、TCP 监听和 DHCP 会在只读状态下返回 `ErrPassiveNetwork`。

需要观察主机网络时，优先迁移到 `NewNetworkVM(..., NetworkVMConfig{Mode: NetworkPCAP, Device: ...})`。需要普通应用出站连接时，优先选择 `NetworkShared`；需要独立二层身份时选择 `NetworkBridged`。

旧 `Option` 构造器可通过 `WithPCAPReadOnly(false)` 显式允许主动注入。这会允许实际向主机网卡写包，包括 TCP 报文，不能用于要求静默的旁路观察。设置 `WithForceSystemNetStack(true)` 的 entry 仍强制只读。新 `NetworkPCAP` 模式不接受这个旧选项。

`DisallowTCP` / `FastKillTCP` 是旧的主动 RST 干预功能，不是“禁止 VM 自己发 TCP”的开关；特别是 `FastKillTCP` 会显式开启主动注入。不要把它用于实现旁路安全策略。

旧 entry 和 `NetStackVirtualMachine` 也提供 `Close()`；请关闭它们，不要仅调用 `GetStack().Close()` 而遗漏抓包资源。

## 10. 测试与复现

以下命令都在仓库根目录执行。

默认整包测试不依赖物理桥接网络，不执行外网扫描，也不会启动永久监听：

```sh
go test ./common/netstackvm -count=1 -timeout=120s
go test -race ./common/netstackvm -count=3 -timeout=120s
go vet ./common/netstackvm ./common/netstackvm/cmd
```

单独验证旁路、桥接、DHCP 和共享模式：

```sh
go test -race ./common/netstackvm \
  -run 'TestPCAP|TestEthernetBridge|TestNetworkVM' \
  -count=1 -timeout=120s
```

macOS 上的真实回环抓包测试（测试进程通过普通主机 UDP 套接字产生流量，两个 VM 只抓包）：

```sh
NETSTACKVM_PCAP_DEVICE=lo0 go test -race ./common/netstackvm \
  -run '^TestPCAPLiveLoopbackOptional$' -count=3 -timeout=30s
```

Linux 应选择实际的回环接口名，例如 `lo`，并确保具有抓包权限。

Windows 已在 Windows 11 / Go 1.22.12 amd64 / LLVM MinGW / Npcap 环境原生执行 race 检测和真实抓包测试。编译需要 CGO 和兼容的 C 编译器；`go test -c -race` 生成的二进制也可以复制到安装 Npcap 的 Windows 机器运行。PowerShell 示例（先确保 `go`、编译器在当前进程 PATH 中）：

```powershell
$env:CGO_ENABLED = '1'
$env:CC = 'x86_64-w64-mingw32-clang.exe'
$env:NETSTACKVM_PCAP_DEVICE = 'Loopback Pseudo-Interface 1'
go test -race ./common/netstackvm ./common/synscanx -count=3 -shuffle=on -timeout=180s
```

`NETSTACKVM_PCAP_DEVICE` 使用 `net.InterfaceByName` 可识别的回环接口名。可选实机用例检查 64 次探测期间已有 TCP 连接保持可用、监听器没有接受半开连接、丢弃前两次捕获响应后重试成功、匹配 RST、ctx 取消和抓包关闭耗时。默认不设置变量时，这些实机测试跳过，mock 测试仍执行。

对安装了代理 TUN 的机器，还可验证默认拒绝以及低层显式 opt-in 的生命周期；该测试不向远端发送 SYN：

```powershell
$env:NETSTACKVM_UNVERIFIED_DEVICE = 'tun0'
$env:NETSTACKVM_UNVERIFIED_SOURCE = '172.18.0.1' # 改成接口的实际地址
go test -race ./common/netstackvm -run '^TestHalfOpenPCAPUnverifiedTransportRejected$' -count=3 -timeout=60s
```

物理网卡可通过 `TestHalfOpenPCAPControlledPeer` 做双端验证：在受控对端保持一个普通 TCP 连接，读取服务的累计 Accept 数，探测 16 次，再检查 Accept 数不变且原连接仍可读写。对端协议为 `COUNT\n` 返回十进制累计连接数和换行。设置 `NETSTACKVM_LIVE_DEVICE`、`NETSTACKVM_LIVE_SOURCE`、`NETSTACKVM_LIVE_TARGET`（`IP:port`），跨网段时设置 `NETSTACKVM_LIVE_GATEWAY`；可另设 `NETSTACKVM_LIVE_CLOSED_TARGET` 指向确定未监听端口。后者允许匹配拒绝或无响应，绝不允许 open，因为防火墙可能丢弃 RST。

```powershell
$env:NETSTACKVM_LIVE_DEVICE = '以太网'
$env:NETSTACKVM_LIVE_SOURCE = '192.168.0.140' # 改为测试机实际地址
$env:NETSTACKVM_LIVE_TARGET = '192.168.0.135:59823' # 改为受控服务地址
go test -race ./common/netstackvm -run '^TestHalfOpenPCAPControlledPeer$' -count=1 -timeout=60s
```

对端可用以下 Python 3 服务，运行时传入对端的 LAN IPv4。它打印两个端口：第一个监听并统计连接，第二个只绑定而不监听。将输出填入上面的 target 变量，验收后用 Ctrl-C 退出。

```python
# python3 peer.py <LAN_IP>
import socket, sys, threading
listener = socket.socket()
listener.bind((sys.argv[1], 0))
listener.listen(128)
closed = socket.socket()
closed.bind((sys.argv[1], 0))
print("open:", listener.getsockname(), "non-listening:", closed.getsockname(), flush=True)
lock, accepted = threading.Lock(), 0

def serve(conn):
    try:
        with conn, conn.makefile("rwb", buffering=0) as stream:
            for line in stream:
                if line.strip() == b"COUNT":
                    with lock:
                        count = accepted
                    stream.write((str(count) + "\n").encode())
    except OSError:
        pass

while True:
    conn, _ = listener.accept()
    with lock:
        accepted += 1
    threading.Thread(target=serve, args=(conn,), daemon=True).start()
```

这些实机测试是有限流量的功能回归，不代表任意速率下不会丢包，也不证明所有 Npcap、VPN 或网卡驱动组合行为一致。

只测试主机非回环接口能否打开，可显式运行已有的可选测试：

```sh
NETSTACK_TRY_NIC=1 go test ./common/netstackvm \
  -run '^TestHostPCAPDeviceOptional$' -count=1 -timeout=30s
```

`NETSTACKVM_HOST_TESTS=1` 会启用既有手工测试，其中包含主动外网扫描、固定目标连接和长期监听。它不是默认回归测试开关，不应在普通整包测试或 CI 中随意启用；需要时先阅读目标测试，并通过 `-run` 精确选择。

主要验证入口：

- [network_vm_test.go](network_vm_test.go)：隔离 TCP/UDP、共享网关到主机真实套接字、分片数据报、策略、取消和租约变更。
- [network_dhcp_test.go](network_dhcp_test.go)：DHCP Discover/Offer/Request/ACK、等待超时和关闭。
- [ethernet_link_test.go](ethernet_link_test.go)：完整以太网帧、独立 MAC、ARP、TCP、ICMP。
- [network_usage_test.go](network_usage_test.go)：VM 内 DNS/HTTP 和可选实机 pcap 双订阅者。
- [pcap_policy_test.go](pcap_policy_test.go)：自动 RST、手工写包、过滤器和旧接口均不能绕过只读限制。
- [tcp_probe_test.go](tcp_probe_test.go)：分步握手、载荷、挥手及原有 DialTCP 回归。
- [halfopen_syn_test.go](halfopen_syn_test.go)：丢包/写失败重试、错误响应/旧 ISN、ctx/背压、并发清理和可选回环半开测试。

真实回环半开验证（会向测试临时启动的本机 TCP 端口主动发送 SYN）：

```sh
NETSTACKVM_PCAP_DEVICE=lo0 go test -race ./common/netstackvm \
  -run '^TestHalfOpenPCAP' -count=3 -timeout=90s
```

## 11. 常见问题

- **共享模式设置 `Gateway` 后创建失败**：删除它；共享网关由 VM 自动配置。
- **域名拨号报告 DNS 未配置**：设置静态 `DNS`，或在桥接模式下等待 DHCP 提供 DNS。也可以先用 IP 字面量排除解析问题。
- **用 `127.0.0.1` 访问不到主机服务**：VM 与主机不是同一个协议栈，VM 的回环地址不能当作主机回环地址使用。共享模式可在 `HostDialContext` 中，把约定的虚拟目标地址映射到主机地址；参考共享网关测试中的做法。
- **隔离 VM 拨号超时**：检查两个 endpoint 是否有双向转发，以及地址和路由是否匹配；隔离模式不会自动共享主机网络。
- **DHCP 一直不就绪**：检查帧设备是否已接到目标二层网络、MAC 是否唯一、上游是否允许额外 MAC，以及网络中是否有 DHCP 服务。给 `WaitReady` 设置超时，并在失败后关闭或主动决定继续等待。
- **pcap 抓不到包**：确认接口名、权限、抓包驱动和接口上是否有流量。`WaitReady` 成功只表示 VM 初始化完成，不保证观察到了流量。
- **pcap 无法拨号或监听**：这是旁路模式的预期行为。根据用途另建共享或桥接 VM。
- **关闭卡住**：检查自定义设备的 `Close` 是否解除读写阻塞、自定义主机拨号器是否响应上下文，以及 `OnPacket` 是否阻塞或同步调用了 VM 关闭。
