package stream_parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
	yaml "github.com/yaklang/yaklang/common/utils/orderedyaml"
)

func TestMinioS3BridgeTransactions(t *testing.T) {
	valid := []byte(minioS3NativeTestWire())
	for offset := uint64(0); offset < 8; offset++ {
		for _, outcome := range []string{"commit", "invalid", "short", "callback"} {
			t.Run(fmt.Sprintf("%d/%s", offset, outcome), func(t *testing.T) {
				input := bytes.Clone(valid)
				if outcome == "invalid" {
					input[0] = '!'
				} else if outcome == "short" {
					input = input[:len(input)-1]
				}
				root := giopBridgeInlineRoot(t, "endian: big\nunit: byte\nPackage:\n  Message: {}\n")
				n := root.Children[0].Children[0]
				n.Cfg.SetItem(CfgIsTerminal, false)
				require.NoError(t, root.Parse(base.NewBitReader(bytes.NewReader(nil))))
				root.Cfg.SetItem(CfgLength, offset+uint64(len(valid))*8)
				p := &DefParser{}
				require.NoError(t, p.OnRoot(root))
				n.Cfg.SetItem(CfgLength, uint64(len(valid))*8)
				n.Cfg.SetItem("additionInfo", map[string]any{"caller": true})
				alias := &base.Node{Name: "existing", Origin: yaml.MapSlice{}, Cfg: base.NewConfig(n.Cfg), Ctx: n.Ctx}
				alias.Cfg.SetItem(CfgParent, n)
				n.Children = []*base.Node{alias}
				cfg, ctx := n.Cfg, n.Ctx
				var wire bytes.Buffer
				w := base.NewBitWriter(&wire)
				if offset != 0 {
					require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					_, err := p.write([]byte{0x55}, offset)
					require.NoError(t, err)
				}
				require.NoError(t, w.WriteBits(input, uint64(len(input))*8))
				if offset != 0 {
					require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
				}
				r := base.NewBitReader(bytes.NewReader(wire.Bytes()))
				if offset != 0 {
					_, err := r.ReadBits(offset)
					require.NoError(t, err)
				}
				writer, buffer := ctx.GetItem("writer").(*base.BitWriter), ctx.GetItem("buffer").(*bytes.Buffer)
				before, beforeBuffer := writer.Snapshot(), bytes.Clone(buffer.Bytes())
				calls, finishes := 0, 0
				err := parseS3SignatureV4Request(n, func(raw *base.Node) (func(bool), error) {
					calls++
					require.NotContains(t, n.Children, raw)
					require.Same(t, n, raw.Cfg.GetItem(CfgParent))
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
				})
				require.Equal(t, 1, calls)
				require.Equal(t, 1, finishes)
				require.Same(t, cfg, n.Cfg)
				require.Same(t, ctx, n.Ctx)
				if outcome == "commit" {
					require.NoError(t, err)
					giopBridgeAssertTree(t, n, offset, offset+uint64(len(valid))*8)
					require.Equal(t, offset+uint64(len(valid))*8, ctx.GetUint64("pointer"))
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
	for _, bits := range []uint64{0, 1, 7, 9, 31, minioS3MaxBytes*8 + 1, (minioS3MaxBytes + 1) * 8} {
		root := giopBridgeInlineRoot(t, "Package:\n  Message: {}\n")
		n := root.Children[0].Children[0]
		n.Cfg.SetItem(CfgLength, bits)
		err := parseS3SignatureV4Request(n, func(*base.Node) (func(bool), error) { t.Fatal("invalid boundary read input"); return nil, nil })
		require.Error(t, err)
	}
}

func minioS3NativeTestWire() string {
	return "GET /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIA/20200101/us-east-1/s3/aws4_request,SignedHeaders=host;x-amz-date,Signature=" + strings.Repeat("0", 64) + "\r\nx-amz-date: 20200101T000000Z\r\nx-amz-content-sha256: UNSIGNED-PAYLOAD\r\n\r\n"
}

func TestMinioS3ValueBoundsAndIndependence(t *testing.T) {
	valid := minioS3NativeTestWire()
	for _, size := range []int{8192, 8193} {
		header := "X-Extra: " + strings.Repeat("x", size-len("X-Extra: \r\n")) + "\r\n"
		wire := []byte(strings.TrimSuffix(valid, "\r\n") + header + "\r\n")
		fields, info, err := decodeS3SignatureV4Request(wire)
		if size == 8192 {
			require.NoError(t, err)
			require.NotEmpty(t, fields)
			require.NotNil(t, info)
		} else {
			require.ErrorContains(t, err, "8192-byte line")
			require.Nil(t, fields)
			require.Nil(t, info)
		}
	}
	for _, size := range []int{256, 257} {
		for _, old := range []string{"AKIA", "us-east-1"} {
			wire := []byte(strings.Replace(valid, old, strings.Repeat("x", size), 1))
			_, _, err := decodeS3SignatureV4Request(wire)
			if size == 256 {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, "credential scope")
			}
		}
	}
	for _, count := range []int{256, 257} {
		names := make([]string, count)
		for i := range names {
			names[i] = fmt.Sprintf("h%03d", i)
		}
		auth := "AWS4-HMAC-SHA256 Credential=AKIA/20200101/local/s3/aws4_request,SignedHeaders=" + strings.Join(names, ";") + ",Signature=" + strings.Repeat("0", 64)
		_, _, err := minioS3Authorization(auth, 42)
		if count == 256 {
			require.NoError(t, err)
		} else {
			require.ErrorContains(t, err, "256-entry")
		}
	}
	for _, encoding := range []string{"aws-chunked", "gzip, AWS-CHUNKED", "gzip\r\nContent-Encoding: aws-chunked"} {
		_, _, err := decodeS3SignatureV4Request([]byte(strings.TrimSuffix(valid, "\r\n") + "Content-Encoding: " + encoding + "\r\n\r\n"))
		require.ErrorContains(t, err, "streaming/chunked")
	}
	_, _, err := decodeS3SignatureV4Request([]byte(strings.TrimSuffix(valid, "\r\n") + "Content-Encoding: opaque-aws-chunked-extension\r\n\r\n"))
	require.NoError(t, err) // an unknown coding is retained, not decoded or substring-matched
	for at := range []byte(valid) {
		for _, mask := range []byte{1, 0x80, 0xff} {
			wire := []byte(valid)
			wire[at] ^= mask
			one, info, err := decodeS3SignatureV4Request(wire)
			two, again, repeat := decodeS3SignatureV4Request(wire)
			require.Equal(t, err == nil, repeat == nil)
			require.Equal(t, one, two)
			require.Equal(t, info, again)
			if err != nil {
				require.Nil(t, one)
				require.Nil(t, info)
			}
		}
	}
	one, info, err := decodeS3SignatureV4Request([]byte(valid))
	require.NoError(t, err)
	one[0].Children[0].Name = "mutated"
	info["Signed Headers"].([]string)[0] = "mutated"
	two, again, err := decodeS3SignatureV4Request([]byte(valid))
	require.NoError(t, err)
	require.Equal(t, "Method", two[0].Children[0].Name)
	require.Equal(t, "host", again["Signed Headers"].([]string)[0])
}
