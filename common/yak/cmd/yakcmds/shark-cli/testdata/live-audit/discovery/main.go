// Fresh SSDP socket controls plus a bounded physical-network discovery capture.
package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/gopacket/gopacket/pcapgo"
	"github.com/yaklang/pcap"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

func must(e error) {
	if e != nil {
		panic(e)
	}
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: discovery OUTPUT_DIR PHYSICAL_IPV4")
	}
	out := os.Args[1]
	must(os.MkdirAll(out, 0700))
	devices, e := pcap.FindAllDevs()
	must(e)
	physical, loop := "", ""
	for _, d := range devices {
		if d.Name == "\\Device\\NPF_Loopback" {
			loop = d.Name
		}
		for _, a := range d.Addresses {
			if a.IP.String() == os.Args[2] {
				physical = d.Name
			}
		}
	}
	if physical == "" || loop == "" {
		panic("requested physical/loopback interface not found")
	}
	server, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	must(e)
	defer server.Close()
	client, e := net.DialUDP("udp4", nil, server.LocalAddr().(*net.UDPAddr))
	must(e)
	defer client.Close()
	stop := make(chan struct{})
	var wg sync.WaitGroup
	counts := map[string]int{}
	var mu sync.Mutex
	start := func(name, device, filter string) {
		h, e := pcap.OpenLive(device, 65535, false, 100*time.Millisecond)
		must(e)
		must(h.SetBPFFilter(filter))
		f, e := os.Create(filepath.Join(out, name+".pcap"))
		must(e)
		w := pcapgo.NewWriterNanos(f)
		must(w.WriteFileHeader(65535, h.LinkType()))
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer h.Close()
			defer f.Close()
			for {
				select {
				case <-stop:
					return
				default:
				}
				b, ci, e := h.ZeroCopyReadPacketData()
				if e == pcap.NextErrorTimeoutExpired {
					continue
				}
				if e != nil {
					return
				}
				must(w.WritePacket(ci, b))
				mu.Lock()
				counts[name]++
				mu.Unlock()
			}
		}()
	}
	start("loopback", loop, fmt.Sprintf("udp port %d", server.LocalAddr().(*net.UDPAddr).Port))
	start("physical", physical, "udp port 1900")
	nonce := make([]byte, 8)
	_, e = rand.Read(nonce)
	must(e)
	id := "uuid:audit-" + hex.EncodeToString(nonce)
	common := "HOST: 239.255.255.250:1900\r\nNT: upnp:rootdevice\r\nUSN: " + id + "::upnp:rootdevice\r\n"
	alive := "CACHE-CONTROL: max-age=66\r\nLOCATION: http://127.0.0.1:49152/audit.xml\r\nSERVER: Audit/1.0 UPnP/1.1 Test/1.0\r\n"
	wires := []string{"NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:alive\r\n" + alive + "\r\n", "NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:byebye\r\n\r\n", "NOTIFY * HTTP/1.1\r\n" + common + "NTS: ssdp:update\r\nLOCATION: http://127.0.0.1/audit.xml\r\nBOOTID.UPNP.ORG: 17\r\nNEXTBOOTID.UPNP.ORG: 18\r\nCONFIGID.UPNP.ORG: 5\r\n\r\n", "M-SEARCH * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nMAN: \"ssdp:discover\"\r\nMX: 1\r\nST: ssdp:all\r\n\r\n", "HTTP/1.1 200 OK\r\nST: upnp:rootdevice\r\nUSN: " + id + "::upnp:rootdevice\r\nEXT:\r\n" + alive + "\r\n", "NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\n\r\n", "not SSDP"}
	for _, wire := range wires {
		_, e = client.Write([]byte(wire))
		must(e)
		buf := make([]byte, 65535)
		n, _, e := server.ReadFromUDP(buf)
		must(e)
		if string(buf[:n]) != wire {
			panic("socket bytes differ")
		}
		time.Sleep(50 * time.Millisecond)
	}
	probe, e := net.DialUDP("udp4", &net.UDPAddr{IP: net.ParseIP(os.Args[2])}, &net.UDPAddr{IP: net.IPv4(239, 255, 255, 250), Port: 1900})
	must(e)
	_, e = probe.Write([]byte(wires[3]))
	must(e)
	defer probe.Close()
	time.Sleep(20 * time.Second)
	close(stop)
	wg.Wait()
	b, e := json.MarshalIndent(map[string]any{"physical_interface": physical, "physical_ip": os.Args[2], "frames": counts, "controlled_messages": wires, "probe": "one multicast M-SEARCH, no LOCATION retrieval"}, "", "  ")
	must(e)
	must(os.WriteFile(filepath.Join(out, "manifest.json"), b, 0600))
	fmt.Println(string(b))
}
