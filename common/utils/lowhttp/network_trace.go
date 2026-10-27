package lowhttp

import (
	"context"
	"net/http"

	"github.com/yaklang/yaklang/common/utils/lowhttp/httpctx"
	"github.com/yaklang/yaklang/common/utils/lowhttp/httptrace"
)

// Trace at transport dispatch, after request normalization and before dialing.
// Redirects, status retries and reconnects each enter here independently.
func roundTripWithNetworkTrace(ctx context.Context, transport transport, tr *transportRequest) (*transportResult, error) {
	record := httptrace.Start(ctx, tr.packet, tr.option.Https)
	if record == nil {
		return transport.RoundTrip(ctx, tr)
	}
	// Do not mutate shared options while a pooled transport still owns them.
	copyRequest, copyOption := *tr, *tr.option
	copyOption.networkTraceWriter = record
	copyRequest.option = &copyOption
	result, err := transport.RoundTrip(ctx, &copyRequest)
	var raw []byte
	if result != nil {
		raw = result.rawBytes
	}
	status := GetStatusCodeFromResponse(raw)
	contentType, retryAfter := GetHTTPPacketHeader(raw, "Content-Type"), GetHTTPPacketHeader(raw, "Retry-After")
	if result != nil && result.firstResponse != nil {
		status = result.firstResponse.StatusCode
		contentType, retryAfter = result.firstResponse.Header.Get("Content-Type"), result.firstResponse.Header.Get("Retry-After")
	}
	traceErr := err
	if traceErr == nil {
		traceErr = ctx.Err()
	}
	// HTTP/2 can retry refused streams internally before returning to exec.
	// The transport rotates the recorder at each such boundary.
	record, _ = copyRequest.option.networkTraceWriter.(*httptrace.Record)
	record.Finish(traceErr, status, contentType, retryAfter, result != nil && !httpctx.GetResponseTooLarge(tr.reqIns))
	return result, err
}

func restartNetworkTrace(ctx context.Context, tr *transportRequest, err error) {
	record, ok := tr.option.networkTraceWriter.(*httptrace.Record)
	if !ok || record == nil {
		return
	}
	record.Finish(err, 0, "", "", false)
	option := *tr.option
	option.networkTraceWriter = httptrace.Start(ctx, tr.packet, tr.option.Https)
	tr.option = &option
}

func traceAdditionalHeaders(option *LowhttpExecConfig, kind string, status int, header http.Header) {
	if option == nil {
		return
	}
	if record, ok := option.networkTraceWriter.(*httptrace.Record); ok {
		record.AdditionalHeaders(kind, status, header)
	}
}
