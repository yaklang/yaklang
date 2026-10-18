package pcaputil

import (
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
	"unsafe"
)

func atgDelivery(w string) []byte {
	b := []byte(w)
	var n uint16
	for _, v := range b {
		n += uint16(v)
	}
	return append(b, []byte(fmt.Sprintf("%04X\x03", -n))...)
}
func TestATGDeliveryExistingAPI(t *testing.T) {
	for _, command := range []string{"i201", "i202"} {
		t.Run(command, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			q := []byte("\x01" + command + "01")
			a := s.Feed(0, time.Unix(1, 0), q)
			require.Len(t, a.Events, 1, "documented read-only command requires an actual observed message")
			require.Empty(t, a.Events[0].Error)
			require.Equal(t, command, a.Events[0].Session["Function"])
			var r []byte
			if command == "i201" {
				r = atgTestResponse("01", "011000507"+atgTestValues)
			} else {
				r = atgDelivery("\x01i20201261015093001101" + "2610150900" + "2610150910" + "0A" + "447A00004478C0003FC00000C080000044BB800044BAE0003FC00000C040000042280000427C0000" + "&&")
			}
			o := s.Feed(1, time.Unix(2, 0), r)
			require.Len(t, o.Events, 1)
			require.Empty(t, o.Events[0].Error)
			require.Equal(t, "matched", o.Events[0].Session["Association"])
			if command == "i202" {
				require.Equal(t, "In-Tank Delivery Response", o.Events[0].Session["Packet Name"])
				row := o.Events[0].Session["Tanks"].([]map[string]any)[0]
				require.Equal(t, 1, row["Delivery Count"])
				d := row["Deliveries"].([]map[string]any)[0]
				require.Equal(t, 1000.0, d["Starting Volume"])
				require.Equal(t, 1500.0, d["Ending Volume"])
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestATGQueryOwnershipExistingAPI(t *testing.T) {
	for _, edit := range []bool{false, true} {
		t.Run(fmt.Sprint(edit), func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), []byte("\x01i20101"))
			require.Len(t, q.Events, 1)
			o := s.Feed(1, time.Unix(2, 0), atgTestResponse("01", "011000507"+atgTestValues))
			require.Len(t, o.Events, 1)
			e := o.Events[0]
			before, err := e.Decode()
			require.NoError(t, err)
			require.Equal(t, "matched", before["session"].(map[string]any)["Association"])
			want, err := json.Marshal(before)
			require.NoError(t, err)
			if edit {
				tlsDTLSPoison(e, "redis")
			}
			after, err := e.Decode()
			require.NoError(t, err)
			got, err := json.Marshal(after)
			require.NoError(t, err)
			require.JSONEq(t, string(want), string(got))
			require.Empty(t, s.Close("done"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestATGCloseRefusalExistingAPI(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	q := s.Feed(0, time.Unix(1, 0), []byte("\x01i20101"))
	require.Len(t, q.Events, 1)
	closed := s.Close("done")
	require.Len(t, closed, 1)
	require.Equal(t, "incomplete", closed[0].Status)
	f, err := closed[0].GetFields()
	rocTypedError(t, "NeedMore", err)
	require.Nil(t, f)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestATGDeliveryRejectAmbiguity(t *testing.T) {
	s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	a := s.Feed(0, time.Unix(1, 0), []byte("\x01i20201"))
	b := s.Feed(0, time.Unix(2, 0), []byte("\x01i20202"))
	require.Len(t, a.Events, 1)
	require.Len(t, b.Events, 1)
	r := s.Feed(1, time.Unix(3, 0), []byte("\x019999FF1B\x03"))
	require.Len(t, r.Events, 1)
	require.Zero(t, r.Events[0].ResponseTo)
	wire := atgDelivery("\x01i20202261015093002A00&&")
	o := s.Feed(1, time.Unix(4, 0), wire)
	require.Len(t, o.Events, 1)
	require.Empty(t, o.Events[0].Error)
	require.Zero(t, o.Events[0].ResponseTo, "untagged rejection cannot choose which pending command survived")
	require.Zero(t, o.Events[0].TransactionID)
	require.Equal(t, "ambiguous-response", o.Events[0].Session["Association"])
	closeEvents := s.Close("done")
	require.Len(t, closeEvents, 1)
	require.Equal(t, map[string]any{"Outstanding": 2}, closeEvents[0].Session)
	_, closeErr := closeEvents[0].GetFields()
	rocTypedError(t, "NeedMore", closeErr)
	require.Empty(t, s.Close("again"))
	require.Zero(t, s.Stats().BufferedBytes)
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestATGDeliveryByteBudget(t *testing.T) {
	for _, limit := range []int{64, 256} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			b := DefaultParserBudget()
			b.MaxMessageBytes, b.MaxFrameBytes, b.MaxBufferedBytes, b.ProbeBytes = limit, limit, 4096, 64
			s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			q := s.Feed(0, time.Unix(1, 0), []byte("\x01i20201"))
			require.Len(t, q.Events, 1)
			r := atgDelivery("\x01i20201261015093001101" + "2610150900" + "2610150910" + "0A" + "447A00004478C0003FC00000C080000044BB800044BAE0003FC00000C040000042280000427C0000" + "&&")
			o := s.Feed(1, time.Unix(2, 0), r)
			require.Len(t, o.Events, 1)
			if limit == 64 {
				rocTypedError(t, "ResourceExceeded", o.Err)
				require.Equal(t, "limited", o.Events[0].Status)
				require.LessOrEqual(t, len(o.Events[0].Raw), 64)
				require.Zero(t, s.Stats().BufferedBytes, "terminal byte refusal releases retained request state immediately")
			} else {
				require.Nil(t, o.Err)
				require.Equal(t, 1, o.Events[0].Session["Delivery Count"])
				require.Equal(t, q.Events[0].ID, o.Events[0].ResponseTo)
			}
			require.Empty(t, s.Close("done"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}
func TestATGLegacyBudgetProtection(t *testing.T) {
	b := DefaultParserBudget()
	b.MaxFrameBytes, b.MaxMessageBytes, b.MaxBufferedBytes, b.ProbeBytes = 128, 128, 300, 64
	s, err := NewProtocolSessionWithOptions(b, WithSessionTransport("tcp"), WithSessionClientDirection(0))
	require.NoError(t, err)
	q := s.Feed(0, time.Unix(1, 0), []byte("\x01i20100"))
	require.Nil(t, q.Err, "pure legacy inventory must not pay for delivery-only context")
	require.Len(t, q.Events, 1)
	r := s.Feed(1, time.Unix(2, 0), atgDelivery("\x01i201002610150930&&"))
	require.Nil(t, r.Err)
	require.Len(t, r.Events, 1)
	require.Equal(t, "matched", r.Events[0].Session["Association"])
	require.Empty(t, s.Close("done"))
	require.Zero(t, s.Stats().BufferedBytes)
}
func TestATGLegacyContextExistingAPI(t *testing.T) {
	for _, name := range []string{"reused-selector", "untagged-rejection"} {
		t.Run(name, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport("tcp"), WithSessionClientDirection(0))
			require.NoError(t, err)
			defer s.Close("cleanup")
			q := []byte("\x01i20101")
			r := atgTestResponse("01", "011000507"+atgTestValues)
			a := s.Feed(0, time.Unix(1, 0), q)
			require.Len(t, a.Events, 1)
			if name == "reused-selector" {
				first := s.Feed(1, time.Unix(2, 0), r)
				require.Len(t, first.Events, 1)
				require.Equal(t, "matched", first.Events[0].Session["Association"])
				s.Feed(0, time.Unix(3, 0), q)
				o := s.Feed(1, time.Unix(4, 0), r)
				require.Len(t, o.Events, 1)
				require.Equal(t, "ambiguous-response", o.Events[0].Session["Association"], "unsequenced reused selector cannot prove a new reply owner")
			} else {
				s.Feed(0, time.Unix(2, 0), []byte("\x01i20102"))
				o := s.Feed(1, time.Unix(3, 0), []byte("\x019999FF1B\x03"))
				require.Len(t, o.Events, 1)
				require.Equal(t, "ambiguous-response", o.Events[0].Session["Association"])
				require.NotContains(t, o.Events[0].Session, "Request Tank Selector", "untagged rejection cannot name a specific tank")
			}
		})
	}
}

func TestATGCompactCandidateState(t *testing.T) {
	require.LessOrEqual(t, int64(unsafe.Sizeof(binATG{})), (&binATG{}).sessionBytes())
	s := &binATG{clientKnown: true, maxElements: 16}
	for i := 1; i <= 3; i++ {
		_, err := s.consume(0, []byte(fmt.Sprintf("\x01i202%02d", i)), uint64(i))
		require.NoError(t, err)
	}
	_, err := s.consume(0, []byte("\x01i20203"), 4)
	require.NoError(t, err)
	f, err := s.consume(1, atgDelivery("\x01i20202261015093002A00&&"))
	require.NoError(t, err)
	require.Equal(t, uint64(2), f["Observed Request ID"])
	require.Equal(t, 3, s.count)
	f, err = s.consume(1, atgDelivery("\x01i20203261015093003A00&&"))
	require.NoError(t, err)
	require.Equal(t, "ambiguous-response", f["Association"])
	require.Zero(t, f["Observed Request ID"])
	require.Equal(t, []map[string]any{{"Function": "i202", "Tank Selector": 3, "Observed Request ID": uint64(3)}, {"Function": "i202", "Tank Selector": 3, "Observed Request ID": uint64(4)}}, f["Candidate Requests"])
	f, err = s.consume(1, atgDelivery("\x01i20201261015093001100&&"))
	require.NoError(t, err)
	require.Equal(t, uint64(1), f["Observed Request ID"])
	require.Equal(t, 2, s.count)
	require.Equal(t, uint64(3), s.ids[0])
	require.Equal(t, uint64(4), s.ids[1])
	require.Equal(t, uint16(3), s.ambiguous)
	require.Zero(t, s.ids[2])
	require.Equal(t, atgPending{}, s.pending[2])
}
