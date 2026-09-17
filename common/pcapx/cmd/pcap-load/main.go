// pcap-load generates bounded traffic exclusively on localhost. Run it as a
// separate process so its HTTP/TLS and sender CPU is not charged to pcap-inspect.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"time"
)

type totals struct{ bytes, operations, failures atomic.Uint64 }

func publish(size int) []byte {
	n := size + 3
	w := []byte{0x30}
	for {
		v := byte(n % 128)
		n /= 128
		if n > 0 {
			v |= 128
		}
		w = append(w, v)
		if n == 0 {
			break
		}
	}
	w = append(w, 0, 1, 'a')
	return append(w, bytes.Repeat([]byte{'x'}, size)...)
}
func mqttFrame(c net.Conn) (byte, int, error) {
	var one [1]byte
	if _, err := io.ReadFull(c, one[:]); err != nil {
		return 0, 0, err
	}
	typ := one[0]
	n, m := 0, 1
	for i := 0; i < 4; i++ {
		if _, err := io.ReadFull(c, one[:]); err != nil {
			return 0, 0, err
		}
		n += int(one[0]&127) * m
		if one[0]&128 == 0 {
			if n > 2<<20 {
				return 0, 0, fmt.Errorf("MQTT frame too large")
			}
			_, err := io.CopyN(io.Discard, c, int64(n))
			return typ, n, err
		}
		m *= 128
	}
	return 0, 0, fmt.Errorf("invalid MQTT remaining length")
}
func writeAll(c net.Conn, w []byte) error {
	for len(w) > 0 {
		n, err := c.Write(w)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		w = w[n:]
	}
	return nil
}
func run(ctx context.Context, args []string, out io.Writer) error {
	f := flag.NewFlagSet("pcap-load", flag.ContinueOnError)
	f.SetOutput(out)
	duration := f.Duration("duration", 10*time.Second, "load duration, max 10 minutes")
	delay := f.Duration("delay", 2*time.Second, "wait before generating traffic")
	rate := f.Float64("rate", 200, "aggregate application-body Mbps; 0 sends as fast as possible")
	size := f.Int("body", 4096, "HTTP/MQTT body bytes")
	flows := f.Int("flows", 24, "concurrent local clients, evenly split HTTP/TLS/MQTT")
	port := f.Int("port", 18080, "three consecutive localhost ports")
	reconnect := f.Int("reconnect", 128, "requests/messages per connection; 0 keeps connections")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || *duration <= 0 || *duration > 10*time.Minute || *delay < 0 || *delay > time.Minute || math.IsNaN(*rate) || *rate < 0 || *rate > 10000 || *size < 1 || *size > 1<<20 || *flows < 3 || *flows > 256 || *port < 1024 || *port > 65533 || *reconnect < 0 {
		return fmt.Errorf("invalid load limits")
	}
	body := bytes.Repeat([]byte{'x'}, *size)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		w.Write(body)
	})
	servers := make([]*httptest.Server, 2)
	for i := range servers {
		listener, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port+i))
		if err != nil {
			return err
		}
		server := httptest.NewUnstartedServer(handler)
		server.Listener.Close()
		server.Listener = listener
		if i == 1 {
			server.StartTLS()
		} else {
			server.Start()
		}
		servers[i] = server
		defer server.Close()
	}
	broker, err := net.Listen("tcp4", fmt.Sprintf("127.0.0.1:%d", *port+2))
	if err != nil {
		return err
	}
	defer broker.Close()
	var serverWG sync.WaitGroup
	var serverBytes, serverOperations atomic.Uint64
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		for {
			c, err := broker.Accept()
			if err != nil {
				return
			}
			serverWG.Add(1)
			go func() {
				defer serverWG.Done()
				defer c.Close()
				c.SetDeadline(time.Now().Add(*delay + *duration + 5*time.Second))
				typ, _, err := mqttFrame(c)
				if err != nil || typ != 0x10 {
					return
				}
				if err = writeAll(c, []byte{0x20, 2, 0, 0}); err != nil {
					return
				}
				for {
					typ, n, err := mqttFrame(c)
					if err != nil {
						return
					}
					if typ != 0x30 || n < 3 {
						return
					}
					serverBytes.Add(uint64(n - 3))
					serverOperations.Add(1)
				}
			}()
		}
	}()
	defer func() { broker.Close(); <-serverDone; serverWG.Wait() }()
	fmt.Fprintf(out, "localhost HTTP=%d TLS=%d MQTT=%d; BPF: tcp portrange %d-%d\n", *port, *port+1, *port+2, *port, *port+2)
	timer := time.NewTimer(*delay)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
	}
	start := time.Now()
	end := start.Add(*duration)
	var count totals
	var sentMQTT atomic.Uint64
	var wg sync.WaitGroup
	var firstErr atomic.Pointer[string]
	fail := func(err error) { count.failures.Add(1); s := err.Error(); firstErr.CompareAndSwap(nil, &s) }
	for index := 0; index < *flows; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			kind := index % 3
			next := start
			wait := func(n int) bool {
				if *rate == 0 {
					return ctx.Err() == nil
				}
				next = next.Add(time.Duration(float64(n) * 8 * float64(*flows) / (*rate * 1e6) * float64(time.Second)))
				if sleep := time.Until(next); sleep > 0 {
					timer := time.NewTimer(sleep)
					select {
					case <-ctx.Done():
						timer.Stop()
						return false
					case <-timer.C:
					}
				}
				return ctx.Err() == nil
			}
			if kind < 2 {
				client := servers[kind].Client()
				transport := client.Transport.(*http.Transport).Clone()
				transport.MaxIdleConnsPerHost = 1
				transport.MaxConnsPerHost = 1
				client = &http.Client{Transport: transport, Timeout: 3 * time.Second}
				defer transport.CloseIdleConnections()
				for i := 0; time.Now().Before(end) && ctx.Err() == nil; i++ {
					req, err := http.NewRequest(http.MethodPost, servers[kind].URL+"/pcap-load", bytes.NewReader(body))
					if err != nil {
						fail(err)
						return
					}
					req.Close = *reconnect > 0 && (i+1)%*reconnect == 0
					rsp, err := client.Do(req)
					if err != nil {
						fail(err)
						return
					}
					n, err := io.Copy(io.Discard, rsp.Body)
					rsp.Body.Close()
					if err != nil || n != int64(*size) {
						fail(fmt.Errorf("HTTP read: bytes=%d error=%v", n, err))
						return
					}
					count.bytes.Add(uint64(*size * 2))
					count.operations.Add(1)
					if !wait(*size * 2) {
						return
					}
				}
			} else {
				wire := publish(*size)
				var c net.Conn
				defer func() {
					if c != nil {
						c.Close()
					}
				}()
				for i := 0; time.Now().Before(end) && ctx.Err() == nil; i++ {
					if c == nil {
						var err error
						c, err = net.DialTimeout("tcp4", broker.Addr().String(), time.Second)
						if err != nil {
							fail(err)
							return
						}
						c.SetDeadline(end.Add(3 * time.Second))
						connect := []byte{0x10, 14, 0, 4, 'M', 'Q', 'T', 'T', 4, 2, 0, 60, 0, 2, 'i', 'd'}
						if err = writeAll(c, connect); err != nil {
							fail(err)
							return
						}
						var ack [4]byte
						if _, err = io.ReadFull(c, ack[:]); err != nil || ack != [4]byte{0x20, 2, 0, 0} {
							fail(fmt.Errorf("MQTT connect ack: %v", err))
							return
						}
					}
					if err = writeAll(c, wire); err != nil {
						fail(err)
						return
					}
					count.bytes.Add(uint64(*size))
					sentMQTT.Add(1)
					count.operations.Add(1)
					if *reconnect > 0 && (i+1)%*reconnect == 0 {
						c.Close()
						c = nil
					}
					if !wait(*size) {
						return
					}
				}
			}
		}(index)
	}
	wg.Wait()
	broker.Close()
	<-serverDone
	serverWG.Wait()
	if sentMQTT.Load() != serverOperations.Load() || sentMQTT.Load()*uint64(*size) != serverBytes.Load() {
		fail(fmt.Errorf("MQTT receiver mismatch: sent=%d received=%d body_bytes=%d", sentMQTT.Load(), serverOperations.Load(), serverBytes.Load()))
	}
	elapsed := time.Since(start).Seconds()
	result := map[string]any{"elapsed_seconds": elapsed, "application_bytes": count.bytes.Load(), "operations": count.operations.Load(), "application_Mbps": float64(count.bytes.Load()) * 8 / elapsed / 1e6, "errors": count.failures.Load(), "mqtt_server_body_bytes": serverBytes.Load(), "mqtt_server_messages": serverOperations.Load(), "flows": *flows, "body": *size, "reconnect": *reconnect}
	if e := firstErr.Load(); e != nil {
		result["error"] = *e
	}
	if err := json.NewEncoder(out).Encode(result); err != nil {
		return err
	}
	if e := firstErr.Load(); e != nil {
		return fmt.Errorf("local load failed: %s", *e)
	}
	return nil
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := run(ctx, os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
