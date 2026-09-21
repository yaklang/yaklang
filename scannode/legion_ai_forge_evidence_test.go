package scannode

import (
	aiv1 "github.com/yaklang/yaklang/scannode/gen/legionpb/legion/ai/v1"
	"testing"
)

type forgeObservationFixture []aiApplicationMaterialReference

func (f forgeObservationFixture) applicationHTTPMaterialReferences() []aiApplicationMaterialReference {
	return f
}

func TestLegionDiscoveryRequiresCompleteObservations(t *testing.T) {
	dns := aiApplicationMaterialReference{Kind: "dns_lookup", Operations: []string{"dns_lookup"}}
	tcp := aiApplicationMaterialReference{Kind: "tcp_connect_scan", Operations: []string{"tcp_connect_scan"}}
	www := aiApplicationMaterialReference{Kind: "dns_lookup", Operations: []string{"dns_lookup", "label:www"}}
	api := aiApplicationMaterialReference{Kind: "dns_lookup", Operations: []string{"dns_lookup", "label:api"}}
	for _, tc := range []struct {
		name, labels string
		refs         forgeObservationFixture
		valid        bool
	}{
		{"missing TCP", "", forgeObservationFixture{dns}, false},
		{"full scan", "", forgeObservationFixture{dns, tcp}, true},
		{"base only", "www,api", forgeObservationFixture{dns}, false},
		{"partial labels", "www,api", forgeObservationFixture{www}, false},
		{"all labels", "www,api", forgeObservationFixture{www, api}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			release := &aiv1.ContextForgeRelease{CapabilityProfile: legionForgeDiscoveryProfile}
			if tc.labels != "" {
				release.Parameters = []*aiv1.ContextForgeParameter{{Key: "labels", Value: tc.labels, ValueKind: "string"}}
			}
			_, err := aiApplicationMaterialReferences(release, aiSessionBinding{}, tc.refs)
			if (err == nil) != tc.valid {
				t.Fatalf("valid=%v err=%v", tc.valid, err)
			}
		})
	}
}

func TestLegionEvidenceRequiresTypedSuccessfulRead(t *testing.T) {
	for _, test := range []struct {
		path, op string
		valid    bool
	}{
		{"inputs/a.pcap", "read", false}, {"inputs/a.apk", "read_lines", false},
		{"inputs/a.pcapng", "parse_packet_capture", true}, {"inputs/a.apk", "parse_android_package", true},
		{"inputs/a.log", "metadata", false}, {"inputs/a.xlsx", "read", false},
		{"inputs/a.xlsx", "extract_xlsx", true}, {"inputs/a.log", "read_lines", true},
	} {
		if got := legionEvidenceOperationMatches(test.path, test.op); got != test.valid {
			t.Fatalf("%s %s = %v", test.path, test.op, got)
		}
	}
	_, err := aiApplicationMaterialReferences(&aiv1.ContextForgeRelease{CapabilityProfile: legionForgeEvidenceProfile, Parameters: []*aiv1.ContextForgeParameter{{Key: "text", Value: "not a file", ValueKind: "text"}}}, aiSessionBinding{}, nil)
	if err == nil {
		t.Fatal("evidence without file passed")
	}
}
