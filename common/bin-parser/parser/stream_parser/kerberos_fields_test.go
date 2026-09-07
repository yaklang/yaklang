package stream_parser

import (
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func krbTestTLV(tag byte, parts ...[]byte) []byte {
	var body []byte
	for _, p := range parts {
		body = append(body, p...)
	}
	w := []byte{tag}
	switch {
	case len(body) < 128:
		w = append(w, byte(len(body)))
	case len(body) < 256:
		w = append(w, 0x81, byte(len(body)))
	case len(body) < 65536:
		w = append(w, 0x82, byte(len(body)>>8), byte(len(body)))
	default:
		w = append(w, 0x83, byte(len(body)>>16), byte(len(body)>>8), byte(len(body)))
	}
	return append(w, body...)
}
func krbTestInt(v int64) []byte {
	w, err := asn1.Marshal(v)
	if err != nil {
		panic(err)
	}
	return w
}
func krbTestCtx(id byte, w []byte) []byte { return krbTestTLV(0xa0+id, w) }
func krbTestString(s string) []byte       { return krbTestTLV(27, []byte(s)) }
func krbTestTime() []byte                 { return krbTestTLV(24, []byte("20260701020304Z")) }
func krbTestPrincipal() []byte {
	return krbTestTLV(0x30, krbTestCtx(0, krbTestInt(2)), krbTestCtx(1, krbTestTLV(0x30, krbTestString("host"), krbTestString("sample.example"))))
}
func krbTestEncrypted() []byte {
	return krbTestTLV(0x30, krbTestCtx(0, krbTestInt(23)), krbTestCtx(1, krbTestInt(4294967295)), krbTestCtx(2, krbTestTLV(4, []byte{0x30, 0})))
}
func krbTestTicket() []byte {
	return krbTestTLV(0x61, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestString("EXAMPLE")), krbTestCtx(2, krbTestPrincipal()), krbTestCtx(3, krbTestEncrypted())))
}
func krbTestPA(typ int64, w []byte) []byte {
	return krbTestTLV(0x30, krbTestCtx(1, krbTestInt(typ)), krbTestCtx(2, krbTestTLV(4, w)))
}
func krbTestBody(etypes ...[]byte) []byte {
	if etypes == nil {
		etypes = [][]byte{krbTestInt(23), krbTestInt(-133), krbTestInt(-128), krbTestInt(3)}
	}
	return krbTestTLV(0x30, krbTestCtx(0, krbTestTLV(3, []byte{0, 0x40, 0x80, 0, 0})), krbTestCtx(1, krbTestPrincipal()), krbTestCtx(2, krbTestString("EXAMPLE")), krbTestCtx(3, krbTestPrincipal()), krbTestCtx(4, krbTestTime()), krbTestCtx(5, krbTestTime()), krbTestCtx(6, krbTestTime()), krbTestCtx(7, krbTestInt(4294967295)), krbTestCtx(8, krbTestTLV(0x30, etypes...)), krbTestCtx(9, krbTestTLV(0x30)), krbTestCtx(10, krbTestEncrypted()), krbTestCtx(11, krbTestTLV(0x30, krbTestTicket())))
}
func krbTestRequest(msg byte, body []byte, pa ...[]byte) []byte {
	return krbTestTLV(0x60|msg, krbTestTLV(0x30, krbTestCtx(1, krbTestInt(5)), krbTestCtx(2, krbTestInt(int64(msg))), krbTestCtx(3, krbTestTLV(0x30, pa...)), krbTestCtx(4, body)))
}
func krbTestAP(msg byte) []byte {
	f := [][]byte{krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(int64(msg)))}
	if msg == 14 {
		f = append(f, krbTestCtx(2, krbTestTLV(3, []byte{0, 0, 0, 0, 0})), krbTestCtx(3, krbTestTicket()), krbTestCtx(4, krbTestEncrypted()))
	} else {
		f = append(f, krbTestCtx(2, krbTestEncrypted()))
	}
	return krbTestTLV(0x60|msg, krbTestTLV(0x30, f...))
}
func krbTestReply(msg byte, pa ...[]byte) []byte {
	return krbTestTLV(0x60|msg, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(int64(msg))), krbTestCtx(2, krbTestTLV(0x30, pa...)), krbTestCtx(3, krbTestString("EXAMPLE")), krbTestCtx(4, krbTestPrincipal()), krbTestCtx(5, krbTestTicket()), krbTestCtx(6, krbTestEncrypted())))
}
func krbTestError(usec, code int64, extra ...[]byte) []byte {
	return krbTestTLV(0x7e, krbTestTLV(0x30, append([][]byte{krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(30)), krbTestCtx(2, krbTestTime()), krbTestCtx(3, krbTestInt(0)), krbTestCtx(4, krbTestTime()), krbTestCtx(5, krbTestInt(usec)), krbTestCtx(6, krbTestInt(code)), krbTestCtx(7, krbTestString("EXAMPLE")), krbTestCtx(8, krbTestPrincipal()), krbTestCtx(9, krbTestString("EXAMPLE")), krbTestCtx(10, krbTestPrincipal())}, extra...)...))
}
func krbTestTCP(w []byte) []byte {
	record := make([]byte, 4, len(w)+4)
	binary.BigEndian.PutUint32(record, uint32(len(w)))
	return append(record, w...)
}

func TestKerberosFieldsLayouts(t *testing.T) {
	checksum := krbTestTLV(0x30, krbTestCtx(0, krbTestInt(-138)), krbTestCtx(1, krbTestTLV(4, []byte{1, 2})))
	fastReq := krbTestTLV(0xa0, krbTestTLV(0x30, krbTestCtx(1, checksum), krbTestCtx(2, krbTestEncrypted())))
	fastAS := krbTestTLV(0xa0, krbTestTLV(0x30, krbTestCtx(0, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(1)), krbTestCtx(1, krbTestTLV(4, []byte{0})))), krbTestCtx(1, checksum), krbTestCtx(2, krbTestEncrypted())))
	fastRep := krbTestTLV(0xa0, krbTestTLV(0x30, krbTestCtx(0, krbTestEncrypted())))
	fixtures := map[string][]byte{
		"as-request":  krbTestRequest(10, krbTestBody(), krbTestPA(2, krbTestEncrypted()), krbTestPA(149, []byte{1, 2, 3})),
		"tgs-request": krbTestRequest(12, krbTestBody(), krbTestPA(1, krbTestAP(14)), krbTestPA(136, fastReq)),
		"as-fast":     krbTestRequest(10, krbTestBody(), krbTestPA(136, fastAS)),
		"as-reply":    krbTestReply(11, krbTestPA(136, fastRep)),
		"tgs-reply":   krbTestReply(13),
		"ap-request":  krbTestAP(14), "ap-reply": krbTestAP(15),
		"error":                krbTestError(999999, 52, krbTestCtx(11, krbTestString("sample response")), krbTestCtx(12, krbTestTLV(4, []byte{0xff}))),
		"method-data":          krbTestError(0, 25, krbTestCtx(12, krbTestTLV(4, krbTestTLV(0x30, krbTestPA(2, krbTestEncrypted()))))),
		"empty-optional-lists": krbTestRequest(12, krbTestBody()),
		"unknown-pa":           krbTestRequest(10, krbTestBody(), krbTestPA(-2147483648, []byte{0xff, 0x00})),
	}
	for name, wire := range fixtures {
		for _, tcp := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%t", name, tcp), func(t *testing.T) {
				w := wire
				if tcp {
					w = krbTestTCP(wire)
				}
				f, info, err := decodeKerberosFields(w, tcp)
				require.NoError(t, err)
				tlsCertificateTestCoverage(t, f, len(w))
				require.Equal(t, int64(wire[0]&31), info["Message Type"])
				for _, v := range info["Decoded Fields"].([]map[string]any) {
					if v["Kind"] == "Integer" {
						require.IsType(t, int64(0), v["Value"])
					}
				}
				if name == "unknown-pa" {
					require.Equal(t, false, info["PA-DATA Details"].([]map[string]any)[0]["Payload Layout Decoded"])
				}
				if name == "as-request" {
					require.Equal(t, false, info["PA-DATA Details"].([]map[string]any)[1]["Payload Layout Decoded"], "nonempty ignored PA-149 bytes are not decoded")
				}
				for cut := 0; cut < len(w); cut++ {
					_, _, err := decodeKerberosFields(w[:cut], tcp)
					require.Error(t, err)
				}
				_, _, err = decodeKerberosFields(append(bytes.Clone(w), 0), tcp)
				require.Error(t, err)
			})
		}
	}
	_, _, err := decodeKerberosFields(krbTestRequest(10, krbTestBody(), krbTestPA(136, fastReq)), false)
	require.Error(t, err, "AS request requires FAST armor")
	_, _, err = decodeKerberosFields(krbTestRequest(12, krbTestBody(), krbTestPA(1, krbTestAP(15))), false)
	require.Error(t, err)
	// Cipher bytes happen to be DER but must never be decoded as plaintext.
	_, info, err := decodeKerberosFields(krbTestAP(15), false)
	require.NoError(t, err)
	require.Equal(t, 2, info["Cipher Byte Count"])
	// The extensible FAST sequence keeps an unknown field as encoded bytes,
	// without labeling its inner integer as an understood protocol value.
	extension := krbTestCtx(3, krbTestInt(12345))
	fastExtended := krbTestTLV(0xa0, krbTestTLV(0x30, krbTestCtx(0, krbTestEncrypted()), extension))
	w := krbTestReply(13, krbTestPA(136, fastExtended))
	fields, info, err := decodeKerberosFields(w, false)
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, fields, len(w))
	require.Equal(t, 1, info["Unparsed Extension Count"])
	for _, v := range info["Decoded Fields"].([]map[string]any) {
		require.NotEqual(t, int64(12345), v["Value"])
	}
}

func TestKerberosFieldsPrimitiveAndStructuralBounds(t *testing.T) {
	for _, v := range []int64{-2147483649, -2147483648, -1, 0, 2147483647, 2147483648, 4294967295, 4294967296} {
		w := krbTestRequest(10, krbTestBody(krbTestInt(v)))
		_, _, err := decodeKerberosFields(w, false)
		if v >= -2147483648 && v <= 2147483647 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, v := range []int64{-1, 0, 999999, 1000000} {
		_, _, err := decodeKerberosFields(krbTestError(v, 52), false)
		if v >= 0 && v <= 999999 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, v := range []int64{-1, 0, 2147483648, 4294967295, 4294967296} {
		encrypted := krbTestTLV(0x30, krbTestCtx(0, krbTestInt(23)), krbTestCtx(1, krbTestInt(v)), krbTestCtx(2, krbTestTLV(4)))
		w := krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(15)), krbTestCtx(2, encrypted)))
		_, _, err := decodeKerberosFields(w, false)
		if v >= 0 && v <= 4294967295 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, bad := range [][]byte{
		{0x7e, 0x80, 0, 0}, // indefinite
		{0x7e, 0x81, 0},    // nonminimal length
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(4)), krbTestCtx(1, krbTestInt(15)), krbTestCtx(2, krbTestEncrypted()))),
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(14)), krbTestCtx(2, krbTestEncrypted()))),
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(15)))),
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(1, krbTestInt(15)), krbTestCtx(0, krbTestInt(5)), krbTestCtx(2, krbTestEncrypted()))),
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, krbTestInt(5)), krbTestCtx(1, krbTestInt(15)))),
		krbTestTLV(0x6f, krbTestTLV(0x30, krbTestCtx(0, []byte{2, 2, 0, 5}), krbTestCtx(1, krbTestInt(15)), krbTestCtx(2, krbTestEncrypted()))),
		krbTestRequest(10, krbTestBody(), krbTestPA(2, append(krbTestEncrypted(), 0))),
		krbTestRequest(10, krbTestBody([]byte{2, 0})),
		krbTestRequest(10, krbTestBody([]byte{2, 2, 0xff, 0xff})),
		krbTestRequest(10, krbTestBody([][]byte{}...)),
	} {
		_, _, err := decodeKerberosFields(bad, false)
		require.Error(t, err)
	}
	for _, length := range []int{31, 32, 33} {
		flag := append([]byte{byte((8 - length%8) % 8)}, make([]byte, (length+7)/8)...)
		body := krbTestTLV(0x30, krbTestCtx(0, krbTestTLV(3, flag)), krbTestCtx(2, krbTestString("EXAMPLE")), krbTestCtx(5, krbTestTime()), krbTestCtx(7, krbTestInt(0)), krbTestCtx(8, krbTestTLV(0x30, krbTestInt(23))))
		_, _, err := decodeKerberosFields(krbTestRequest(10, body), false)
		if length < 32 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
	for _, s := range []string{"20260230020304Z", "20260701020304.1Z", "20260701020304+0000"} {
		// Exercise the same date validator directly on independently built DER.
		w := krbTestTLV(24, []byte(s))
		r := &x509DERReader{wire: w}
		n, err := r.element(0, len(w), 0)
		require.NoError(t, err)
		d := &kerberosFieldsDecoder{r: r}
		require.Error(t, d.date(n))
	}
	for _, tcp := range []bool{false, true} {
		w := krbTestAP(15)
		if tcp {
			w = krbTestTCP(w)
			w[0] |= 128
		} else {
			w[0] = 0x60
		}
		_, _, err := decodeKerberosFields(w, tcp)
		require.Error(t, err)
	}
}

func TestKerberosFieldsResourceLimits(t *testing.T) {
	for _, count := range []int{1024, 1025} {
		items := make([][]byte, count)
		for i := range items {
			items[i] = krbTestInt(23)
		}
		_, _, err := decodeKerberosFields(krbTestRequest(10, krbTestBody(items...)), false)
		if count == 1024 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "list")
		}
	}
	for _, size := range []int{65536, 65537} {
		w := krbTestString(string(bytes.Repeat([]byte{'a'}, size)))
		r := &x509DERReader{wire: w}
		n, err := r.element(0, len(w), 0)
		require.NoError(t, err)
		d := &kerberosFieldsDecoder{r: r}
		err = d.string(n)
		if size == 65536 {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	w := krbTestString("é")
	r := &x509DERReader{wire: w}
	n, err := r.element(0, len(w), 0)
	require.NoError(t, err)
	require.Error(t, (&kerberosFieldsDecoder{r: r}).string(n))
	w = krbTestInt(1)
	for i := 0; i < 34; i++ {
		w = krbTestTLV(0x30, w)
	}
	_, _, err = decodeKerberosFields(w, false)
	require.ErrorContains(t, err, "depth/node")
	// A valid DER tree can exceed the shared node cap before schema work.
	w = krbTestTLV(0x6f, krbTestTLV(0x30, bytes.Repeat(krbTestInt(0), 16384)))
	_, _, err = decodeKerberosFields(w, false)
	require.ErrorContains(t, err, "depth/node")
	_, _, err = decodeKerberosFields(make([]byte, kerberosFieldsMaxBytes+1), false)
	require.Error(t, err)
	for _, tcp := range []bool{false, true} {
		build := func(size int) []byte {
			w := krbTestRequest(10, krbTestBody(), krbTestPA(999, bytes.Repeat([]byte{0xa5}, size)))
			if tcp {
				w = krbTestTCP(w)
			}
			return w
		}
		payloadSize := kerberosFieldsMaxBytes - 1024
		payloadSize += kerberosFieldsMaxBytes - len(build(payloadSize))
		w := build(payloadSize)
		require.Len(t, w, kerberosFieldsMaxBytes)
		fields, info, err := decodeKerberosFields(w, tcp)
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, fields, len(w))
		require.Equal(t, false, info["PA-DATA Details"].([]map[string]any)[0]["Payload Layout Decoded"])
		_, _, err = decodeKerberosFields(build(payloadSize+1), tcp)
		require.Error(t, err)
	}
}

func TestKerberosFieldsBridgeTransactions(t *testing.T) {
	testExactByteFieldsBridgeTransactions(t, []string{"message", "tcp"}, func(profile string) []byte {
		w := krbTestAP(15)
		if profile == "tcp" {
			w = krbTestTCP(w)
		}
		return w
	}, func(n *base.Node, process func(*base.Node) (func(bool), error), profile string) error {
		return parseKerberosFields(n, process, profile == "tcp")
	})
}

// Shared exact-byte bridge contract, exercised with distinct protocol planners.
func testExactByteFieldsBridgeTransactions(t *testing.T, profiles []string, makeWire func(string) []byte, parseFields func(*base.Node, func(*base.Node) (func(bool), error), string) error) {
	t.Helper()
	for _, profile := range profiles {
		for offset := uint64(0); offset < 8; offset++ {
			for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
				t.Run(fmt.Sprintf("%s/%d/%s", profile, offset, outcome), func(t *testing.T) {
					input := makeWire(profile)
					inputBits := uint64(len(input)) * 8
					if outcome == "invalid" {
						input[0] = 0xff
					} else if outcome == "short" {
						input = input[:len(input)-1]
					}
					root := giopBridgeInlineRoot(t, "endian: little\nunit: byte\nPackage:\n  Message: {}\n")
					n := root.Children[0].Children[0]
					n.Cfg.SetItem(CfgIsTerminal, false)
					require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
					root.Cfg.SetItem(CfgLength, offset+inputBits)
					p := &DefParser{}
					require.NoError(t, p.OnRoot(root))
					n.Cfg.SetItem(CfgLength, inputBits)
					n.Cfg.SetItem("additionInfo", map[string]any{"caller": true})
					alias := &base.Node{Name: "existing", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
					alias.Cfg.SetItem(CfgParent, n)
					n.Children = []*base.Node{alias}
					cfg, ctx := n.Cfg, n.Ctx
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
						_, err := p.write([]byte{0x55}, offset)
						require.NoError(t, err)
					}
					require.NoError(t, w.WriteBits(input, uint64(len(input))*8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					if offset > 0 {
						_, err := r.ReadBits(offset)
						require.NoError(t, err)
					}
					writer, buffer := ctx.GetItem("writer").(*base.BitWriter), ctx.GetItem("buffer").(*bytes.Buffer)
					before, beforeBuffer := writer.Snapshot(), bytes.Clone(buffer.Bytes())
					calls, finishes := 0, 0
					err := parseFields(n, func(raw *base.Node) (func(bool), error) {
						calls++
						require.NotContains(t, n.Children, raw)
						require.NoError(t, r.Backup())
						position, state := ctx.GetUint64("pointer"), writer.Snapshot()
						err := p.Parse(r, raw)
						if outcome == "callback" {
							err = errors.New("injected callback failure")
						}
						return func(rollback bool) {
							finishes++
							if rollback {
								ctx.SetItem("pointer", position)
								buffer.Truncate(int(position / 8))
								require.NoError(t, writer.Restore(state))
								require.NoError(t, r.Recovery())
							} else {
								require.NoError(t, r.PopBackup())
							}
						}, err
					}, profile)
					require.Equal(t, 1, calls)
					require.Equal(t, 1, finishes)
					require.Same(t, cfg, n.Cfg)
					require.Same(t, ctx, n.Ctx)
					if outcome == "commit" {
						require.NoError(t, err)
						giopBridgeAssertTree(t, n, offset, offset+inputBits)
						require.Equal(t, offset+inputBits, ctx.GetUint64("pointer"))
					} else {
						require.Error(t, err)
						require.Len(t, n.Children, 1)
						require.Same(t, alias, n.Children[0])
						require.Equal(t, map[string]any{"caller": true}, n.Cfg.GetItem("additionInfo"))
						require.Equal(t, offset, ctx.GetUint64("pointer"))
						require.Equal(t, before, writer.Snapshot())
						require.True(t, bytes.Equal(beforeBuffer, buffer.Bytes()))
						got, err := r.ReadBits(uint64(len(input)) * 8)
						require.NoError(t, err)
						require.Equal(t, input, got)
					}
					require.ErrorContains(t, r.PopBackup(), "no backup")
				})
			}
		}
	}
}
