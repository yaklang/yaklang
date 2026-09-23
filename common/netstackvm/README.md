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

分步探测属于旧的 `NetStackVirtualMachineEntry` channel 接口，适合协议步骤验证；不能直接用在 pcap entry 上，也不是 `NetworkVM` 的方法。

操作顺序：

1. 用 `NewChannelNetStackVirtualMachineEntry` 创建两个同子网 entry，并在服务端 `ListenTCP`、启动 `Accept`。
2. 客户端调用 `StartTCPProbe(ctx, target, serverEntry)`，成功后 `defer probe.Close()`。
3. 依次调用 `ProbeSYN()` → `ReceiveSYNACK()` → `ProbeACK()`。
4. 握手完成后可用 `probe.Conn()` 取得连接，或用 `probe.Write()` 发送载荷。
5. 关闭时调用 `SendFIN()`；服务端应用需要配合关闭写方向/连接，再调用 `ReceivePeerClose()` → `SendFinalACK()`。

步骤返回的 `TCPSegment` 包含真实地址、端口、序号、确认号和 flags。跳过步骤会返回错误。`ReceivePeerClose` 返回对端 ACK 和 FIN 两个报文结果。

不要同时在同一对 entry 上运行 `BridgeChannelNetStacks` 和 `StartTCPProbe`，否则两者会竞争出站队列，破坏分步控制。完整的连接、载荷和挥手示例见 [tcp_probe_test.go](tcp_probe_test.go) 的 `TestTCPProbeHandshakePayloadAndClose`。

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

Linux 应选择实际的回环接口名，例如 `lo`，并确保具有抓包权限。Windows 需要匹配实际捕获设备及抓包环境；本手册不宣称已经完成 Windows 实机验证。

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

## 11. 常见问题

- **共享模式设置 `Gateway` 后创建失败**：删除它；共享网关由 VM 自动配置。
- **域名拨号报告 DNS 未配置**：设置静态 `DNS`，或在桥接模式下等待 DHCP 提供 DNS。也可以先用 IP 字面量排除解析问题。
- **用 `127.0.0.1` 访问不到主机服务**：VM 与主机不是同一个协议栈，VM 的回环地址不能当作主机回环地址使用。共享模式可在 `HostDialContext` 中，把约定的虚拟目标地址映射到主机地址；参考共享网关测试中的做法。
- **隔离 VM 拨号超时**：检查两个 endpoint 是否有双向转发，以及地址和路由是否匹配；隔离模式不会自动共享主机网络。
- **DHCP 一直不就绪**：检查帧设备是否已接到目标二层网络、MAC 是否唯一、上游是否允许额外 MAC，以及网络中是否有 DHCP 服务。给 `WaitReady` 设置超时，并在失败后关闭或主动决定继续等待。
- **pcap 抓不到包**：确认接口名、权限、抓包驱动和接口上是否有流量。`WaitReady` 成功只表示 VM 初始化完成，不保证观察到了流量。
- **pcap 无法拨号或监听**：这是旁路模式的预期行为。根据用途另建共享或桥接 VM。
- **关闭卡住**：检查自定义设备的 `Close` 是否解除读写阻塞、自定义主机拨号器是否响应上下文，以及 `OnPacket` 是否阻塞或同步调用了 VM 关闭。
