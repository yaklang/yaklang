// Same capture options as the closed events helper, plus ProtocolStats and the
// seven public API contracts omitted from its export. External audit source;
// the child agent never builds or executes this file.
package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "flag"
    "fmt"
    "io"
    "os"
    "sync"

    "github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func must(err error) { if err != nil { panic(err) } }
func errorText(err error) string { if err != nil { return err.Error() }; return "" }
func captureHash(path string) string {
    f, err := os.Open(path); must(err); defer f.Close()
    h := sha256.New(); _, err = io.Copy(h, f); must(err)
    return hex.EncodeToString(h.Sum(nil))
}
func apiContract(e *pcaputil.ProtocolEvent) map[string]any {
    fields, fieldsErr := e.GetFields()
    decoded, decodeErr := e.Decode()
    return map[string]any{
        "ID":e.ID, "FlowID":e.FlowID, "Protocol":e.Protocol, "Transport":e.Transport,
        "Profile":e.Profile, "Admission":e.Admission, "Rule":e.Rule, "Entry":e.Entry,
        "Status":e.Status, "Completeness":e.Completeness, "Error":e.Error, "ExpertCode":e.ExpertCode,
        "Length":e.Length, "TimestampNS":e.Timestamp.UnixNano(), "Source":e.Source, "Destination":e.Destination,
        "Direction":e.Direction, "Domain":e.Domain, "CarrierLayers":e.CarrierLayers,
        "TransactionID":e.TransactionID, "ResponseTo":e.ResponseTo, "TCPFlowID":e.TCPFlowID,
        "SourceBytes":e.SourceBytes, "Raw":e.Raw, "Summary":e.Summary, "Session":e.Session,
        "GetFields":fields, "GetFieldsError":errorText(fieldsErr), "Decode":decoded, "DecodeError":errorText(decodeErr),
        "DisplayFieldValues":e.DisplayFieldValues(),
    }
}
func main() {
    capture := flag.String("capture", "", "original unmodified capture outside Git")
    keylog := flag.String("keylog", "", "optional explicit keylog outside Git")
    ignoreEmbedded := flag.Bool("ignore-embedded-secrets", true, "match frozen public-runtime explicit key scope")
    flag.Parse()
    if *capture == "" || flag.NArg() != 0 { panic("expected -capture FILE [-keylog FILE] [-ignore-embedded-secrets=true]") }
    sourceHash := captureHash(*capture)
    if sourceHash != "ecbca543fbe011a85c8876420fcb2c244b9290ad653d13cd1a574158167073f4" { panic("this seven-frame probe is bound to the original Weber capture") }
    targets := map[uint64]bool{46978:true,46981:true,46982:true,46983:true,46984:true,46985:true,46988:true}
    encoder := json.NewEncoder(os.Stdout)
    var mu sync.Mutex
    events, selected := uint64(0), uint64(0)
    var stats pcaputil.ProtocolStats
    statsCallbacks := 0
    opts := []pcaputil.CaptureOption{
        pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete:true}),
        pcaputil.WithIgnoreEmbeddedSecrets(*ignoreEmbedded),
        pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
            mu.Lock(); defer mu.Unlock(); events++
            var frame uint64
            for _, ref := range e.SourceBytes.PacketRefs {
                if targets[ref.Number] { if frame != 0 && frame != ref.Number { panic("target event spans multiple unrelated physical targets") }; frame = ref.Number }
            }
            if frame != 0 {
                selected++
                must(encoder.Encode(map[string]any{"kind":"selected", "frame":frame, "event":apiContract(e)}))
            }
        }),
        pcaputil.WithOnProtocolStats(func(value pcaputil.ProtocolStats) { stats = value; statsCallbacks++ }),
    }
    if *keylog != "" {
        data, err := os.ReadFile(*keylog); must(err)
        keys, err := pcaputil.ParseTLSKeyLog(string(data)); must(err)
        opts = append(opts, pcaputil.WithTLSSecrets(keys))
    }
    err := pcaputil.ReplayPcapFile(*capture, opts...)
    must(encoder.Encode(map[string]any{"kind":"complete", "capture_SHA256":sourceHash,
        "observed_event_count":events, "selected_observations":selected, "protocol_stats_callbacks":statsCallbacks,
        "protocol_stats":stats, "replay_error":errorText(err), "ignore_embedded_secrets":*ignoreEmbedded,
        "explicit_keylog_used":*keylog != "", "AllowIncomplete":true}))
    if err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
}
