package pcaputil

import (
	"fmt"
	"github.com/stretchr/testify/require"
	"os"
	"strings"
	"testing"
)

func TestFirstBatchT10Native(t *testing.T) {
	for _, deferred := range []bool{false, true} {
		for _, workers := range []int{1, 2, 4} {
			t.Run(fmt.Sprintf("deferred=%t/workers=%d", deferred, workers), func(t *testing.T) {
				data, err := os.ReadFile("testdata/protocol-sessions/first-batch-m1/http-ws-native.pcap")
				require.NoError(t, err)
				events, stats, err := binReplay(t, data, workers, WithProtocolDeferred(deferred))
				require.Zero(t, stats.BufferedBytes)
				require.NoError(t, err)
				messages, responses := 0, 0
				for _, e := range events {
					require.Empty(t, e.Error, "%s %s %v", e.Protocol, e.Summary, e.Session)
					if e.Protocol == "websocket" {
						if payload, ok := e.Session["Application"].([]byte); ok {
							require.Equal(t, strings.Repeat(fmt.Sprintf("M1 websocket %d ", messages/2), 100), string(payload))
							require.Equal(t, true, e.Session["Compressed"])
							messages++
						}
					}
					if e.Protocol == "http" && e.ResponseTo != 0 {
						responses++
						if strings.HasPrefix(string(e.Raw), "HTTP/1.1 200") {
							body, err := e.DecodeHTTPBody(4096)
							require.NoError(t, err)
							if len(body.Data) > 0 {
								require.Equal(t, "M1 chunked body", string(body.Data))
								require.Equal(t, "done", body.Trailers.Get("X-M1"))
							}
						}
					}
				}
				require.GreaterOrEqual(t, responses, 4)
				require.Equal(t, 4, messages)
			})
		}
	}
}

func TestFirstBatchT10DeflateNegotiation(t *testing.T) {
	for _, tc := range []struct {
		offer, response string
		bad             bool
	}{
		{"permessage-deflate", "permessage-deflate", false},
		{"x-permessage-deflate", "permessage-deflate", true},
		{"permessage-deflate", "permessage-deflate; client_max_window_bits=15", true},
		{"permessage-deflate; client_max_window_bits", "permessage-deflate; client_max_window_bits=15", false},
		{"permessage-deflate; server_max_window_bits=10", "permessage-deflate", true},
		{"permessage-deflate; server_no_context_takeover", "permessage-deflate", true},
		{"permessage-deflate; server_no_context_takeover", "permessage-deflate; server_no_context_takeover", false},
	} {
		err := validateWSDeflateOffer(tc.offer, tc.response)
		if tc.bad {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
		}
	}
}
