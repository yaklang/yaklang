package differential

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseSuricataEVE(t *testing.T) {
	data := []byte(`{"event_type":"flow"}
{"event_type":"alert","alert":{"gid":1,"signature_id":9900001,"rev":2,"signature":"first"}}
{"event_type":"alert","alert":{"gid":1,"signature_id":9900001,"rev":2,"signature":"first"}}
{"event_type":"stats","stats":{"detect":{"engines":[{"rules_loaded":2,"rules_failed":0,"rules_skipped":1}]}}}
`)
	alerts, stats, err := parseSuricataEVE(data)
	require.NoError(t, err)
	require.Equal(t, []Alert{{GID: 1, SID: 9900001, Rev: 2, Count: 2, Msg: "first"}}, alerts)
	require.Equal(t, &RuleStats{Loaded: 2, Skipped: 1}, stats)
}

func TestParseSnortJSON(t *testing.T) {
	data := []byte(`{"rule":"1:9900002:3","msg":"second"}
{"rule":"1:9900002:3","msg":"second"}
`)
	alerts, err := ParseSnortJSON(data)
	require.NoError(t, err)
	require.Equal(t, []Alert{{GID: 1, SID: 9900002, Rev: 3, Count: 2, Msg: "second"}}, alerts)
}

func TestNormalizeAlertsDefaultsGIDToOne(t *testing.T) {
	alerts := normalizeAlerts([]Alert{{SID: 9900003, Rev: 1}})
	require.Equal(t, []Alert{{GID: 1, SID: 9900003, Rev: 1, Count: 1}}, alerts)
}

func TestParseSnortJSONRejectsInvalidRuleID(t *testing.T) {
	_, err := ParseSnortJSON([]byte(`{"rule":"invalid"}`))
	require.Error(t, err)
}

func TestExternalRunnerRequiresConfiguration(t *testing.T) {
	config := ExternalConfig{}
	require.NotEmpty(t, config.RunSuricata(context.Background(), "rules", "capture").Error)
	require.NotEmpty(t, config.RunSnort(context.Background(), "rules", "capture").Error)
}
