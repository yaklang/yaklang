package lowhttp

import (
	"context"
	"crypto/tls"
	"fmt"
	"github.com/quic-go/quic-go"
	"github.com/quic-go/quic-go/http3"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
)

const bodyCopyBufferSize = 8 * 1024

func doHttp3Request(ctx context.Context, clientConn *http3.ClientConn, reqPacket []byte) (*http.Response, []byte, error) {
	stream, err := clientConn.OpenRequestStream(ctx)
	if err != nil {
		return nil, nil, err
	}

	req, err := ParseBytesToHttpRequest(reqPacket)
	if err != nil {
		return nil, nil, err
	}

	req.URL.Scheme = "https"

	err = stream.SendRequestHeader(req)
	if err != nil {
		return nil, nil, err
	}

	if req.Body == nil {
		stream.Close()
	} else {
		// send the request body asynchronously
		go func() {
			contentLength := int64(-1)
			// According to the documentation for http.Request.ContentLength,
			// a value of 0 with a non-nil Body is also treated as unknown content length.
			if req.ContentLength > 0 {
				contentLength = req.ContentLength
			}
			err := sendRequestBody(stream, req.Body, contentLength)
			if err != nil {
				return
			}
			stream.Close()
		}()
	}

	// copy from net/http: support 1xx responses
	trace := httptrace.ContextClientTrace(req.Context())
	num1xx := 0               // number of informational 1xx headers received
	const max1xxResponses = 5 // arbitrary bound on number of informational responses

	var res *http.Response
	for {
		var err error
		res, err = stream.ReadResponse()
		if err != nil {
			return nil, nil, err
		}
		resCode := res.StatusCode
		is1xx := 100 <= resCode && resCode <= 199
		// treat 101 as a terminal status, see https://github.com/golang/go/issues/26161
		is1xxNonTerminal := is1xx && resCode != http.StatusSwitchingProtocols
		if is1xxNonTerminal {
			num1xx++
			if num1xx > max1xxResponses {
				return nil, nil, utils.Error("http: too many 1xx informational responses")
			}
			if trace != nil && trace.Got1xxResponse != nil {
				if err := trace.Got1xxResponse(resCode, textproto.MIMEHeader(res.Header)); err != nil {
					return nil, nil, err
				}
			}
			continue
		}
		break
	}
	res.Request = req
	respPacket, _ := utils.DumpHTTPResponse(res, res.Body != nil)
	return res, respPacket, nil

}

func sendRequestBody(str http3.RequestStream, body io.ReadCloser, contentLength int64) error {
	defer body.Close()
	buf := make([]byte, bodyCopyBufferSize)
	if contentLength == -1 {
		_, err := io.CopyBuffer(str, body, buf)
		return err
	}

	// make sure we don't send more bytes than the content length
	n, err := io.CopyBuffer(str, io.LimitReader(body, contentLength), buf)
	if err != nil {
		return err
	}
	var extra int64
	extra, err = io.CopyBuffer(io.Discard, body, buf)
	n += extra
	if n > contentLength {
		str.CancelWrite(quic.StreamErrorCode(http3.ErrCodeRequestCanceled))
		return fmt.Errorf("http: ContentLength=%d with Body length %d", contentLength, n)
	}
	return err
}

func NewHTTP3ClientConn(conn quic.Connection) *http3.ClientConn {
	tr := &http3.Transport{}
	return tr.NewClientConn(conn)
}

func getHTTP3Conn(ctx context.Context, target string, opt ...netx.DialXOption) (*http3.ClientConn, error) {
	udpConn, remoteAddr, err := netx.DialUdpX(target, append(opt, netx.DialX_WithUdpJustListen(true))...)
	if err != nil {
		return nil, err
	}
	sni := utils.ExtractHost(target)
	if sni == "" {
		return nil, utils.Error("cannot extract sni from target")
	}
	conn, err := quic.Dial(ctx, udpConn, remoteAddr, &tls.Config{
		ServerName: sni,
		NextProtos: []string{"h3"},
	}, &quic.Config{})
	if err != nil {
		return nil, utils.Wrapf(err, "quic.Dial %v failed", remoteAddr.String())
	}

	return NewHTTP3ClientConn(conn), nil
}
