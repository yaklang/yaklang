package yakgrpc

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/mutate"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func (s *Server) CreateWebsocketFuzzer(stream ypb.Yak_CreateWebsocketFuzzerServer) error {
	firstReq, err := stream.Recv()
	if err != nil {
		return utils.Errorf("first websocket fuzzer: %s", err)
	}
	ctx, cancel := context.WithCancel(stream.Context())

	var id int64 = 0
	_requireIndexLock := new(sync.Mutex)
	requireDataFrameID := func() int64 {
		_requireIndexLock.Lock()
		defer _requireIndexLock.Unlock()
		id++
		return id
	}

	client, err := lowhttp.NewWebsocketClient(
		firstReq.GetUpgradeRequest(),
		lowhttp.WithWebsocketWithContext(ctx),
		lowhttp.WithWebsocketTLS(firstReq.GetIsTLS()),
		lowhttp.WithWebsocketTotalTimeout(float64(firstReq.GetTotalTimeoutSeconds())),
		lowhttp.WithWebsocketProxy(strings.Split(firstReq.GetProxy(), ",")...),
		lowhttp.WithWebsocketAllFrameHandler(func(c *lowhttp.WebsocketClient, f *lowhttp.Frame, data []byte, shutdown func()) {
			opcode := f.Type()
			switch opcode {
			case lowhttp.PingMessage:
				c.WritePong(data, true)
			case lowhttp.TextMessage, lowhttp.BinaryMessage:
				var encoded []string
				if utils.IsGzip(data) {
					encoded = append(encoded, "gzip")
				}

				_, isJson := utils.IsJSON(string(data))
				dataVerbose := utils.EscapeInvalidUTF8Byte(data)
				if isJson {
					var buf bytes.Buffer
					_ = json.Indent(&buf, data, "", "")
					dataVerbose = strings.ReplaceAll(string(buf.Bytes()), "\n", "")
					var formattedBuf bytes.Buffer
					_ = json.Indent(&formattedBuf, data, "", "    ")
					if formattedBuf.Len() > 0 {
						data = formattedBuf.Bytes()
					}
				}
				if dataVerbose == "" {
					dataVerbose = strings.Trim(strconv.Quote(string(data)), `"`)
				}

				size := len(data)
				msg := &ypb.ClientWebsocketResponse{
					SwitchProtocolSucceeded: true,
					IsDataFrame:             true,
					FromServer:              true,
					Data:                    data,
					DataVerbose:             dataVerbose,
					DataLength:              int64(size),
					DataSizeVerbose:         utils.ByteSize(uint64(size)),
					IsJson:                  isJson,
					IsProtobuf:              utils.IsProtobuf(data),
					DataFrameIndex:          requireDataFrameID(),
				}
				stream.Send(msg)
			case lowhttp.CloseMessage:
				cancel()
			default:
				log.Debugf("[grpc-ws] [>client] write unknown message: %d", f.GetData())
			}
		}),
	)
	if err != nil {
		return utils.Errorf("websocket client build tunnel failed: %v", err)
	}

	stream.Send(&ypb.ClientWebsocketResponse{
		IsUpgradeResponse: true,
		UpgradeResponse:   client.Response,
	})

	client.Start()

	go func() {
		defer func() {
			client.Stop()
		}()

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			req, err := stream.Recv()
			if err != nil {
				log.Errorf("stream recv wsfuzzer req failed: %v", err)
				return
			}
			raw := req.GetToServer()
			fuzzResult := mutate.MutateQuick(raw)
			// fallback
			if len(fuzzResult) == 0 {
				fuzzResult = append(fuzzResult, string(raw))
			}

			for _, message := range fuzzResult {
				messageBytes := []byte(message)
				dataVerbose := ""
				_, isJson := utils.IsJSON(message)
				if isJson {
					var buf bytes.Buffer
					_ = json.Indent(&buf, req.GetToServer(), "", "")
					if buf.Len() > 0 {
						dataVerbose = strings.ReplaceAll(string(buf.Bytes()), "\n", "")
					}
				}
				if dataVerbose == "" {
					dataVerbose = strings.Trim(strconv.Quote(message), `"`)
				}
				rawMsg := make([]byte, len(messageBytes))
				copy(rawMsg, messageBytes)
				err = client.WriteText(messageBytes)
				if err != nil {
					log.Errorf("wsfuzzer write text failed: %v", err)
					continue
				}

				msg := &ypb.ClientWebsocketResponse{
					SwitchProtocolSucceeded: true,
					IsDataFrame:             true,
					FromServer:              false,
					Data:                    rawMsg,
					DataVerbose:             dataVerbose,
					DataLength:              int64(len(message)),
					DataSizeVerbose:         utils.ByteSize(uint64(len(message))),
					IsJson:                  isJson,
					IsProtobuf:              utils.IsProtobuf(rawMsg),
					DataFrameIndex:          requireDataFrameID(),
				}
				stream.Send(msg)
			}

		}
	}()
	client.Wait()
	return nil
}
