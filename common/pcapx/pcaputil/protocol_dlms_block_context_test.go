package pcaputil

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func dlmsUnobservedNextControls(t *testing.T) []dlmsListControl {
	return dlmsNamedBlockControls(t, "dlms-hdlc-unobserved-next", "owned-dlms-hdlc-unobserved-next/v1", 12)
}

func TestDLMSHDLCBlockUnobservedNextExistingAPI(t *testing.T) {
	for _, c := range dlmsUnobservedNextControls(t) {
		t.Run(c.ID, func(t *testing.T) {
			s, err := NewProtocolSessionWithOptions(DefaultParserBudget(), WithSessionTransport(c.Transport), WithSessionPorts(40000, 4059))
			require.NoError(t, err)
			defer s.Close("context")
			var events []*ProtocolEvent
			for i, w := range c.Events {
				out := s.Feed(w.Direction, time.Unix(int64(i+1), 0), wrapperWire(t, w.Raw))
				require.Len(t, out.Events, 1)
				events = append(events, out.Events...)
			}
			dlmsListAssert(t, c, events)
			require.Empty(t, s.Close("context"))
			require.Empty(t, s.Close("again"))
			require.Zero(t, s.Stats().BufferedBytes)
		})
	}
}

func TestDLMSHDLCBlockUnobservedNextSealedMatrix(t *testing.T) {
	dlmsSealedMatrix(t, dlmsUnobservedNextControls(t))
}

func TestDLMSHDLCBlockUnobservedNextOwnershipAndChunks(t *testing.T) {
	dlmsOwnershipAndChunks(t, dlmsUnobservedNextControls(t))
}
