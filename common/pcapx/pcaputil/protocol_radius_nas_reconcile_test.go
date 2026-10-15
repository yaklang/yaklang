package pcaputil

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/stretchr/testify/require"
	"testing"
)

// Original captures/answers stay immutable. This separate, source-pinned
// disposition applies the mandatory NAS constraint before interpreting EAP.
func radiusNASReconcileHistoricalEAP(t *testing.T, cases []radiusEAPControl) {
	t.Helper()
	var d struct {
		Original string `json:"original_oracle_sha256"`
		Records  []struct {
			Case        string
			Step        int
			SHA         string `json:"input_sha256"`
			Disposition string
			Expected    struct{ Error, Profile, Completeness string }
		}
		Replies []struct {
			Case        string
			Step        int
			SHA         string `json:"input_sha256"`
			Association string `json:"expected_association"`
		} `json:"direct_reply_owner_impacts"`
	}
	require.NoError(t, json.Unmarshal(finsReadBatch(t, "radius-nak", "RADIUS_NAK_TEST_ROOT", "eap50-nas-disposition.json"), &d))
	require.Equal(t, "a1180cd2b842351029b9f183111b2ee31e05b76692188a24ccdefd98b3d01da4", d.Original)
	byName := map[string]*radiusEAPControl{}
	for i := range cases {
		byName[cases[i].Name] = &cases[i]
	}
	require.Len(t, d.Records, 41)
	refused, earlier := 0, 0
	for _, r := range d.Records {
		c := byName[r.Case]
		require.NotNil(t, c)
		require.Less(t, r.Step, len(c.Steps))
		w := &c.Steps[r.Step]
		require.Equal(t, r.SHA, fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire))))
		if r.Disposition != "NAS-absence-rejection" {
			earlier++
			require.Equal(t, "ResourceExceeded", w.Error)
			continue
		}
		refused++
		require.Equal(t, "MalformedMessage", r.Expected.Error)
		require.Equal(t, "radius-access-observed", r.Expected.Profile)
		w.Error, w.Profile, w.Completeness = r.Expected.Error, r.Expected.Profile, r.Expected.Completeness
		w.Fields, w.Reply, w.Transaction = nil, nil, nil
		if c.Name == "pending-close" {
			require.Equal(t, 1, c.Close.Outstanding)
			c.Close.Outstanding = 0
			c.Close.Error = ""
		}
	}
	require.Equal(t, 40, refused)
	require.Equal(t, 1, earlier)
	require.Len(t, d.Replies, 11)
	for _, r := range d.Replies {
		c := byName[r.Case]
		require.NotNil(t, c)
		w := &c.Steps[r.Step]
		require.Equal(t, r.SHA, fmt.Sprintf("%x", sha256.Sum256(radiusWire(t, w.Wire))))
		require.Equal(t, "", w.Error)
		require.Equal(t, "ambiguous-identifier", r.Association)
		w.Reply, w.Transaction = nil, nil
		w.Fields = cloneSession(w.Fields)
		delete(w.Fields, "In Reply To")
		w.Fields["Association"] = r.Association
	}
}
