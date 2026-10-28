package pcaputil

import (
	"bytes"
	"encoding/json"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/internal/trafficfixture"
	"strings"
	"testing"
	"time"
)

func TestProtocolSessionSparkplugDeviceLifecycle(t *testing.T) {
	seq := uint64(0)
	alias := uint64(1)
	will := spTestPayload(nil, spTestMetric("bdSeq", 4, 11, 0, nil))
	birth := spTestPayload(&seq, spTestMetric("bdSeq", 4, 11, 0, nil), spTestMetric("Node Control/Rebirth", 11, 14, 0, nil), spTestMetric("Temperature", 2, 10, 23, &alias))
	s, err := NewProtocolSession(DefaultParserBudget())
	require.NoError(t, err)
	defer s.Close("done")
	require.Nil(t, s.Feed(0, time.Unix(1, 0), spTestConnect(will, "spBv1.0/g/NDEATH/n")).Err)
	require.Nil(t, s.Feed(1, time.Unix(2, 0), []byte{0x20, 2, 0, 0}).Err)
	r := s.Feed(0, time.Unix(3, 0), spTestPublish("spBv1.0/g/NBIRTH/n", birth))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "matched", r.Events[0].Session["Sparkplug"].(map[string]any)["Context Status"])
	seq = 1
	alias = 2
	db := spTestPayload(&seq, spTestMetric("Temperature", 2, 10, 42, &alias))
	r = s.Feed(0, time.Unix(4, 0), spTestPublish("spBv1.0/g/DBIRTH/n/d", db))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	sp := r.Events[0].Session["Sparkplug"].(map[string]any)
	require.Equal(t, "matched", sp["Context Status"], "existing API fails to register a device under an observed node birth")
	require.Equal(t, "d", sp["Device ID"])
	require.Equal(t, "g/n", sp["Node Key"])
	require.Equal(t, uint64(0), sp["bdSeq"])
	require.Equal(t, []map[string]any{{"Name": "Temperature", "Datatype": uint64(2), "Value": int64(42)}}, sp["Metrics"])
	require.Empty(t, s.Close("complete"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Nil(t, s.Close("again"))
}

func testSparkplugDeviceBudget(t *testing.T) {
	budget := DefaultParserBudget()
	budget.MaxBufferedBytes = 1300
	budget.MaxMessageBytes = 512
	budget.MaxFrameBytes = 512
	s, err := NewProtocolSession(budget)
	require.NoError(t, err)
	defer s.Close("done")
	var answers struct {
		Profiles map[string]struct {
			Events []struct {
				Record    string
				Direction int
				Session   map[string]any
			}
		}
	}
	raw, err := trafficfixture.ReadFile("testdata/sparkplug-device/answers.json")
	require.NoError(t, err)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&answers))
	for i, w := range answers.Profiles["two-devices-same-name"].Events[:5] {
		wire, err := trafficfixture.ReadFile(strings.TrimPrefix(w.Record, "common/pcapx/pcaputil/"))
		require.NoError(t, err)
		x := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wire)
		if i < 4 {
			require.Nil(t, x.Err, "normal node and first device must fit budget")
			require.Len(t, x.Events, 1)
			spAssertTree(t, w.Session, x.Events[0].Session)
			continue
		}
		require.NotNil(t, x.Err, "second device must actually exceed the retained-state budget")
		require.Equal(t, ErrResourceExceeded, x.Err.Kind)
		require.Len(t, x.Events, 1)
		_, err = x.Events[0].GetFields()
		var typed *ProtocolError
		require.ErrorAs(t, err, &typed)
		require.Equal(t, ErrResourceExceeded, typed.Kind)
	}
	require.Nil(t, s.(*captureSession).f.mqtt, "refusal must release node/device and MQTT identifiers")
	require.Zero(t, s.Stats().BufferedBytes)
	require.Empty(t, s.Close("complete"))
	require.Nil(t, s.Close("again"))
}
