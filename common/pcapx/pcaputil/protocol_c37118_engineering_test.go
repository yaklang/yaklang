package pcaputil

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"math"
	"strings"
	"testing"
	"time"
)

// The pre-feature baseline uses only the existing public selection API. Its
// runtime rejection documents a missing explicit profile, not a compile error.
func TestC37118EngineeringProfileExistingAPI(t *testing.T) {
	c := NewDefaultConfig()
	require.NoError(t, WithProtocolDecodeAs("udp", 28400, "c37118")(c), "legacy raw observation remains available")
	c = NewDefaultConfig()
	require.NoError(t, WithProtocolDecodeAs("udp", 28400, "c37118-engineering")(c), "explicit configured quantity profile must be selectable")
}

// Raw fields, keys, order, metadata, numeric-state tags, and nonfinite strings
// are exact. Only derived finite quantities permit eight representable values
// of cross-runtime rounding; there is no absolute tolerance near zero.
func c37EngineeringEqual(t *testing.T, want, got map[string]any) {
	t.Helper()
	b, err := json.Marshal(got)
	require.NoError(t, err)
	var actual map[string]any
	require.NoError(t, json.Unmarshal(b, &actual))
	var walk func(any, any, string)
	walk = func(w, g any, path string) {
		t.Helper()
		switch x := w.(type) {
		case map[string]any:
			y, ok := g.(map[string]any)
			require.True(t, ok, path)
			require.Len(t, y, len(x), path)
			for k, v := range x {
				a, ok := y[k]
				require.True(t, ok, path+"/"+k)
				walk(v, a, path+"/"+k)
			}
		case []any:
			y, ok := g.([]any)
			require.True(t, ok, path)
			require.Len(t, y, len(x), path)
			for i, v := range x {
				walk(v, y[i], fmt.Sprintf("%s/%d", path, i))
			}
		case float64:
			y, ok := g.(float64)
			require.True(t, ok, path)
			if strings.HasPrefix(path, "/Engineering/") && strings.HasSuffix(path, "/Value") {
				lo, hi := x, x
				for i := 0; i < 8; i++ {
					lo = math.Nextafter(lo, math.Inf(-1))
					hi = math.Nextafter(hi, math.Inf(1))
				}
				require.True(t, y >= lo && y <= hi, "%s: wanted %.17g got %.17g", path, x, y)
			} else {
				require.Equal(t, x, y, path)
			}
		default:
			require.Equal(t, w, g, path)
		}
	}
	walk(want, actual, "")
}

func TestC37118EngineeringDepthDirectionAndOwnership(t *testing.T) {
	for _, carrier := range []string{"tcp", "udp"} {
		for _, depth := range []int{7, 8} {
			t.Run(fmt.Sprintf("%s/depth%d", carrier, depth), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxRecursionDepth = depth
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport(carrier), WithSessionPorts(39300, 28400), WithSessionC37118Engineering())
				require.NoError(t, err)
				defer s.Close("test")
				cfg := c37CFG3Control()
				r := s.Feed(0, time.Unix(1, 0), cfg)
				clear(cfg)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				data := c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254})
				r = s.Feed(1, time.Unix(2, 0), data)
				require.NotNil(t, r.Err)
				require.Equal(t, ErrContextRequired, r.Err.Kind)
				require.NotContains(t, r.Events[0].Session, "Engineering")
				r = s.Feed(0, time.Unix(3, 0), data)
				clear(data)
				require.Len(t, r.Events, 1)
				if depth == 7 {
					require.NotNil(t, r.Err)
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					require.NotContains(t, r.Events[0].Session, "Engineering")
				} else {
					require.Nil(t, r.Err)
					e := r.Events[0]
					want, err := e.GetFields()
					require.NoError(t, err)
					eng := want["Engineering"].(map[string]any)
					require.Equal(t, false, eng["Physical Validity Verified"])
					require.Equal(t, true, eng["Complete Conversion"])
					p := eng["PMUs"].([]map[string]any)[0]
					require.EqualValues(t, 50, p["Nominal Frequency Hz"])
					require.Equal(t, 50.005, p["Frequency Hz"].(map[string]any)["Value"])
					require.Equal(t, -0.02, p["Frequency Derivative Hz/s"].(map[string]any)["Value"])
					require.Empty(t, p["Phasors"])
					require.Empty(t, p["Analogs"])
					require.Empty(t, p["Digitals"])
					e.Session["Engineering"].(map[string]any)["Complete Conversion"] = false
					decoded, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, want, decoded["fields"])
					require.Equal(t, want, decoded["session"])
					clear(decoded["session"].(map[string]any))
					again, err := e.GetFields()
					require.NoError(t, err)
					require.Equal(t, want, again)
				}
				s.Close("test")
				s.Close("again")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestC37118EngineeringContinuationBudgetAndChunks(t *testing.T) {
	frames, data := c37ContinuationControl(256)
	for _, chunk := range []int{1, 14, 16, 1200, 65535} {
		t.Run(fmt.Sprint(chunk), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionC37118Engineering())
			require.NoError(t, err)
			defer s.Close("test")
			for i, w := range append(frames, data) {
				var events []*ProtocolEvent
				var last *ProtocolError
				for at := 0; at < len(w); at += chunk {
					part := bytes.Clone(w[at:min(at+chunk, len(w))])
					r := s.Feed(0, time.Unix(int64(i+1), 0), part)
					clear(part)
					events = append(events, r.Events...)
					last = r.Err
					if at+chunk < len(w) {
						require.Empty(t, r.Events)
						require.NotNil(t, r.Err)
						require.Equal(t, ErrNeedMore, r.Err.Kind)
					}
				}
				require.Len(t, events, 1)
				e := events[0]
				require.Equal(t, "c37118-engineering", e.Profile)
				require.Equal(t, w, e.Raw)
				require.Zero(t, e.ResponseTo)
				require.Zero(t, e.TransactionID)
				if i < len(frames)-1 {
					require.NotNil(t, last)
					require.Equal(t, ErrContextRequired, last.Kind)
					continue
				}
				require.Nil(t, last)
				if i == len(frames) {
					raw, err := e.GetFields()
					require.NoError(t, err)
					eng := raw["Engineering"].(map[string]any)
					pmus := eng["PMUs"].([]map[string]any)
					require.Len(t, pmus, 1)
					p := pmus[0]
					require.EqualValues(t, 60, p["Nominal Frequency Hz"])
					require.Equal(t, 60.005, p["Frequency Hz"].(map[string]any)["Value"])
					require.Equal(t, -0.02, p["Frequency Derivative Hz/s"].(map[string]any)["Value"])
					ph := p["Phasors"].([]map[string]any)
					require.Len(t, ph, 256)
					for _, v := range ph {
						require.Equal(t, strings.Repeat("A", 255), v["Label"])
						require.Equal(t, "V", v["Unit"])
						for _, key := range []string{"Magnitude", "Angle Radians", "Real", "Imaginary"} {
							require.Equal(t, c37118Quantity(0), v[key])
						}
					}
					require.Empty(t, p["Analogs"])
					require.Empty(t, p["Digitals"])
					e.Session["Engineering"].(map[string]any)["PMUs"] = nil
					d, err := e.Decode()
					require.NoError(t, err)
					require.Equal(t, raw, d["session"])
				}
			}
			s.Close("test")
			s.Close("again")
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
	// Numeric projection reserves more than the legacy raw observation; rejecting
	// bytes must release all pending state and never yield a successful quantity.
	b := DefaultParserBudget()
	b.MaxFrameBytes = 65535
	b.MaxMessageBytes = 65535
	b.MaxBufferedBytes = 65535
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionC37118Engineering())
	require.NoError(t, err)
	r := s.Feed(0, time.Unix(1, 0), frames[0])
	require.NotNil(t, r.Err)
	require.Equal(t, ErrResourceExceeded, r.Err.Kind)
	for _, e := range r.Events {
		require.NotContains(t, e.Session, "Engineering")
	}
	s.Close("test")
	s.Close("again")
	require.Zero(t, s.Stats().BufferedBytes)
}

func TestC37118EngineeringTCPChunkBoundaries(t *testing.T) {
	b, err := trafficfixture.ReadFile("c37118-engineering/controls.json")
	require.NoError(t, err)
	var controls struct {
		Cases []struct {
			ID, Carrier, Answer string
			Limit               int `json:"collection_limit"`
		}
	}
	require.NoError(t, json.Unmarshal(b, &controls))
	for _, c := range controls.Cases {
		if c.Carrier != "tcp" {
			continue
		}
		ab, err := trafficfixture.ReadFile("c37118-engineering/" + c.Answer)
		require.NoError(t, err)
		var answer struct {
			Events []struct {
				Raw, Error, Profile string
				Direction           int
				Fields              map[string]any
			}
		}
		require.NoError(t, json.Unmarshal(ab, &answer))
		for _, chunk := range []int{1, 3, 14, 16, 255} {
			t.Run(fmt.Sprintf("%s/chunk%d", c.ID, chunk), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxCollectionElements = c.Limit
				s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionPorts(39300, 28400), WithSessionC37118Engineering())
				require.NoError(t, err)
				defer s.Close("test")
				for i, w := range answer.Events {
					wire, err := hex.DecodeString(w.Raw)
					require.NoError(t, err)
					var events []*ProtocolEvent
					var feedError *ProtocolError
					for at := 0; at < len(wire); {
						end := min(at+chunk, len(wire))
						part := append([]byte(nil), wire[at:end]...)
						r := s.Feed(w.Direction, time.Unix(int64(i+1), 0), part)
						clear(part)
						events = append(events, r.Events...)
						if end == len(wire) {
							feedError = r.Err
						}
						if end < len(wire) {
							require.Empty(t, r.Events)
							require.NotNil(t, r.Err)
							require.Equal(t, ErrNeedMore, r.Err.Kind)
						}
						at = end
					}
					require.Len(t, events, 1)
					e := events[0]
					require.Equal(t, w.Profile, e.Profile)
					require.Equal(t, w.Raw, hex.EncodeToString(e.Raw))
					require.Zero(t, e.ResponseTo)
					require.Zero(t, e.TransactionID)
					if w.Error != "" {
						rocTypedError(t, w.Error, feedError)
						c37EngineeringEqual(t, w.Fields, e.Session)
						continue
					}
					require.Nil(t, feedError)
					f, err := e.GetFields()
					require.NoError(t, err)
					c37EngineeringEqual(t, w.Fields, f)
					if cfg, ok := e.Session["Configuration"].(map[string]any); ok {
						cfg["PMUs"] = nil
					}
					d, err := e.Decode()
					require.NoError(t, err)
					c37EngineeringEqual(t, w.Fields, d["session"].(map[string]any))
				}
				s.Close("test")
				s.Close("again")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}

func TestC37118EngineeringProjectionBudget(t *testing.T) {
	cfg, data := c37CFG3Control(), c37frame(0, 2, 7, []byte{0, 0, 0, 5, 255, 254})
	// One configuration fits, but retaining it plus the derived projection does
	// not. The same byte budget still admits the adjacent legacy raw view.
	ceiling := 512 + len(cfg)*64 + len(data)*32
	for _, engineering := range []bool{false, true} {
		for _, carrier := range []string{"tcp", "udp"} {
			t.Run(fmt.Sprintf("%s/engineering%t", carrier, engineering), func(t *testing.T) {
				b := DefaultParserBudget()
				b.MaxMessageBytes = 1024
				b.MaxFrameBytes = 1024
				b.MaxBufferedBytes = ceiling
				opts := []ProtocolSessionOption{WithSessionTransport(carrier)}
				if engineering {
					opts = append(opts, WithSessionC37118Engineering())
				}
				s, err := NewProtocolSessionWithOptions(b, opts...)
				require.NoError(t, err)
				defer s.Close("test")
				r := s.Feed(0, time.Unix(1, 0), cfg)
				require.Nil(t, r.Err)
				require.Len(t, r.Events, 1)
				r = s.Feed(0, time.Unix(2, 0), data)
				require.Len(t, r.Events, 1)
				if engineering {
					require.NotNil(t, r.Err)
					require.Equal(t, ErrResourceExceeded, r.Err.Kind)
					require.NotContains(t, r.Events[0].Session, "Engineering")
				} else {
					require.Nil(t, r.Err)
					require.Contains(t, r.Events[0].Session, "Data")
					require.NotContains(t, r.Events[0].Session, "Engineering")
				}
				s.Close("test")
				s.Close("again")
				require.Zero(t, s.Stats().BufferedBytes)
			})
		}
	}
}
