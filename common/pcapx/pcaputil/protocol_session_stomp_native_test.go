package pcaputil

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/internal/trafficfixture"
)

var stompNativeRoot = filepath.Join("..", "..", "bin-parser", "testdata", "protocol-native", "stomp")

type stompOracle struct {
	Version string `json:"stomp_version"`
	Events  []struct {
		Direction, Command string
		Headers            map[string]any
		Body               string `json:"body_base64"`
	}
}

func stompNativeOracle(t *testing.T, version string) stompOracle {
	t.Helper()
	raw, err := trafficfixture.ReadFile(filepath.Join(stompNativeRoot, "rabbitmq-4.1.0-stomp-"+version+"-client-oracle.json"))
	require.NoError(t, err)
	var oracle stompOracle
	require.NoError(t, json.Unmarshal(raw, &oracle))
	return oracle
}
func assertSTOMPNativeOracle(t *testing.T, events []*ProtocolEvent, oracle stompOracle) {
	t.Helper()
	var actual []map[string]any
	for _, event := range events {
		require.Empty(t, event.Error, "%s %q", event.Protocol, event.Raw)
		if event.Session["Heartbeat"] == true {
			continue
		}
		require.Equal(t, "stomp", event.Protocol, "%q", event.Raw)
		actual = append(actual, event.Session)
	}
	require.Len(t, actual, len(oracle.Events))
	// Preserve each role's order; capture ordering across the two sockets can
	// differ from the Python receiver thread's callback scheduling.
	for _, server := range []bool{false, true} {
		var got []map[string]any
		for _, event := range actual {
			if stompServerCommands[event["Command"].(string)] == server {
				got = append(got, event)
			}
		}
		at := 0
		for _, want := range oracle.Events {
			if (want.Direction == "server") != server {
				continue
			}
			require.Less(t, at, len(got))
			event := got[at]
			at++
			require.Equal(t, want.Command, event["Command"])
			if want.Command != "CONNECT" && want.Command != "STOMP" {
				require.Equal(t, oracle.Version, event["Version"])
			}
			body, err := base64.StdEncoding.DecodeString(want.Body)
			require.NoError(t, err)
			require.True(t, bytes.Equal(body, event["Body"].([]byte)))
			headers := event["Headers"].(map[string]any)
			for key, value := range want.Headers {
				// stomp.py's on_send callback contains escaped wire headers;
				// the paired server callback independently decoded this exact
				// echoed header, and client.py asserted the original value.
				if want.Direction == "client" && want.Command == "SEND" && key == "x-variant" {
					found := false
					for _, response := range oracle.Events {
						if response.Direction == "server" && response.Command == "MESSAGE" && response.Body == want.Body {
							value = response.Headers[key]
							found = true
							break
						}
					}
					require.True(t, found)
				}
				if key == "passcode" {
					value = "[redacted]"
				}
				require.Equal(t, fmt.Sprint(value), headers[key], "command=%s header=%s", want.Command, key)
			}
		}
		require.Equal(t, len(got), at)
	}
}
func TestSTOMPNativeCapturesAndOracles(t *testing.T) {
	raw, err := trafficfixture.ReadFile(filepath.Join(stompNativeRoot, "manifest.json"))
	require.NoError(t, err)
	var manifest struct {
		Kind  string
		Files []struct{ File, SHA256 string }
	}
	require.NoError(t, json.Unmarshal(raw, &manifest))
	require.Equal(t, "native", manifest.Kind)
	for _, file := range manifest.Files {
		raw, err := trafficfixture.ReadFile(filepath.Join(stompNativeRoot, file.File))
		require.NoError(t, err)
		require.Equal(t, file.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)), file.File)
	}
	for _, version := range []string{"1.0", "1.1", "1.2"} {
		t.Run(version, func(t *testing.T) {
			oracle := stompNativeOracle(t, version)
			raw, err := trafficfixture.ReadFile(filepath.Join(stompNativeRoot, "rabbitmq-4.1.0-stomp-"+version+".pcap"))
			require.NoError(t, err)
			for _, workers := range []int{1, 4} {
				var events []*ProtocolEvent
				require.NoError(t, ReplayPcap(bytes.NewReader(raw), WithTCPReassemblyWorkers(workers), WithOnProtocolMessage(func(e *ProtocolEvent) { events = append(events, e) })))
				assertSTOMPNativeOracle(t, events, oracle)
			}
		})
	}
}
func TestSTOMPNativeExtractedPayloadFragmentation(t *testing.T) {
	for _, version := range []string{"1.0", "1.1", "1.2"} {
		raw, err := trafficfixture.ReadFile(filepath.Join(stompNativeRoot, "rabbitmq-4.1.0-stomp-"+version+"-tcp-oracle.tsv"))
		require.NoError(t, err)
		reader := csv.NewReader(bytes.NewReader(raw))
		reader.Comma = '\t'
		rows, err := reader.ReadAll()
		require.NoError(t, err)
		var steps []sessionStep
		for _, row := range rows[1:] {
			if row[12] == "" {
				continue
			}
			wire, err := hex.DecodeString(strings.ReplaceAll(row[12], ":", ""))
			require.NoError(t, err)
			dir := 0
			if row[4] == "46164" {
				dir = 1
			}
			if len(steps) > 0 && steps[len(steps)-1].dir == dir {
				steps[len(steps)-1].wire = append(steps[len(steps)-1].wire, wire...)
			} else {
				steps = append(steps, sessionStep{dir, wire})
			}
		}
		for _, chunk := range []int{0, 1, 7, 64} {
			for _, deferred := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/chunk=%d/deferred=%v", version, chunk, deferred), func(t *testing.T) {
					events, _ := sessionTestFlow(t, "stomp", steps, chunk, deferred)
					assertSTOMPNativeOracle(t, events, stompNativeOracle(t, version))
				})
			}
		}
	}
}
