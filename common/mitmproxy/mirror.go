package mitmproxy

import (
	"bufio"
	"bytes"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"yaklang.io/yaklang/common/log"
	"yaklang.io/yaklang/common/utils"
	"yaklang.io/yaklang/common/utils/lowhttp"
)

func mirrorRequest(reqInConn io.Reader, hijacker func(*http.Request, []byte) []byte, reqOut io.Writer, cbs ...func(r *http.Request)) error {
	var byteBuffer *bytes.Buffer
	var reqIn *bufio.Reader
	if hijacker == nil {
		reqIn = bufio.NewReader(io.TeeReader(reqInConn, reqOut))
	} else {
		byteBuffer = bytes.NewBuffer(nil)
		reqIn = bufio.NewReader(io.TeeReader(reqInConn, byteBuffer))
	}
	request, err := lowhttp.ReadHTTPRequest(reqIn)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Debugf("read request-in failed: %s", err)
		return utils.Errorf("read request-in failed: %s", err)
	}
	if request != nil && request.Body != nil {
		raw, _ := ioutil.ReadAll(request.Body)
		request.GetBody = func() (io.ReadCloser, error) {
			return ioutil.NopCloser(bytes.NewBuffer(raw)), nil
		}
	}
	if request != nil {
		for _, cb := range cbs {
			cb(request)
		}
	}

	raw := byteBuffer.Bytes()
	if hijacker != nil && raw != nil {
		result := hijacker(request, raw)
		if result != nil {
			_, err = reqOut.Write(result)
			if err != nil {
				log.Errorf("hijack request error: %v", err)
				return utils.Errorf("write request error: %v", err)
			}
		} else {
			return nil
		}
	}

	if raw == nil {
		return utils.Error("empty buffer for writing request")
	}

	return nil
}

func mirrorResponse(request *http.Request, rspInConn net.Conn, hijacker func([]byte) []byte, rspOut net.Conn, cbs ...func(r *http.Response)) error {
	var byteBuffer *bytes.Buffer
	var rspIn *bufio.Reader
	if hijacker == nil {
		rspIn = bufio.NewReader(io.TeeReader(rspInConn, rspOut))
	} else {
		byteBuffer = bytes.NewBuffer(nil)
		rspIn = bufio.NewReader(io.TeeReader(rspInConn, byteBuffer))
	}

	response, err := http.ReadResponse(rspIn, request)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		log.Debugf("read response-in failed: %s", err)
		return utils.Errorf("read response-in failed: %s", err)
	}
	if response != nil && response.Body != nil {
		raw, _ := ioutil.ReadAll(response.Body)
		response.Body = ioutil.NopCloser(bytes.NewBuffer(raw))
	}

	if response != nil {
		for _, cb := range cbs {
			cb(response)
		}
	}

	raw := byteBuffer.Bytes()
	if hijacker != nil && raw != nil {
		result := hijacker(raw)
		if result != nil {
			_, err = rspOut.Write(result)
			if err != nil {
				log.Errorf("hijack response error: %v", err)
				return utils.Errorf("write response error: %v", err)
			}
		} else {
			return nil
		}
	}

	if raw == nil {
		return utils.Errorf("empty buffer for writing response")
	}

	return nil
}
