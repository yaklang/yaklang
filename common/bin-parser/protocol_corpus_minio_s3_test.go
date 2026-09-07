package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/bin-parser/parser"
	"github.com/yaklang/yaklang/common/bin-parser/parser/base"
)

const minioS3TestRule = "application-layer.minio_s3"
const minioS3TestEmptyHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
const minioS3TestSignature = "0000000000000000000000000000000000000000000000000000000000000000"

// Explicit structural examples, not executable requests. Except aws-get
// (AWS's public worked example), signatures are deliberately inert literals.
// Neither signature nor digest validity is an acceptance claim of this rule.
var minioS3TestFixtures = []struct{ name, wire string }{
	{"minimal-get", "GET /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIA/20200101/us-east-1/s3/aws4_request, SignedHeaders=host;x-amz-date, Signature=" + minioS3TestSignature + "\r\nx-amz-date: 20200101T000000Z\r\nx-amz-content-sha256: " + minioS3TestEmptyHash + "\r\n\r\n"},
	{"aws-get", "GET /test.txt HTTP/1.1\r\nHost: examplebucket.s3.amazonaws.com\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/20130524/us-east-1/s3/aws4_request,SignedHeaders=host;range;x-amz-content-sha256;x-amz-date,Signature=f0e8bdb87c964420e857bd35b5d6ed310bd44f0170aba48dd91039c6036bdb41\r\nRange: bytes=0-9\r\nx-amz-content-sha256: " + minioS3TestEmptyHash + "\r\nx-amz-date: 20130524T000000Z\r\n\r\n"},
	{"put-unsigned", "PUT /bucket/a//b%20c?partNumber=1&uploadId=x%2Fy HTTP/1.1\r\nHost: [2001:db8::1]:9000\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=content-type;host;x-amz-date;x-amz-security-token,Signature=" + minioS3TestSignature + "\r\nContent-Type: application/octet-stream\r\nContent-Length: 3\r\nx-amz-security-token: EXAMPLE-SESSION\r\nx-amz-date: 20200101T010203Z\r\nx-amz-content-sha256: UNSIGNED-PAYLOAD\r\n\r\nabc"},
	{"head-http-date", "HEAD /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=date;host,Signature=" + minioS3TestSignature + "\r\nDate: Wed, 01 Jan 2020 00:00:00 GMT\r\nx-amz-content-sha256: " + minioS3TestEmptyHash + "\r\nContent-Length: 0\r\n\r\n"},
	{"delete-whitespace", "DELETE /bucket/object?versionId=one&versionId=two HTTP/1.1\r\nhOsT:\tminio.local \t\r\nAuThOrIzAtIoN: AWS4-HMAC-SHA256  Signature = " + minioS3TestSignature + " ,\tCredential = example/20200101/local-region/s3/aws4_request , SignedHeaders = host;x-amz-date\r\nx-amz-date: 20200101T010203Z\r\nDate: overridden raw value\r\nx-amz-content-sha256: UNSIGNED-PAYLOAD\r\n\r\n"},
	{"repeated-extension", "GET /bucket/./object//?acl&prefix=a+b HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=example/20200101/us-east-1/s3/aws4_request,SignedHeaders=host;x-amz-date;x-amz-meta-color,Signature=" + minioS3TestSignature + "\r\nx-amz-meta-color: blue\r\nx-amz-meta-color: green\r\nx-amz-date: 20200101T000000Z\r\nx-amz-content-sha256: " + minioS3TestEmptyHash + "\r\n\r\n"},
}

func minioS3TestParse(t *testing.T, wire []byte, entry string) *base.Node {
	t.Helper()
	return protocolCorpusRequireBoundedRuleParse(t, wire, minioS3TestRule, entry)
}

func minioS3TestInfo(t *testing.T, n *base.Node) map[string]any {
	t.Helper()
	return gssapiTestInfo(t, n, "Signature Verified")
}

func minioS3TestTree(t *testing.T, n *base.Node, wire []byte, offset uint64) {
	t.Helper()
	method := protocolCorpusFindNode(n, "Method")
	require.NotNil(t, method)
	message := method.Cfg.GetItem(base.CfgParent).(*base.Node).Cfg.GetItem(base.CfgParent).(*base.Node)
	megacoTestTree(t, message, offset, offset+uint64(len(wire))*8)
	info := minioS3TestInfo(t, n)
	for _, name := range []string{"Signature Verified", "Payload Digest Verified", "Canonical Request Constructed", "Credential Validated", "Timestamp Freshness Validated", "Endpoint Product Identified", "Object Operation Outcome Validated"} {
		require.Equal(t, false, info[name], name)
	}
	require.Equal(t, "AWS4-HMAC-SHA256", info["Algorithm"])
	require.Equal(t, "s3", info["Service"])
}

func TestProtocolCorpusMinioS3Fields(t *testing.T) {
	for _, f := range minioS3TestFixtures {
		for _, entry := range []string{"S3SignatureV4Request", "S3SignatureV4Carrier"} {
			t.Run(f.name+"/"+entry, func(t *testing.T) {
				wire := []byte(f.wire)
				n := minioS3TestParse(t, wire, entry)
				require.Equal(t, wire, NodeToBytes(n))
				minioS3TestTree(t, n, wire, 0)
				parts := strings.SplitN(f.wire, " ", 3)
				latTestField(t, n, "Method", "string", 0, uint64(len(parts[0]))*8, parts[0])
				latTestField(t, n, "Request Target", "string", uint64(len(parts[0])+1)*8, uint64(len(parts[0])+1+len(parts[1]))*8, parts[1])
				algorithm := bytes.Index(wire, []byte("AWS4-HMAC-SHA256"))
				latTestField(t, n, "Algorithm", "string", uint64(algorithm)*8, uint64(algorithm+16)*8, "AWS4-HMAC-SHA256")
			})
		}
	}
	wire := []byte(minioS3TestFixtures[0].wire)
	n := minioS3TestParse(t, wire, "S3SignatureV4Request")
	for _, f := range []struct{ name, value string }{
		{"Access Key ID", "AKIA"}, {"Scope Date", "20200101"}, {"Region", "us-east-1"}, {"Service", "s3"}, {"Scope Terminator", "aws4_request"},
		{"Signed Header 0", "host"}, {"Signed Header 1", "x-amz-date"}, {"Signature Hex", minioS3TestSignature},
	} {
		anchor := bytes.Index(wire, []byte("Credential="))
		if strings.HasPrefix(f.name, "Signed Header") {
			anchor = bytes.Index(wire, []byte("SignedHeaders="))
		}
		if f.name == "Signature Hex" {
			anchor = bytes.Index(wire, []byte("Signature="))
		}
		at := anchor + bytes.Index(wire[anchor:], []byte(f.value))
		latTestField(t, n, f.name, "string", uint64(at)*8, uint64(at+len(f.value))*8, f.value)
	}
	n = minioS3TestParse(t, []byte(minioS3TestFixtures[2].wire), "S3SignatureV4Request")
	info := minioS3TestInfo(t, n)
	require.Equal(t, "/bucket/a//b%20c", info["Escaped Path"])
	require.Equal(t, "partNumber=1&uploadId=x%2Fy", info["Raw Query"])
	require.Equal(t, false, info["Payload Digest Declared"])
	require.Equal(t, 3, info["Payload Bytes"])
	protocolCorpusRequireValue(t, n, "Payload", []byte("abc"))
	n = minioS3TestParse(t, []byte(minioS3TestFixtures[3].wire), "S3SignatureV4Request")
	require.Equal(t, "date", minioS3TestInfo(t, n)["Timestamp Header"])
	n = minioS3TestParse(t, []byte(minioS3TestFixtures[4].wire), "S3SignatureV4Request")
	require.Equal(t, "2020-01-01T01:02:03Z", minioS3TestInfo(t, n)["Timestamp UTC"])
	n = minioS3TestParse(t, []byte(minioS3TestFixtures[5].wire), "S3SignatureV4Request")
	require.Equal(t, "/bucket/./object//", minioS3TestInfo(t, n)["Escaped Path"])
	require.Equal(t, "acl&prefix=a+b", minioS3TestInfo(t, n)["Raw Query"])
}

func minioS3TestReject(t *testing.T, wire []byte, diagnostic string) {
	t.Helper()
	_, err := parser.ParseBinary(newProtocolCorpusBoundedReader(wire), minioS3TestRule, "S3SignatureV4Request")
	require.ErrorContains(t, err, diagnostic)
	if len(wire) == 0 {
		return
	}
	n := minioS3TestParse(t, wire, "S3SignatureV4Carrier")
	require.Equal(t, wire, NodeToBytes(n))
	latTestField(t, n, "Unparsed S3 Signature V4 Request", "raw", 0, uint64(len(wire))*8, wire)
	require.Nil(t, protocolCorpusFindNode(n, "Access Key ID"))
	require.Nil(t, protocolCorpusFindNode(n, "Method"))
}

func TestProtocolCorpusMinioS3PrefixesAndNegatives(t *testing.T) {
	for _, f := range minioS3TestFixtures {
		for cut := 0; cut < len(f.wire); cut++ {
			_, err := parser.ParseBinary(newProtocolCorpusBoundedReader([]byte(f.wire[:cut])), minioS3TestRule, "S3SignatureV4Request")
			require.Error(t, err, "%s prefix %d", f.name, cut)
		}
	}
	valid := minioS3TestFixtures[0].wire
	for _, pair := range [][2]string{
		{"HTTP/1.1", "HTTP/1.0"}, {"GET ", "CONNECT "}, {"/bucket/object", "http://minio.local/bucket/object"},
		{"/bucket/object", "/bucket/%xx"}, {"/bucket/object", "/bucket/object#fragment"}, {"Host: minio.local", "Host : minio.local"},
		{"AWS4-HMAC-SHA256", "AWS4-ECDSA-P256-SHA256"}, {"SignedHeaders=host;x-amz-date, ", ""}, {"Signature=" + minioS3TestSignature, "Signature=abcd"},
		{"Signature=" + minioS3TestSignature, "Signature=A" + minioS3TestSignature[1:]}, {"Credential=AKIA", "Credential="},
		{"/20200101/", "/20200230/"}, {"/us-east-1/", "//"}, {"/s3/", "/ec2/"}, {"/aws4_request", "/aws4_other"},
		{"SignedHeaders=host;x-amz-date", "SignedHeaders=Host;x-amz-date"}, {"SignedHeaders=host;x-amz-date", "SignedHeaders=x-amz-date;host"},
		{"SignedHeaders=host;x-amz-date", "SignedHeaders=host;host;x-amz-date"}, {"SignedHeaders=host;x-amz-date", "SignedHeaders=host;x-missing"},
		{"SignedHeaders=host;x-amz-date", "SignedHeaders=host"}, {"SignedHeaders=host;x-amz-date", "SignedHeaders=x-amz-date"},
		{"Signature=", "Other="}, {"20200101T000000Z", "20200102T000000Z"}, {"20200101T000000Z", "20200101T250000Z"},
		{"x-amz-date: 20200101T000000Z\r\n", ""}, {"x-amz-content-sha256: " + minioS3TestEmptyHash, "x-amz-content-sha256: STREAMING-AWS4-HMAC-SHA256-PAYLOAD"},
		{"x-amz-content-sha256: " + minioS3TestEmptyHash, "x-amz-content-sha256: BAD"},
	} {
		minioS3TestReject(t, []byte(strings.Replace(valid, pair[0], pair[1], 1)), "s3-sigv4:")
	}
	for _, header := range []string{
		"Host: minio.local", "Authorization: AWS4-HMAC-SHA256 Credential=x", "x-amz-date: 20200101T000000Z",
		"x-amz-content-sha256: " + minioS3TestEmptyHash, "Transfer-Encoding: chunked", "Content-Encoding: aws-chunked",
		"Content-Length: 1", "Content-Length: +0", "Content-Length: 9999999999999999999999999", "Content-MD5: abc", "x-amz-meta-test: x",
	} {
		wire := strings.Replace(valid, "\r\n\r\n", "\r\n"+header+"\r\n\r\n", 1)
		minioS3TestReject(t, []byte(wire), "s3-sigv4:")
	}
	minioS3TestReject(t, []byte(valid+"x"), "body length")
	minioS3TestReject(t, []byte(strings.Replace(valid, "Signature=", "Credential=other,Signature=", 1)), "duplicate authorization")
	minioS3TestReject(t, []byte(strings.Replace(valid, "\r\nx-amz-date", ",\r\nx-amz-date", 1)), "trailing authorization comma")
	dateOnly := minioS3TestFixtures[3].wire
	dateOnly = strings.Replace(dateOnly, "SignedHeaders=date;host", "SignedHeaders=date;host;x-amz-date", 1)
	dateOnly = strings.Replace(dateOnly, "\r\n\r\n", "\r\nx-amz-date:\r\n\r\n", 1)
	minioS3TestReject(t, []byte(dateOnly), "invalid x-amz-date")
}

func TestProtocolCorpusMinioS3OriginalEveryRecord(t *testing.T) {
	for _, capture := range []struct{ path, sha string }{
		{"generated-local/gen-minio-s3.pcap", "2e0d12e372db798e7e39e2e92eabcfe798c0c5b7464844aa3941c183e47f7d16"},
		{"generated-pr5023/pr5023-gen-minio-s3.pcap", "3501b711d412f8b67a160eb2bf9415137854e67cbff7bc93fdee0f765f2a0ab8"},
	} {
		path := "testdata/protocol-corpus/captures/" + capture.path
		file, err := os.ReadFile(path)
		require.NoError(t, err)
		require.Equal(t, capture.sha, fmt.Sprintf("%x", sha256.Sum256(file)))
		frames := protocolCorpusAuditPackets(t, path)
		require.Len(t, frames, 4)
		for i, frame := range frames {
			require.Equal(t, byte(0x45), frame[14])
			require.Equal(t, byte(6), frame[23])
			require.Equal(t, byte(0x50), frame[46])
			require.Equal(t, len(frame)-14, int(binary.BigEndian.Uint16(frame[16:18])))
			payload := frame[54:]
			if i < 3 {
				require.Empty(t, payload)
				minioS3TestReject(t, payload, "boundary")
				continue
			}
			require.Equal(t, []byte("GET /bucket/object HTTP/1.1\r\nHost: minio.local\r\nAuthorization: AWS4-HMAC-SHA256 Credential=AKIA/20200101/us-east-1/s3/aws4_request\r\n\r\n"), payload)
			minioS3TestReject(t, payload, "missing authorization parameter SignedHeaders")
		}
	}
}

func TestProtocolCorpusMinioS3CompanionEveryRecord(t *testing.T) {
	path := "testdata/protocol-corpus/captures/generated-validated/gen-minio-s3-valid.pcap"
	file, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "d56497b68f50c718f06321398bef3ffe553d4ea6672e10576f0ea574e53ea00d", fmt.Sprintf("%x", sha256.Sum256(file)))
	frames := protocolCorpusAuditPackets(t, path)
	require.Len(t, frames, len(minioS3TestFixtures))
	for i, frame := range frames {
		require.Equal(t, byte(0x45), frame[14])
		require.Equal(t, byte(6), frame[23])
		require.Equal(t, byte(0x50), frame[46])
		require.Equal(t, len(frame)-14, int(binary.BigEndian.Uint16(frame[16:18])))
		require.Equal(t, uint16(40100), binary.BigEndian.Uint16(frame[34:36]))
		require.Equal(t, uint16(9000), binary.BigEndian.Uint16(frame[36:38]))
		wire := frame[54:]
		require.Equal(t, []byte(minioS3TestFixtures[i].wire), wire)
		for _, entry := range []string{"S3SignatureV4Request", "S3SignatureV4Carrier"} {
			n := minioS3TestParse(t, wire, entry)
			require.Equal(t, wire, NodeToBytes(n))
			minioS3TestTree(t, n, wire, 0)
			protocolCorpusRequireValue(t, n, "Method", strings.SplitN(minioS3TestFixtures[i].wire, " ", 2)[0])
			protocolCorpusRequireValue(t, n, "Service", "s3")
			protocolCorpusRequireValue(t, n, "Scope Terminator", "aws4_request")
		}
	}
}

func TestProtocolCorpusMinioS3ResourcesAndIsolation(t *testing.T) {
	valid := minioS3TestFixtures[0].wire
	for _, count := range []int{256, 257} {
		extra := strings.Repeat("X-Extra: value\r\n", count-4)
		wire := []byte(strings.TrimSuffix(valid, "\r\n") + extra + "\r\n")
		if count == 256 {
			require.Equal(t, wire, NodeToBytes(minioS3TestParse(t, wire, "S3SignatureV4Request")))
		} else {
			minioS3TestReject(t, wire, "header count")
		}
	}
	for _, size := range []int{1048576, 1048577} {
		header := strings.TrimSuffix(valid, "\r\n") + "Content-Length: 1000000\r\n\r\n"
		body := size - len(header)
		header = strings.Replace(header, "1000000", fmt.Sprintf("%07d", body), 1)
		wire := append([]byte(header), bytes.Repeat([]byte{'x'}, body)...)
		if size == 1048576 {
			require.Equal(t, wire, NodeToBytes(minioS3TestParse(t, wire, "S3SignatureV4Request")))
		} else {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader(wire), bits: uint64(len(wire)) * 8}
			_, err := parser.ParseBinary(r, minioS3TestRule, "S3SignatureV4Request")
			require.ErrorContains(t, err, "boundary")
			require.Equal(t, size, r.Len())
		}
	}
	for _, entry := range []string{"S3SignatureV4Request", "S3SignatureV4Carrier"} {
		_, err := parser.ParseBinary(bytes.NewReader([]byte(valid)), minioS3TestRule, entry)
		require.ErrorContains(t, err, "explicit")
		_, err = parser.GenerateBinary(map[string]any{}, minioS3TestRule, entry)
		require.Error(t, err)
		for extra := uint64(1); extra < 8; extra++ {
			r := &giopCarrierTestBitReader{Reader: bytes.NewReader([]byte(valid + "x")), bits: uint64(len(valid))*8 + extra}
			_, err = parser.ParseBinary(r, minioS3TestRule, entry)
			require.ErrorContains(t, err, "byte")
			require.Equal(t, len(valid)+1, r.Len())
		}
	}
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			wire := []byte(minioS3TestFixtures[i%len(minioS3TestFixtures)].wire)
			n := minioS3TestParse(t, wire, "S3SignatureV4Carrier")
			require.Equal(t, wire, NodeToBytes(n))
			minioS3TestTree(t, n, wire, 0)
		}(i)
	}
	wg.Wait()
}

func TestProtocolCorpusMinioS3ImportedHeldReader(t *testing.T) {
	for _, entry := range []string{"S3SignatureV4Request", "S3SignatureV4Carrier"} {
		for offset := uint64(0); offset < 8; offset++ {
			for _, valid := range []bool{true, false} {
				if !valid && entry == "S3SignatureV4Request" {
					continue // direct failures and exact staging rollback are tested natively
				}
				wire := []byte(minioS3TestFixtures[4].wire)
				if !valid {
					wire = append(wire, 'x')
				}
				t.Run(fmt.Sprintf("%s/%d/%t", entry, offset, valid), func(t *testing.T) {
					var packed bytes.Buffer
					w := base.NewBitWriter(&packed)
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					}
					require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
					require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
					if offset > 0 {
						require.NoError(t, w.WriteBits([]byte{0}, 8-offset))
					}
					root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.GetSubNode("Message").SetMaxLength(%d)
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
      if %d > 0 { this.ProcessSubNode("Padding") }
    Prefix: uint8,%dbit
    Message: "import:application-layer/minio_s3.yaml;node:%s"
    Sentinel: uint8
    Padding: uint8,%dbit
`, offset, len(wire), offset, offset, entry, 8-offset))
					root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
					root.Ctx.SetItem("s3-caller", "preserved")
					r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
					require.NoError(t, r.Backup())
					require.NoError(t, root.ParseSubNode(r, "Envelope"))
					n := base.GetNodeByPath(root, "@Envelope")
					if valid {
						minioS3TestTree(t, n, wire, offset)
					} else {
						latTestField(t, n, "Unparsed S3 Signature V4 Request", "raw", offset, offset+uint64(len(wire))*8, wire)
						require.Nil(t, protocolCorpusFindNode(n, "Method"))
					}
					protocolCorpusRequireValue(t, n, "Sentinel", uint64(0xd3))
					require.Equal(t, "preserved", root.Ctx.GetItem("s3-caller"))
					require.Equal(t, packed.Bytes(), NodeToBytes(n))
					require.NoError(t, r.Recovery())
					got, err := r.ReadBits(uint64(packed.Len()) * 8)
					require.NoError(t, err)
					require.Equal(t, packed.Bytes(), got)
					require.ErrorContains(t, r.PopBackup(), "no backup")
				})
			}
		}
	}
}

func TestProtocolCorpusMinioS3ImportedNonByteBoundary(t *testing.T) {
	wire := []byte(minioS3TestFixtures[0].wire)
	for _, entry := range []string{"S3SignatureV4Request", "S3SignatureV4Carrier"} {
		for offset := uint64(0); offset < 8; offset++ {
			for extra := uint64(1); extra < 8; extra++ {
				var packed, before bytes.Buffer
				w, expected := base.NewBitWriter(&packed), base.NewBitWriter(&before)
				if offset > 0 {
					require.NoError(t, w.WriteBits([]byte{0x55}, offset))
					require.NoError(t, expected.WriteBits([]byte{0x55}, offset))
				}
				require.NoError(t, w.WriteBits(wire, uint64(len(wire))*8))
				require.NoError(t, w.WriteBits([]byte{0x55}, extra))
				require.NoError(t, w.WriteBits([]byte{0xd3}, 8))
				if pad := (8 - (offset+extra)%8) % 8; pad > 0 {
					require.NoError(t, w.WriteBits([]byte{0}, pad))
				}
				root := latTestInline(t, fmt.Sprintf(`Package:
  Envelope:
    operator: |
      if %d > 0 { this.ProcessSubNode("Prefix") }
      this.ProcessSubNode("Message")
      this.ProcessSubNode("Sentinel")
    Prefix: uint8,%dbit
    Message:
      import: application-layer/minio_s3.yaml
      node: %s
      length: %d
    Sentinel: uint8
`, offset, offset, entry, uint64(len(wire))*8+extra))
				root.Cfg.SetItem(base.CfgLength, uint64(packed.Len())*8)
				r := base.NewBitReader(bytes.NewReader(packed.Bytes()))
				require.ErrorContains(t, root.ParseSubNode(r, "Envelope"), "explicit byte")
				require.Equal(t, offset, root.Ctx.GetUint64("pointer"))
				require.Equal(t, before.Bytes(), root.Ctx.GetItem("buffer").(*bytes.Buffer).Bytes())
				require.Equal(t, expected.Snapshot(), root.Ctx.GetItem("writer").(*base.BitWriter).Snapshot())
				got, err := r.ReadBits(uint64(len(wire)) * 8)
				require.NoError(t, err)
				require.Equal(t, wire, got)
				_, err = r.ReadBits(extra)
				require.NoError(t, err)
				got, err = r.ReadBits(8)
				require.NoError(t, err)
				require.Equal(t, []byte{0xd3}, got)
				require.ErrorContains(t, r.PopBackup(), "no backup")
			}
		}
	}
}
