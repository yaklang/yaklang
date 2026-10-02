package stream_parser

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func sshPlaintextTestString(b []byte) []byte {
	w := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(w, uint32(len(b)))
	copy(w[4:], b)
	return w
}

func sshPlaintextTestPacket(payload ...[]byte) []byte {
	p := bytes.Join(payload, nil)
	pad := 8 - (5+len(p))%8
	if pad < 4 {
		pad += 8
	}
	w := make([]byte, 5+len(p)+pad)
	binary.BigEndian.PutUint32(w, uint32(len(w)-4))
	w[4] = byte(pad)
	copy(w[5:], p)
	return w
}

func sshPlaintextTestCases() []struct {
	profile string
	wire    []byte
} {
	s := sshPlaintextTestString
	mp := s([]byte{0, 0x81})
	host := s(bytes.Join([][]byte{s([]byte("ssh-rsa")), s([]byte{1, 0, 1}), mp}, nil))
	sig := s(append(s([]byte("rsa-sha2-512")), s([]byte{0, 0x31, 0x82})...))
	p := s(append([]byte{4}, bytes.Repeat([]byte{0x12}, 64)...))
	kex := append([]byte{20}, make([]byte, 16)...)
	for i := 0; i < 10; i++ {
		if i < 8 {
			kex = append(kex, s([]byte("a,b@example.test"))...)
		} else {
			kex = append(kex, s(nil)...)
		}
	}
	kex = append(kex, 255, 1, 2, 3, 4) // any nonzero boolean, ignored reserved value
	return []struct {
		profile string
		wire    []byte
	}{
		{"transport", sshPlaintextTestPacket(kex)}, {"transport", sshPlaintextTestPacket([]byte{21})},
		{"dh", sshPlaintextTestPacket([]byte{30}, mp)}, {"dh", sshPlaintextTestPacket([]byte{31}, host, mp, sig)},
		{"dh-gex", sshPlaintextTestPacket([]byte{34, 0, 0, 4, 0, 0, 0, 8, 0, 0, 0, 16, 0})},
		{"dh-gex", sshPlaintextTestPacket([]byte{30, 0, 0, 8, 0})},
		{"dh-gex", sshPlaintextTestPacket([]byte{31}, mp, s([]byte{2}))},
		{"dh-gex", sshPlaintextTestPacket([]byte{32}, mp)},
		{"dh-gex", sshPlaintextTestPacket([]byte{33}, host, mp, sig)},
		{"ecdh-nistp256", sshPlaintextTestPacket([]byte{30}, p)},
		{"ecdh-nistp256", sshPlaintextTestPacket([]byte{31}, host, p, sig)},
	}
}

func TestSSHPlaintextLayoutsAndBoundaries(t *testing.T) {
	for i, tc := range sshPlaintextTestCases() {
		t.Run(fmt.Sprintf("%d/%s", i, tc.profile), func(t *testing.T) {
			fields, info, err := decodeSSHPlaintextPacket(tc.wire, tc.profile)
			require.NoError(t, err)
			tlsCertificateTestCoverage(t, fields, len(tc.wire))
			require.Equal(t, true, info["Payload Layout Decoded"])
			for cut := 0; cut < len(tc.wire); cut++ {
				_, _, err := decodeSSHPlaintextPacket(tc.wire[:cut], tc.profile)
				require.Error(t, err, "prefix %d", cut)
			}
			_, _, err = decodeSSHPlaintextPacket(append(bytes.Clone(tc.wire), tc.wire...), tc.profile)
			require.Error(t, err)
			for _, pad := range []byte{0, 1, 2, 3, 255} {
				w := bytes.Clone(tc.wire)
				w[4] = pad
				_, _, err := decodeSSHPlaintextPacket(w, tc.profile)
				require.Error(t, err)
			}
			var mutateLengths func([]tlsCertificateField)
			mutateLengths = func(fields []tlsCertificateField) {
				for _, f := range fields {
					if f.Type == "uint32" && strings.HasSuffix(f.Name, " Length") {
						w := bytes.Clone(tc.wire)
						copy(w[f.Start:f.End], []byte{255, 255, 255, 255})
						_, _, err := decodeSSHPlaintextPacket(w, tc.profile)
						require.Error(t, err, f.Name)
					}
					mutateLengths(f.Children)
				}
			}
			mutateLengths(fields)
			payload := tc.wire[5 : len(tc.wire)-int(tc.wire[4])]
			_, _, err = decodeSSHPlaintextPacket(sshPlaintextTestPacket(payload, []byte{0}), tc.profile)
			require.Error(t, err, "selected grammar trailing bytes")
		})
	}
	_, _, err := decodeSSHPlaintextPacket(sshPlaintextTestPacket([]byte{21}), "guess")
	require.Error(t, err)
	for _, size := range []int{34992, 35000, 35008} {
		w := make([]byte, size)
		binary.BigEndian.PutUint32(w, uint32(size-4))
		w[4] = 4
		w[5] = 99
		fields, info, err := decodeSSHPlaintextPacket(w, "transport")
		if size > 35000 {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, false, info["Payload Layout Decoded"])
			tlsCertificateTestCoverage(t, fields, size)
		}
	}
	// Maximum padding is valid when its surrounding packet remains aligned.
	w := make([]byte, 264)
	binary.BigEndian.PutUint32(w, 260)
	w[4] = 255
	w[5] = 99
	_, _, err = decodeSSHPlaintextPacket(w, "transport")
	require.NoError(t, err)
}

func TestSSHPlaintextMPIntsPointsAndOpaqueFormats(t *testing.T) {
	s := sshPlaintextTestString
	for _, b := range [][]byte{nil, {1}, {0, 0x80}, {0x7f}, {1, 0}} {
		f, _, err := decodeSSHPlaintextPacket(sshPlaintextTestPacket([]byte{30}, s(b)), "dh")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, f, len(sshPlaintextTestPacket([]byte{30}, s(b))))
	}
	for _, b := range [][]byte{{0}, {0, 1}, {0, 0x7f}, {0x80}, {0xff}, {0xff, 0x81}} {
		_, _, err := decodeSSHPlaintextPacket(sshPlaintextTestPacket([]byte{30}, s(b)), "dh")
		require.Error(t, err)
	}
	for _, format := range []byte{0, 1, 2, 3, 4, 5, 6, 7, 255} {
		for _, size := range []int{0, 1, 32, 33, 34, 64, 65, 66} {
			point := make([]byte, size)
			if size > 0 {
				point[0] = format
			}
			f, _, err := decodeSSHPlaintextPacket(sshPlaintextTestPacket([]byte{30}, s(point)), "ecdh-nistp256")
			if format == 4 && size == 65 || (format == 2 || format == 3) && size == 33 {
				require.NoError(t, err)
				require.NotEmpty(t, f) // zero coordinates do not imply a valid point
			} else {
				require.Error(t, err)
			}
		}
	}
	host := s(append(s([]byte("custom-key@example.test")), []byte{1, 2, 3}...))
	sig := s(append(s([]byte("custom-signature@example.test")), []byte{255, 0, 1}...))
	w := sshPlaintextTestPacket([]byte{31}, host, s([]byte{2}), sig)
	f, info, err := decodeSSHPlaintextPacket(w, "dh")
	require.NoError(t, err)
	tlsCertificateTestCoverage(t, f, len(w))
	require.Equal(t, false, info["Host Key Fields Decoded"])
	require.Equal(t, false, info["Signature Fields Decoded"])
	// Method numbers alone must not select DH, GEX, or ECDH.
	for _, tc := range sshPlaintextTestCases()[2:] {
		f, info, err := decodeSSHPlaintextPacket(tc.wire, "transport")
		require.NoError(t, err)
		require.Equal(t, false, info["Payload Layout Decoded"])
		tlsCertificateTestCoverage(t, f, len(tc.wire))
	}
	// A format identifier is not permission to reinterpret nested trailing bytes.
	rsa := s(bytes.Join([][]byte{s([]byte("ssh-rsa")), s([]byte{3}), s([]byte{5}), {0}}, nil))
	_, _, err = decodeSSHPlaintextPacket(sshPlaintextTestPacket([]byte{31}, rsa, s([]byte{2}), sig), "dh")
	require.Error(t, err)
}

func TestSSHPlaintextNameLists(t *testing.T) {
	valid := sshPlaintextTestCases()[0].wire
	firstEnd := 26 + int(binary.BigEndian.Uint32(valid[22:26]))
	tail := valid[firstEnd : len(valid)-int(valid[4])]
	for _, bad := range [][]byte{nil, []byte(",a"), []byte("a,"), []byte("a,,b"), []byte("a b"), {0}, {128}, bytes.Repeat([]byte{'a'}, 65)} {
		w := sshPlaintextTestPacket(valid[5:22], sshPlaintextTestString(bad), tail)
		_, _, err := decodeSSHPlaintextPacket(w, "transport")
		require.Error(t, err)
	}
	for _, names := range [][]byte{[]byte("a"), bytes.Repeat([]byte{'a'}, 64), []byte("a,b@sample.example")} {
		w := sshPlaintextTestPacket(valid[5:22], sshPlaintextTestString(names), tail)
		f, _, err := decodeSSHPlaintextPacket(w, "transport")
		require.NoError(t, err)
		tlsCertificateTestCoverage(t, f, len(w))
	}
}

func TestSSHPlaintextBridgeTransactions(t *testing.T) {
	for _, profile := range []string{"transport", "dh", "dh-gex", "ecdh-nistp256"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
				t.Run(fmt.Sprintf("%s/%d/%s", profile, offset, outcome), func(t *testing.T) {
					input := sshPlaintextTestPacket([]byte{21})
					inputBits := uint64(len(input)) * 8
					if outcome == "invalid" {
						input[5] = 20
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
					err := parseSSHPlaintextPacket(n, func(raw *base.Node) (func(bool), error) {
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
	for _, bits := range []uint64{0, 1, 7, 8, 127, 129, 35001 * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseSSHPlaintextPacket(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil }, "dh")
		require.Error(t, err)
	}
}
