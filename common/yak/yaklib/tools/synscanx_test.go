package tools

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	_ "net/http/pprof"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/yaklang/pcap"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx"
	"github.com/yaklang/yaklang/common/synscanx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

func Test__scanx(t *testing.T) {
	t.Skip("跳过测试：依赖外部IP 172.22.166.244，不符合测试不外连的原则")

	log.SetLevel(log.DebugLevel)
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	startSYNPacketCounter := func() {
		go func() {
			for {
				time.Sleep(2 * time.Second)
				log.Infof("SYN 发包数 %v", synPacketCounter)
			}
		}()
	}
	startSYNPacketCounter()

	res, err := _scanx(
		//"192.168.124.50/24",
		//"124.222.42.210/24",
		//"192.168.3.3,192.168.3.5",
		"172.22.166.244",
		//"172.22.160.1",
		//"baidu.com",
		//"U:137",
		"12345",
		//synscanx.WithInitFilterPorts("443"),
		//synscanx.WithWaiting(5),
		synscanx.WithShuffle(false),
		synscanx.WithIface("vEthernet (WSL (Hyper-V firewall))"),
		//synscanx.WithConcurrent(2000),
		synscanx.WithSubmitTaskCallback(func(i string) {
			addSynPacketCounter()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
	t.Log("synPacketCounter:", synPacketCounter)
}

func Test__scanx2(t *testing.T) {
	t.Skip("跳过测试：依赖外部IP 124.222.42.210/24，不符合测试不外连的原则")

	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}

	startSYNPacketCounter := func() {
		go func() {
			for {
				time.Sleep(2 * time.Second)
				t.Log("SYN 发包数", synPacketCounter)
			}
		}()
	}
	startSYNPacketCounter()
	swg := utils.NewSizedWaitGroup(50)
	for _, target := range utils.ParseStringToHosts("124.222.42.210/24") {
		host := target
		swg.Add()
		go func() {
			defer swg.Done()
			res, err := _scanx(
				host,
				"21,22,443,445,80",
				//synscanx.WithInitFilterPorts("443"),
				//synscanx.WithWaiting(2),
				synscanx.WithShuffle(false),
				//synscanx.WithConcurrent(2000),
				synscanx.WithSubmitTaskCallback(func(i string) {
					addSynPacketCounter()
				}),
			)
			if err != nil {
				t.Fatal(err)
			}
			for re := range res {
				t.Log(re.String())
			}
		}()
	}

	t.Log("synPacketCounter:", synPacketCounter)
	swg.Wait()
}

func Test___scanxFromPingUtils(t *testing.T) {
	t.Skip("跳过测试：依赖内网IP 192.168.3.3/24，不符合测试不外连的原则")

	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}
	list := utils.ParseStringToHosts("192.168.3.3/24")

	c := make(chan *pingutil.PingResult)
	go func() {
		defer close(c)
		for _, ip := range list {
			c <- &pingutil.PingResult{
				IP: ip,
				Ok: true,
			}
		}
	}()

	res, err := _scanxFromPingUtils(
		c,
		//"47.52.100.35/24",
		//"U:137",
		"21,22,443,445,80",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithWaiting(5),
		synscanx.WithConcurrent(2000),
		synscanx.WithSubmitTaskCallback(func(i string) {
			addSynPacketCounter()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
	t.Log("synPacketCounter:", synPacketCounter)
}

func Test___scanxFromPingUtilsCancel(t *testing.T) {
	t.Skip("跳过测试：依赖内网IP 192.168.3.3/24，不符合测试不外连的原则")

	ctx, cancel := context.WithCancel(context.Background())
	synPacketCounter := 0
	addSynPacketCounter := func() {
		synPacketCounter++
	}
	list := utils.ParseStringToHosts("192.168.3.3/24")

	c := make(chan *pingutil.PingResult)
	go func() {
		defer close(c)
		for _, ip := range list {
			c <- &pingutil.PingResult{
				IP: ip,
				Ok: true,
			}
		}
	}()
	go func() {
		time.Sleep(2 * time.Second)
		cancel()
	}()
	res, err := _scanxFromPingUtils(
		c,
		//"47.52.100.35/24",
		//"U:137",
		"21,22,443,445,80",
		//synscanx.WithInitFilterPorts("443"),
		synscanx.WithCtx(ctx),
		synscanx.WithWaiting(5),
		synscanx.WithConcurrent(2000),
		synscanx.WithSubmitTaskCallback(func(i string) {
			addSynPacketCounter()
		}),
	)
	if err != nil {
		t.Fatal(err)
	}
	for re := range res {
		t.Log(re.String())
	}
	t.Log("synPacketCounter:", synPacketCounter)

	time.Sleep(3 * time.Second)
}

func Test___scanxFromPingUtils_NoAliveHosts(t *testing.T) {
	t.Skip("跳过测试：依赖内网IP 192.168.3.3/24，不符合测试不外连的原则")

	list := utils.ParseStringToHosts("192.168.3.3/24")
	c := make(chan *pingutil.PingResult)
	go func() {
		defer close(c)
		for _, ip := range list {
			c <- &pingutil.PingResult{
				IP: ip,
				Ok: false, // 设置所有主机都不存活
			}
		}
	}()

	// 执行扫描
	_, err := _scanxFromPingUtils(
		c,
		"80,443",
		synscanx.WithWaiting(1),
	)

	// 验证错误信息
	if err == nil {
		t.Fatal("expected error but got nil")
	}

	expectedErr := "no valid ping results found"
	if err.Error() != expectedErr {
		t.Fatalf("expected error message %q but got %q", expectedErr, err.Error())
	}
}

func Test___filter(t *testing.T) {
	t.Skip("跳过测试：存在无限循环的竞态条件测试，会导致测试永远不结束")

	wg := sync.WaitGroup{}
	for {
		filter := utils.NewPortsFilter()

		wg.Add(2)
		go func() {
			defer wg.Done()
			filter.Add("1-100")
			filter = nil
		}()
		time.Sleep(100 * time.Millisecond)

		go func() {
			defer wg.Done()

			if filter.Contains(1) {
				t.Log("contains 1")
			}
		}()

		wg.Wait()
	}

}

func Test___Loopback(t *testing.T) {
	t.Skip("跳过测试：需要特殊网络权限和网络接口，在某些环境中会失败")

	// 打开环回设备
	handle, err := pcap.OpenLive("lo", 1600, true, pcap.BlockForever)
	if err != nil {
		t.Fatal(err)
	}
	//defer handle.Close()
	var opts []any

	opts = append(opts, pcapx.WithLoopback(true))
	ipSrc := net.ParseIP("127.0.0.1").String()
	host := ipSrc
	opts = append(opts, pcapx.WithIPv4_Flags(layers.IPv4DontFragment))
	opts = append(opts, pcapx.WithIPv4_Version(4))
	opts = append(opts, pcapx.WithIPv4_NextProtocol(layers.IPProtocolTCP))
	opts = append(opts, pcapx.WithIPv4_TTL(64))
	opts = append(opts, pcapx.WithIPv4_ID(40000+rand.Intn(10000)))
	opts = append(opts, pcapx.WithIPv4_SrcIP(ipSrc))
	opts = append(opts, pcapx.WithIPv4_DstIP(host))
	opts = append(opts, pcapx.WithIPv4_Option(nil, nil))
	srcPort := rand.Intn(65534) + 1
	opts = append(opts,
		pcapx.WithTCP_SrcPort(srcPort),
		pcapx.WithTCP_DstPort(22),
		pcapx.WithTCP_Flags(pcapx.TCP_FLAG_SYN),
		pcapx.WithTCP_Window(1024),
		pcapx.WithTCP_Seq(500000+rand.Intn(10000)),
	)

	packetBytes, err := pcapx.PacketBuilder(opts...)
	if err != nil {
		t.FailNow()
	}
	// 发送数据包
	err = handle.WritePacketData(packetBytes)
	if err != nil {
		t.Fatal(err)
	}

	t.Log("Packet sent.")
	// 设置一个超时时间以便接收响应
	err = handle.SetBPFFilter("tcp and port 22")
	if err != nil {
		t.Fatal(err)
	}

	packetSource := gopacket.NewPacketSource(handle, handle.LinkType())
	packetSource.DecodeOptions.Lazy = true
	packetSource.DecodeOptions.NoCopy = true

	// 使用超时确保测试不会永久阻塞
	timeout := time.After(5 * time.Second)
	done := make(chan bool)
	go func() {
		for packet := range packetSource.Packets() {
			// 处理包
			t.Log("Received a packet.")
			networkLayer := packet.NetworkLayer()
			if networkLayer == nil {
				continue
			}
			ip, _ := networkLayer.(*layers.IPv4)
			if ip != nil {
				t.Logf("From %s to %s\n", ip.SrcIP, ip.DstIP)
			}
			transportLayer := packet.TransportLayer()
			if transportLayer == nil {
				continue
			}
			tcp, _ := transportLayer.(*layers.TCP)
			if tcp != nil {
				t.Logf("From port %d to port %d\n", tcp.SrcPort, tcp.DstPort)
			}
			done <- true
			return
		}
	}()

	select {
	case <-timeout:
		t.Fatal("Test timed out waiting for packets")
	case <-done:
		t.Log("Test completed successfully")
	}
}

func Test_Loopback_unix(t *testing.T) {
	destIP := "127.0.0.1"
	destPort := 22
	listenPort := 0

	srcPort := rand.Intn(65534) + 1
	// 监听 IP 数据包
	conn, err := net.ListenIP("ip4:tcp", &net.IPAddr{IP: net.ParseIP(fmt.Sprintf("0.0.0.0:%d", listenPort))})
	if err != nil {
		t.Logf("ListenIP failed: %v\n", err)
		return
	}
	defer conn.Close()

	ip4 := layers.IPv4{
		DstIP:    net.ParseIP("127.0.0.1"),
		SrcIP:    net.ParseIP("127.0.0.1"),
		Version:  4,
		TTL:      255,
		Protocol: layers.IPProtocolTCP,
	}

	tcpOption := layers.TCPOption{
		OptionType:   layers.TCPOptionKindMSS,
		OptionLength: 4,
		OptionData:   []byte{0x05, 0xB4},
	}
	// 构造一个 TCP SYN 包
	// 注意：这里不包括 IP 头部，因为 net.ListenIP 会处理 IP 头部
	tcp := layers.TCP{
		SrcPort: layers.TCPPort(srcPort),
		DstPort: layers.TCPPort(destPort),
		Window:  1024,
		Options: []layers.TCPOption{tcpOption},
		SYN:     true,
	}
	err = tcp.SetNetworkLayerForChecksum(&ip4)
	var defaultSerializeOptions = gopacket.SerializeOptions{
		FixLengths:       true,
		ComputeChecksums: true,
	}

	buf := gopacket.NewSerializeBuffer()
	if err := gopacket.SerializeLayers(buf, defaultSerializeOptions, &tcp); err != nil {
		t.Log(err)
	}

	// 发送数据包
	destAddr := &net.IPAddr{IP: net.ParseIP(destIP)}

	_, err = conn.WriteTo(buf.Bytes(), destAddr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error sending packet: %v\n", err)
		return
	}

	fmt.Println("TCP SYN packet sent")
	// 等待一段时间查看是否有响应
	time.Sleep(1 * time.Second)
}
