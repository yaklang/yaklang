package pcaputil

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// RFC 4217 sections 4.2/16 and RFC 2228 section 3/Appendix I distinguish
// TLS's 234 from a mechanism's ADAT continuation; these are independent oracles.
func TestProtocolSessionFTPAUTHMechanismScope(t *testing.T) {
	cases := []struct {
		name     string
		lines    []sessionStep
		tlsReply int
	}{
		{"TLS success", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH TLS")}, {1, mailCR("234 Proceed")}}, 2},
		{"TLS-C synonym", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH TLS-C")}, {1, mailCR("234 Proceed")}}, 2},
		{"refused then GSSAPI", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH TLS")}, {1, mailCR("504 Unsupported mechanism")}, {0, mailCR("AUTH GSSAPI")}, {1, mailCR("334 Continue with ADAT")}}, -1},
		{"GSSAPI directly", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH GSSAPI")}, {1, mailCR("334 Continue with ADAT")}}, -1},
		{"334 is not TLS success", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH TLS")}, {1, mailCR("334 Continue")}}, -1},
		{"different mechanism containing TLS", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH NOTTLS")}, {1, mailCR("234 Proceed")}}, -1},
		{"refusal keeps plaintext", []sessionStep{{1, mailCR("220 FTP ready")}, {0, mailCR("AUTH TLS")}, {1, mailCR("504 Unsupported mechanism")}, {0, mailCR("NOOP")}, {1, mailCR("200 OK")}}, -1},
	}
	for _, tc := range cases {
		for _, deferred := range []bool{false, true} {
			for mode := 0; mode < 12; mode++ {
				t.Run(fmt.Sprintf("%s/deferred=%t/chunks=%d", tc.name, deferred, mode), func(t *testing.T) {
					s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
					require.NoError(t, err)
					s.(*captureSession).f.a.config.Deferred = deferred
					var es []*ProtocolEvent
					rng := rand.New(rand.NewSource(int64(mode)))
					pending := ""
					for i, st := range tc.lines {
						for w := st.wire; len(w) > 0; {
							n := len(w)
							if mode == 1 {
								n = 1
							} else if mode > 1 {
								n = min(n, rng.Intn(19)+1)
							}
							r := s.Feed(st.dir, time.Unix(1, 0), w[:n])
							if r.Err != nil {
								require.Equal(t, ErrNeedMore, r.Err.Kind)
							}
							es = append(es, r.Events...)
							w = w[n:]
						}
						require.Len(t, es, i+1)
						e := es[i]
						require.Equal(t, st.wire, e.Raw)
						require.Equal(t, st.dir, e.Direction)
						require.Contains(t, []string{"decoded", "deferred"}, e.Status)
						require.Empty(t, e.Error)
						line := strings.TrimSuffix(string(st.wire), "\r\n")
						want := map[string]any{}
						fields := map[string]any{}
						if st.dir == 0 {
							pending = strings.Fields(line)[0]
							want = map[string]any{"Packet Name": pending, "Role": "command", "Line": line}
							fields = map[string]any{"Line": line}
						} else {
							var code int
							_, err = fmt.Sscanf(line[:3], "%d", &code)
							require.NoError(t, err)
							want = map[string]any{"Packet Name": "Reply", "Role": "reply", "Reply Code": code, "Reply Text": line[4:], "Multiline": false}
							fields = map[string]any{"Code": line[:3], "Separator": 32, "Message": line[4:]}
							if pending != "" {
								want["In Reply To"] = pending
								pending = ""
							}
							if i == tc.tlsReply {
								want["Encrypted"] = true
								want["Protocol Transition"] = "ftp->tls"
							}
						}
						assertSemanticReviewJSON(t, want, e.Session)
						actual, err := e.GetFields()
						require.NoError(t, err)
						assertSemanticReviewJSON(t, fields, actual)
					}
					require.Empty(t, s.Close("eof"))
					require.Zero(t, s.Stats().BufferedBytes)
				})
			}
		}
	}
}
func assertSemanticReviewJSON(t testing.TB, want, actual any) {
	t.Helper()
	w, err := json.Marshal(want)
	require.NoError(t, err)
	a, err := json.Marshal(actual)
	require.NoError(t, err)
	require.JSONEq(t, string(w), string(a))
}
