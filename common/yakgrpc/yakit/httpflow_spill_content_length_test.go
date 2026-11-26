package yakit

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// 「不修复数据包」依赖一个前提：把超大/二进制 body 外置成 {{file}} / {{unquote}} 这类
// 「无损表示」时，报文里的 Content-Length 必须保持原值。
//
// 这些 tag 渲染回来就是原始字节，所以 CL 保持不变才是自洽的；一旦在外置时把 CL 重算成
// 占位文本的长度，渲染后 CL 就和真实 body 不匹配，而 NoFixContentLength 又不会再修正它，
// 实际发出的就是错包（线上表现为 401/400 或被服务端截断）。
func TestSpillMultipartRequestKeepsContentLength(t *testing.T) {
	const dumpLimit = 64 * 1024
	withGlobalMaxContentLength(t, dumpLimit)

	bigPart := bytes.Repeat([]byte("A"), 3*dumpLimit)
	body := append([]byte("--BOUND\r\n"+
		"Content-Disposition: form-data; name=\"file\"; filename=\"image.jpg\"\r\n"+
		"Content-Type: image/jpeg\r\n\r\n"), bigPart...)
	body = append(body, []byte("\r\n--BOUND--\r\n")...)
	packet := buildRepresentationTestPacket("multipart/form-data; boundary=BOUND", body)

	stored, externalized, originalBodyLen, err := PrepareFuzzableHTTPRequestForStorage(packet)
	require.NoError(t, err)
	require.True(t, externalized)
	require.Equal(t, len(body), originalBodyLen)
	t.Cleanup(func() { CleanupFuzzableHTTPRequestResources(stored) })

	header, storedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(stored)
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Length"),
		"外置后的报文必须保留原始 Content-Length，而不是占位文本长度")
	require.Less(t, len(storedBody), len(body), "外置后存储的 body 应当变小")

	// 渲染回来必须与原始字节完全一致，此时 CL 自然匹配
	rendered := renderRepresentationTestPacket(t, stored)
	renderedHeader, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, string(body), string(renderedBody))
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(renderedHeader), "Content-Length"))
}

func TestSpillFlatRequestKeepsContentLength(t *testing.T) {
	const dumpLimit = 64 * 1024
	withGlobalMaxContentLength(t, dumpLimit)

	body := bytes.Repeat([]byte("B"), 3*dumpLimit)
	packet := buildRepresentationTestPacket("application/octet-stream", body)

	stored, externalized, originalBodyLen, err := PrepareFuzzableHTTPRequestForStorage(packet)
	require.NoError(t, err)
	require.True(t, externalized)
	require.Equal(t, len(body), originalBodyLen)
	t.Cleanup(func() { CleanupFuzzableHTTPRequestResources(stored) })

	header, _ := lowhttp.SplitHTTPHeadersAndBodyFromPacket(stored)
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Length"))

	rendered := renderRepresentationTestPacket(t, stored)
	renderedHeader, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, string(body), string(renderedBody))
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(renderedHeader), "Content-Length"))
}

// TestBuildFuzzableFlatSpillKeepsContentLength 直接校验 BuildFuzzableHTTPFlowRequestPacket 的
// {{file}} 替换不会重算 Content-Length。
func TestBuildFuzzableFlatSpillKeepsContentLength(t *testing.T) {
	const dumpLimit = 16 * 1024
	withGlobalMaxContentLength(t, dumpLimit)

	body := bytes.Repeat([]byte("C"), 3*dumpLimit)
	packet := buildRepresentationTestPacket("application/octet-stream", body)

	res, err := spillLargeHTTPFlowRequestIfNeeded(packet)
	require.NoError(t, err)
	require.True(t, res.IsTooLarge)
	t.Cleanup(func() { removeLargeRequestSpillFiles(res.HeaderFile, res.BodyFile) })

	fuzzable, err := BuildFuzzableHTTPFlowRequestPacket(res.StoredPacket, res.BodyFile)
	require.NoError(t, err)
	t.Cleanup(func() { CleanupFuzzableHTTPRequestResources(fuzzable) })

	header, fuzzableBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(fuzzable)
	require.Contains(t, string(fuzzableBody), "{{file(")
	require.Equal(t, strconv.Itoa(len(body)), lowhttp.GetHTTPPacketHeader([]byte(header), "Content-Length"))

	rendered := renderRepresentationTestPacket(t, fuzzable)
	_, renderedBody := lowhttp.SplitHTTPHeadersAndBodyFromPacket(rendered)
	require.Equal(t, string(body), string(renderedBody))
}
