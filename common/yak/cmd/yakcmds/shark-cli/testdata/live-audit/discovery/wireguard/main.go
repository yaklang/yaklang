package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/curve25519"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

type memoryTUN struct {
	in, out chan []byte
	events  chan tun.Event
	stop    chan struct{}
	once    sync.Once
}

func newTUN() *memoryTUN {
	return &memoryTUN{in: make(chan []byte, 32), out: make(chan []byte, 32), events: make(chan tun.Event, 2), stop: make(chan struct{})}
}
func (t *memoryTUN) File() *os.File           { return nil }
func (t *memoryTUN) Name() (string, error)    { return "shark-audit-memory-only", nil }
func (t *memoryTUN) MTU() (int, error)        { return 1420, nil }
func (t *memoryTUN) BatchSize() int           { return 1 }
func (t *memoryTUN) Events() <-chan tun.Event { return t.events }
func (t *memoryTUN) Close() error             { t.once.Do(func() { close(t.stop); close(t.events) }); return nil }
func (t *memoryTUN) Read(bufs [][]byte, sizes []int, offset int) (int, error) {
	select {
	case p := <-t.in:
		sizes[0] = copy(bufs[0][offset:], p)
		return 1, nil
	case <-t.stop:
		return 0, io.EOF
	}
}
func (t *memoryTUN) Write(bufs [][]byte, offset int) (int, error) {
	for _, b := range bufs {
		select {
		case t.out <- bytes.Clone(b[offset:]):
		case <-t.stop:
			return 0, io.EOF
		}
	}
	return len(bufs), nil
}
func must(e error) {
	if e != nil {
		panic(e)
	}
}
func key() (string, string) {
	var k [32]byte
	_, e := rand.Read(k[:])
	must(e)
	k[0] &= 248
	k[31] &= 127
	k[31] |= 64
	p, e := curve25519.X25519(k[:], curve25519.Basepoint)
	must(e)
	return hex.EncodeToString(k[:]), hex.EncodeToString(p)
}
func packet(src, dst byte, body []byte) []byte {
	p := make([]byte, 28+len(body))
	p[0] = 0x45
	binary.BigEndian.PutUint16(p[2:], uint16(len(p)))
	p[8] = 64
	p[9] = 17
	copy(p[12:], []byte{10, 254, 63, src})
	copy(p[16:], []byte{10, 254, 63, dst})
	binary.BigEndian.PutUint16(p[20:], 13000)
	binary.BigEndian.PutUint16(p[22:], 14000)
	binary.BigEndian.PutUint16(p[24:], uint16(8+len(body)))
	copy(p[28:], body)
	var sum uint32
	for i := 0; i < 20; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(p[i:]))
	}
	for sum > 65535 {
		sum = sum>>16 + sum&65535
	}
	binary.BigEndian.PutUint16(p[10:], ^uint16(sum))
	return p
}
func main() {
	out, err := filepath.Abs(os.Args[1])
	must(err)
	must(os.Mkdir(out, 0700))
	marker, err := net.ListenPacket("udp", "127.0.0.1:0")
	must(err)
	markerPort := marker.LocalAddr().(*net.UDPAddr).Port
	capture := exec.Command(os.Args[2], "-n", "-i", `\Device\NPF_Loopback`, "-f", fmt.Sprintf("udp port 51820 or udp port 53123 or udp port %d", markerPort), "-F", "pcap", "-w", filepath.Join(out, "live.pcap"), "-a", "duration:10", "-l", "-P", "-T", "fields", "-e", "frame.number")
	capture.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	var log bytes.Buffer
	capture.Stderr = &log
	stdout, err := capture.StdoutPipe()
	must(err)
	must(capture.Start())
	ready := make(chan error, 1)
	go func() {
		_, err := bufio.NewReader(stdout).ReadString('\n')
		ready <- err
		_, _ = io.Copy(io.Discard, stdout)
	}()
	deadline := time.Now().Add(8 * time.Second)
	for {
		_, err = marker.WriteTo([]byte("capture-ready-wireguard"), marker.LocalAddr())
		must(err)
		select {
		case err = <-ready:
			must(err)
			goto captured
		case <-time.After(100 * time.Millisecond):
			if time.Now().After(deadline) {
				_ = capture.Process.Kill()
				panic("Npcap readiness timeout")
			}
		}
	}
captured:
	must(marker.Close())
	a, b := newTUN(), newTUN()
	d1 := device.NewDevice(a, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "audit-client: "))
	d2 := device.NewDevice(b, conn.NewDefaultBind(), device.NewLogger(device.LogLevelError, "audit-server: "))
	defer d1.Close()
	defer d2.Close()
	k1, p1 := key()
	k2, p2 := key()
	must(d1.IpcSet(fmt.Sprintf("private_key=%s\nlisten_port=53123\npublic_key=%s\nallowed_ip=10.254.63.2/32\nendpoint=127.0.0.1:51820\n", k1, p2)))
	must(d2.IpcSet(fmt.Sprintf("private_key=%s\nlisten_port=51820\npublic_key=%s\nallowed_ip=10.254.63.1/32\nendpoint=127.0.0.1:53123\n", k2, p1)))
	must(d1.Up())
	must(d2.Up())
	var operations []map[string]any
	for i := 0; i < 8; i++ {
		for _, rev := range []bool{false, true} {
			src, dst := byte(1), byte(2)
			from, to := a, b
			if rev {
				src, dst, from, to = 2, 1, b, a
			}
			// Include inner packets at and immediately below the 1420-byte MTU.
			// wireguard-go clips padding to MTU, so valid outer lengths need not
			// be multiples of 16 even though smaller packets normally are.
			nonce := make([]byte, []int{0, 1, 700, 1390, 1391, 1392, 1391, 2}[i])
			_, err := rand.Read(nonce)
			must(err)
			wire := packet(src, dst, nonce)
			from.in <- wire
			select {
			case got := <-to.out:
				if !bytes.Equal(got, wire) {
					panic("independent decrypted TUN packet mismatch")
				}
				operations = append(operations, map[string]any{"direction": rev, "bytes": len(got), "sha256": fmt.Sprintf("%x", sha256.Sum256(got))})
			case <-time.After(4 * time.Second):
				panic("WireGuard delivery timeout")
			}
		}
	}
	must(capture.Wait())
	must(os.WriteFile(filepath.Join(out, "capture.stderr"), log.Bytes(), 0600))
	data, err := json.MarshalIndent(map[string]any{"implementation": "wireguard-go v0.0.0-20231211153847-12269c276173", "marker_port": markerPort, "ports": []int{51820, 53123}, "system_tun_or_routes_modified": false, "operations": operations}, "", "  ")
	must(err)
	must(os.WriteFile(filepath.Join(out, "manifest.json"), data, 0600))
	fmt.Println("PASS 16 real WireGuard encrypted/decrypted packets through memory-only TUN endpoints")
}
