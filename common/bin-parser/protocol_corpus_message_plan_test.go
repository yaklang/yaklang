package bin_parser

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/gopacket/gopacket"
	"github.com/gopacket/gopacket/layers"
	"github.com/shirou/gopsutil/v4/process"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Observed, explicitly framed messages from the existing capture corpus. The
// frame/phase choices mirror each family's independent corpus tests; these are
// NOT inferred by the benchmark, and no protocol detection is timed here.
// Segmented TDS/IMAP/SMTP messages are joined only at the audited boundaries.
func messagePlanCorpus(tb testing.TB) []currentCorpusWork {
	tb.Helper()
	works := currentCorpusWorks(tb, true)
	raw, err := os.ReadFile("testdata/protocol-corpus/manifest.json")
	require.NoError(tb, err)
	var manifest protocolCorpusManifest
	require.NoError(tb, json.Unmarshal(raw, &manifest))
	for _, capture := range manifest.Captures {
		family := ""
		switch capture.ID {
		case "ndpi-mysql":
			family = "mysql"
		case "ndpi-postgresql":
			family = "postgresql"
		case "ndpi-oracle":
			family = "tns"
		case "ndpi-mssql":
			family = "tds"
		case "ndpi-mqtt":
			family = "mqtt"
		case "ndpi-kerberos-error", "ndpi-kerberos-login":
			family = "kerberos"
		case "ndpi-imap":
			family = "imap"
		case "ndpi-smtp":
			family = "smtp"
		case "gen-ldap", "pr5023-gen-ldap":
			family = "ldap"
		case "ndpi-pop3":
			family = "pop3"
		}
		if family == "" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join("testdata/protocol-corpus", capture.CaptureFile))
		require.NoError(tb, err)
		require.Equal(tb, capture.SHA256, fmt.Sprintf("%x", sha256.Sum256(raw)))
		reader, err := protocolCorpusEnvelopePacketReader(raw)
		require.NoError(tb, err)
		var joined []byte
		for frame := 1; ; frame++ {
			record, _, err := reader.ReadPacketData()
			if err == io.EOF {
				break
			}
			require.NoError(tb, err)
			packet := gopacket.NewPacket(record, layers.LayerTypeEthernet, gopacket.Default)
			transport := packet.TransportLayer()
			if transport == nil || len(transport.LayerPayload()) == 0 {
				continue
			}
			wire := transport.LayerPayload()
			tcp, isTCP := transport.(*layers.TCP)
			entry := ""
			switch family {
			case "mysql":
				entry = mysqlFieldsTestOriginalEntries[frame]
			case "postgresql":
				// The existing public fixture set fixes phase and SCRAM context.
				switch frame {
				case 7, 43, 45, 51, 53, 15, 83, 87, 86, 21, 25:
					entry = postgresqlFieldsOriginalEntry(frame, tcp.DstPort == 5432)
				}
			case "tns":
				if name := tnsFieldsTestEntries[frame]; name != "" {
					entry = "TNS" + name + "Fields"
				}
			case "tds":
				if frame >= 26 && frame <= 32 {
					joined = append(joined, wire...)
					if frame != 32 {
						continue
					}
					wire = joined
				}
				version := "72"
				if frame == 5 || frame == 6 || frame == 8 || frame >= 35 && frame <= 37 {
					version = "71"
				}
				kind := map[byte]string{1: "Batch", 3: "RPC", 4: "Response"}[wire[0]]
				require.NotEmpty(tb, kind)
				entry = "TDS" + kind + version + "Fields"
			case "mqtt":
				entry = "MQTT31PacketFields"
				if frame == 9 {
					entry = "MQTT311PacketFields"
				}
			case "kerberos":
				entry = "KerberosMessageFields"
				if isTCP {
					entry = "KerberosTCPFields"
				}
			case "ldap":
				entry = "LDAPBindRequestFields"
			case "imap":
				entry = "IMAPResponseBlockFields"
				if tcp.DstPort == 143 {
					entry = "IMAPCommandFields"
				}
				if frame == 30 || frame == 32 {
					joined = append(joined, wire...)
					if frame != 32 {
						continue
					}
					wire = joined
				}
			case "smtp":
				// SMTP replies use a different Node-building helper and are not
				// covered by this native execution benchmark.
				if tcp.DstPort != 25 {
					continue
				}
				entry = "SMTPCommandFields"
				if frame >= 76 && frame <= 88 {
					joined = append(joined, wire...)
					if frame != 88 {
						continue
					}
					wire, entry = joined, "SMTPDataFields"
				}
			case "pop3":
				// This capture's segmented message groups are handled below in
				// the complete-family regression; retain whole command/status
				// records here, excluding its separately framed mail bodies.
				if tcp.DstPort == 110 {
					entry = "POP3CommandFields"
				}
				if frame == 56 || frame == 85 || frame == 104 {
					entry = "POP3ClientContinuationFields"
				}
			}
			if entry != "" {
				works = append(works, currentCorpusWork{id: fmt.Sprintf("%s/%d", capture.ID, frame), rule: "application-layer." + family + "_fields", entry: entry, wire: bytes.Clone(wire)})
			}
		}
	}
	sort.Slice(works, func(i, j int) bool { return works[i].id < works[j].id })
	return works
}

func TestMessagePlanCorpusEquivalence(t *testing.T) {
	works := messagePlanCorpus(t)
	families, entries, size := map[string]bool{}, map[string]bool{}, 0
	for _, w := range works {
		families[w.rule], entries[w.rule+"/"+w.entry] = true, true
		size += len(w.wire)
		t.Run(w.id, func(t *testing.T) {
			plan, err := PrepareStructured(w.rule, w.entry)
			require.NoError(t, err)
			want, err := structuredNodeReference(w.wire, w.rule, w.entry)
			require.NoError(t, err)
			wire := bytes.Clone(w.wire)
			got, err := plan.Parse(wire)
			require.NoError(t, err)
			require.Equal(t, want, got)
			compat, err := ParseStructured(wire, w.rule, w.entry)
			require.NoError(t, err)
			require.Equal(t, want, compat)
			clear(wire)
			require.Equal(t, want, got, "input reuse changed plan output")
			require.Equal(t, want, compat, "input reuse changed compatibility output")
			for _, cut := range []int{0, 1, len(w.wire) / 2, len(w.wire) - 1} {
				compareMessagePlanOutcome(t, plan, w.wire[:cut], w.rule, w.entry)
			}
			compareMessagePlanOutcome(t, plan, append(bytes.Clone(w.wire), 0), w.rule, w.entry)
			for _, at := range []int{0, len(w.wire) / 2, len(w.wire) - 1} {
				bad := bytes.Clone(w.wire)
				bad[at] ^= 0xff
				compareMessagePlanOutcome(t, plan, bad, w.rule, w.entry)
			}
		})
	}
	t.Logf("%d real framed messages, %d bytes, %d families, %d explicit entries", len(works), size, len(families), len(entries))
	require.Len(t, works, 193)
	require.Equal(t, 73487, size)
	require.Len(t, families, 12)
	require.Len(t, entries, 53)
}

func compareMessagePlanOutcome(t *testing.T, p *StructuredPlan, wire []byte, rule, entry string) {
	t.Helper()
	want, wantErr := structuredNodeReference(wire, rule, entry)
	got, err := p.Parse(wire)
	if wantErr != nil {
		require.Error(t, err)
		require.Nil(t, got)
		compat, compatErr := ParseStructured(wire, rule, entry)
		require.Nil(t, compat)
		require.EqualError(t, compatErr, wantErr.Error())
	} else {
		require.NoError(t, err)
		require.Equal(t, want, got)
	}
}

func TestMessagePlanConcurrentIsolation(t *testing.T) {
	works := messagePlanCorpus(t)
	plans, want := make([]*StructuredPlan, len(works)), make([]map[string]any, len(works))
	for i, w := range works {
		var err error
		plans[i], err = PrepareStructured(w.rule, w.entry)
		require.NoError(t, err)
		want[i], err = structuredNodeReference(w.wire, w.rule, w.entry)
		require.NoError(t, err)
	}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for round := 0; round < 2; round++ {
				for i, w := range works {
					wire := bytes.Clone(w.wire)
					got, err := plans[i].Parse(wire)
					if err != nil {
						t.Error(err)
						return
					}
					clear(wire)
					if !assertMessagePlanEqual(t, want[i], got) {
						return
					}
					clear(got)
				}
			}
		}()
	}
	wg.Wait()
}

func assertMessagePlanEqual(t *testing.T, want, got any) bool {
	t.Helper()
	// Assertions in worker goroutines must not call FailNow.
	return assert.Equal(t, want, got)
}

// Same real messages for all three paths. Workers live for the measurement,
// each processes complete batches; no goroutine or shared sink per message.
// Input extraction, preparation and warmup are excluded, all owned fields and
// metadata, GC and process CPU are included. No JSON encoding is measured.
func BenchmarkMessagePlanCorpus(b *testing.B) {
	works := messagePlanCorpus(b)
	for _, mode := range []string{"node", "structured", "plan"} {
		b.Run(mode, func(b *testing.B) { benchmarkMessagePlanWorks(b, works, mode) })
	}
}

func BenchmarkMessagePlanFamilies(b *testing.B) {
	groups := map[string][]currentCorpusWork{}
	for _, w := range messagePlanCorpus(b) {
		groups[w.rule] = append(groups[w.rule], w)
	}
	names := make([]string, 0, len(groups))
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		b.Run(strings.TrimPrefix(name, "application-layer."), func(b *testing.B) { benchmarkMessagePlanWorks(b, groups[name], "plan") })
	}
}

func benchmarkMessagePlanWorks(b *testing.B, works []currentCorpusWork, mode string) {
	plans := make([]*StructuredPlan, len(works))
	size := 0
	for i, w := range works {
		var err error
		plans[i], err = PrepareStructured(w.rule, w.entry)
		require.NoError(b, err)
		size += len(w.wire)
	}
	parse := func(i int) (map[string]any, error) {
		w := works[i]
		switch mode {
		case "node":
			return structuredNodeReference(w.wire, w.rule, w.entry)
		case "structured":
			return ParseStructured(w.wire, w.rule, w.entry)
		default:
			return plans[i].Parse(w.wire)
		}
	}
	for i := range works {
		_, err := parse(i)
		require.NoError(b, err)
	}
	workers := runtime.GOMAXPROCS(0)
	results, errs := make([]map[string]any, workers), make([]error, workers)
	proc, err := process.NewProcess(int32(os.Getpid()))
	require.NoError(b, err)
	b.ReportAllocs()
	b.SetBytes(int64(size))
	cpu, err := proc.Times()
	require.NoError(b, err)
	b.ResetTimer()
	var wg sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func(worker int) {
			defer wg.Done()
			for round := worker; round < b.N; round += workers {
				for i := range works {
					results[worker], errs[worker] = parse(i)
					if errs[worker] != nil {
						return
					}
				}
			}
		}(worker)
	}
	wg.Wait()
	b.StopTimer()
	end, err := proc.Times()
	require.NoError(b, err)
	for _, err := range errs {
		require.NoError(b, err)
	}
	b.ReportMetric((end.User+end.System-cpu.User-cpu.System)*1e9/float64(b.N), "cpu-ns/op")
	b.ReportMetric(float64(size)*8*float64(b.N)/b.Elapsed().Seconds()/1e6, "Mbps")
	b.ReportMetric(float64(len(works)), "messages/op")
	b.ReportMetric(float64(size), "input-B/op")
	b.ReportMetric(float64(workers), "workers")
	runtime.KeepAlive(results)
}
