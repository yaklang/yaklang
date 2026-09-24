package pcaputil

import (
	"bytes"
	"testing"

	"github.com/gopacket/gopacket/layers"
	"github.com/stretchr/testify/require"
)

func TestWinlab5013TextInternetCaptures(t *testing.T) {
	cases := []struct {
		name     string
		protocol string
		check    func(*testing.T, []*ProtocolEvent)
	}{
		{
			name: "02-finger.pcapng", protocol: "finger",
			check: func(t *testing.T, events []*ProtocolEvent) {
				require.Len(t, events, 4)
				var login, plan bool
				for _, event := range events {
					require.Equal(t, "alice", event.Session["Query User"])
					if event.Session["Packet Name"] == "Login" {
						login = true
						require.Equal(t, "Alice Lab", event.Session["Name"])
					}
					if event.Session["Packet Name"] == "Plan Text" {
						plan = true
						require.Equal(t, "Windows lab finger fixture for yaklang PR 5013.", event.Session["Plan"])
					}
				}
				require.True(t, login)
				require.True(t, plan)
			},
		},
		{
			name: "03-whois.pcapng", protocol: "whois",
			check: func(t *testing.T, events []*ProtocolEvent) {
				require.Len(t, events, 4)
				var domain, referral, id bool
				for _, event := range events {
					require.Equal(t, "example.invalid", event.Session["Query Name"])
					switch event.Session["Packet Name"] {
					case "Response Field":
						if event.Session["Domain Name"] == "EXAMPLE.INVALID" {
							domain = true
						}
						if event.Session["Referral Server Value"] == "whois.lab-invalid:43" {
							referral = true
						}
						if event.Session["Registry Domain ID Value"] == "LAB-5013-EXAMPLE" {
							id = true
						}
					}
				}
				require.True(t, domain)
				require.True(t, referral)
				require.True(t, id)
			},
		},
		{
			name: "04-gopher.pcapng", protocol: "gopher",
			check: func(t *testing.T, events []*ProtocolEvent) {
				require.Len(t, events, 4)
				var menu, body bool
				for _, event := range events {
					if event.Session["Packet Name"] == "Menu" {
						menu = true
						require.Equal(t, "Lab files", event.Session["Menu Item"])
						require.Equal(t, "/files", event.Session["Menu Selector"])
					}
					if event.Session["Packet Name"] == "Text Item" {
						body = true
						require.Equal(t, "/readme", event.Session["Selector"])
						require.Equal(t, "yaklang lab gopher item", event.Session["Body"])
					}
				}
				require.True(t, menu)
				require.True(t, body)
			},
		},
		{
			name: "05-dict.pcapng", protocol: "dict",
			check: func(t *testing.T, events []*ProtocolEvent) {
				require.Len(t, events, 10)
				var database, definition bool
				for _, event := range events {
					require.Equal(t, "lab-win/1.0", event.Session["Client Name"])
					if event.Session["Packet Name"] == "Database List" {
						database = true
						require.Equal(t, "lab-words", event.Session["Database"])
						require.Equal(t, "Windows lab dictionary", event.Session["Database Description"])
					}
					if event.Session["Packet Name"] == "Definition" {
						definition = true
						require.Equal(t, "protocol", event.Session["Word"])
						require.Equal(t, "lab-words", event.Session["Database"])
						require.Equal(t, "A lab definition used only as a parse fixture.", event.Session["Definition"])
					}
				}
				require.True(t, database)
				require.True(t, definition)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.protocol, func(t *testing.T) {
			all := replayWinlab5013Protocols(t, tc.name)
			var events []*ProtocolEvent
			for _, event := range all {
				if event.Protocol != tc.protocol {
					continue
				}
				require.Equal(t, "decoded", event.Status, "%s: %s", event.Status, event.Error)
				require.Equal(t, tc.protocol+"-bounded", event.Profile)
				require.Equal(t, "message", event.Completeness)
				require.Equal(t, "wire-and-port-hint", event.Admission)
				require.Equal(t, "reassembled", event.SourceBytes.Kind)
				require.NotEmpty(t, event.SourceBytes.PacketRefs, "real captures must retain packet provenance")
				require.NotEmpty(t, event.Raw)
				decoded, err := event.Decode()
				require.NoError(t, err)
				require.Equal(t, event.Session, decoded["fields"])
				events = append(events, event)
			}
			if tc.protocol == "gopher" && len(events) != 4 {
				for _, event := range all {
					t.Logf("event protocol=%q status=%q direction=%d summary=%q error=%q", event.Protocol, event.Status, event.Direction, event.Summary, event.Error)
				}
			}
			tc.check(t, events)
			var segmented bool
			for _, event := range events {
				segmented = segmented || len(event.SourceBytes.PacketRefs) > 1
			}
			require.True(t, segmented, "the generated 24-byte TCP segmentation must be reassembled")
		})
	}
}

func TestTextInternetNearSignaturesDoNotAutoAdmit(t *testing.T) {
	cases := []struct {
		name  string
		port  layers.TCPPort
		steps []sessionStep
	}{
		{"finger invalid query", 79, []sessionStep{{dir: 0, wire: []byte("/W \x01\r\n")}}},
		{"whois invalid domain", 43, []sessionStep{{dir: 0, wire: []byte("-bad..invalid\r\n")}}},
		{"gopher malformed menu", 70, []sessionStep{{dir: 0, wire: []byte("\r\n")}, {dir: 1, wire: []byte("1Lab files\t/files\tlab.invalid\t0\r\n.\r\n")}}},
		{"dict malformed greeting", 2628, []sessionStep{{dir: 1, wire: []byte("220 lab dict <1.0\r\n")}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var events []*ProtocolEvent
			capture := sessionTestPCAP(t, tc.steps, tc.port, 1, false, true)
			err := ReplayPcap(bytes.NewReader(capture), WithTCPReassemblyWorkers(1), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) }))
			require.NoError(t, err)
			require.NotEmpty(t, events)
			for _, event := range events {
				require.NotEqual(t, "finger", event.Protocol)
				require.NotEqual(t, "whois", event.Protocol)
				require.NotEqual(t, "gopher", event.Protocol)
				require.NotEqual(t, "dict", event.Protocol)
			}
		})
	}
}
