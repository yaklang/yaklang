package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) < 3 || os.Args[1] != "replay" {
		panic("usage: audit replay CAPTURE [KEYLOG]")
	}
	encoder := json.NewEncoder(os.Stdout)
	opts := []pcaputil.CaptureOption{pcaputil.WithTCPReassemblyOptions(pcaputil.TCPReassemblyOptions{AllowIncomplete: true}), pcaputil.WithOnProtocolMessage(func(e *pcaputil.ProtocolEvent) {
		fields, err := e.GetFields()
		message := map[string]any{"id": e.ID, "flow": e.FlowID, "protocol": e.Protocol, "status": e.Status, "profile": e.Profile, "completeness": e.Completeness, "summary": e.Summary, "expert": e.ExpertCode, "source_bytes": e.SourceBytes, "fields": fields, "session": e.Session}
		message["field_values"] = e.DisplayFieldValues()
		message["raw"], message["entry"], message["admission"] = e.Raw, e.Entry, e.Admission
		message["source"], message["destination"], message["direction"] = e.Source, e.Destination, e.Direction
		message["transport"], message["domain"] = e.Transport, e.Domain
		message["transaction"], message["response_to"] = e.TransactionID, e.ResponseTo
		if err != nil {
			message["fields_error"] = err.Error()
		}
		if e.Error != "" {
			message["error"] = e.Error
		}
		must(encoder.Encode(message))
	})}
	if len(os.Args) > 3 && os.Args[3] != "-" {
		data, err := os.ReadFile(os.Args[3])
		must(err)
		keys, err := pcaputil.ParseTLSKeyLog(string(data))
		must(err)
		opts = append(opts, pcaputil.WithTLSSecrets(keys))
	}
	for _, arg := range os.Args[min(4, len(os.Args)):] {
		if arg == "--ignore-embedded-secrets" {
			opts = append(opts, pcaputil.WithIgnoreEmbeddedSecrets(true))
			continue
		}
		parts := strings.Split(arg, ":")
		if len(parts) != 3 {
			panic("expected udp:PORT:PROFILE")
		}
		p, err := strconv.ParseUint(parts[1], 10, 16)
		must(err)
		opts = append(opts, pcaputil.WithProtocolDecodeAs(parts[0], uint16(p), parts[2]))
	}
	err := pcaputil.ReplayPcapFile(os.Args[2], opts...)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
