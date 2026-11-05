// udp-projection matches independently captured outer IPv4 UDP headers to PDML
// positions. It never treats a flattened list of nested udp.stream values as one
// application conversation. Captures and generated evidence remain outside Git.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/yaklang/yaklang/internal/trafficoracle"
)

func run() error {
	evidencePath := flag.String("evidence", "", "JSONL of independently verified original frame bytes and IP offsets")
	pdmlPath := flag.String("pdml", "", "PDML from the same capture, retaining complete frame/ip/udp protocol fields")
	output := flag.String("output", "", "new JSONL output path; existing files are refused")
	captureHash := flag.String("capture-sha256", "", "SHA256 of the original capture pinned by the evidence producer")
	flag.Parse()
	hash, err := hex.DecodeString(*captureHash)
	if err != nil || len(hash) != 32 || *evidencePath == "" || *pdmlPath == "" || *output == "" || flag.NArg() != 0 {
		return fmt.Errorf("evidence, pdml, output and a 32-byte capture SHA256 are required")
	}
	in, err := os.Open(*evidencePath)
	if err != nil {
		return err
	}
	defer in.Close()
	evidenceHash := sha256.New()
	s := bufio.NewScanner(io.TeeReader(in, evidenceHash))
	s.Buffer(make([]byte, 4096), 2<<20)
	wanted := make(map[uint64]trafficoracle.Evidence)
	total := 0
	for s.Scan() {
		var e trafficoracle.Evidence
		d := json.NewDecoder(strings.NewReader(s.Text()))
		d.DisallowUnknownFields()
		if err := d.Decode(&e); err != nil {
			return err
		}
		var trailing any
		if err := d.Decode(&trailing); err != io.EOF {
			return fmt.Errorf("evidence record has trailing JSON")
		}
		if _, duplicate := wanted[e.Number]; duplicate || e.Number == 0 || len(e.Captured) == 0 || len(e.Captured) > 1<<20 {
			return fmt.Errorf("duplicate, empty or oversized evidence frame %d", e.Number)
		}
		total += len(e.Captured)
		if total > 64<<20 || len(wanted) >= 100000 {
			return fmt.Errorf("evidence budget exceeded; use an explicit smaller comparison scope")
		}
		wanted[e.Number] = e
	}
	if err := s.Err(); err != nil {
		return err
	}
	if len(wanted) == 0 {
		return fmt.Errorf("empty evidence scope")
	}
	pdml, err := os.Open(*pdmlPath)
	if err != nil {
		return err
	}
	defer pdml.Close()
	info, err := pdml.Stat()
	if err != nil || info.Size() > 1<<30 {
		return fmt.Errorf("PDML stat failed or exceeds 1 GiB scope budget")
	}
	pdmlHash := sha256.New()
	seen := make(map[uint64]bool)
	observations := make([]trafficoracle.UDPObservation, 0, len(wanted))
	unselected := 0
	err = trafficoracle.ReadPDML(io.TeeReader(pdml, pdmlHash), func(p trafficoracle.Packet) error {
		e, ok := wanted[p.Number]
		if !ok {
			unselected++
			return nil
		}
		o, err := trafficoracle.MatchDirectIPv4UDP(p, e)
		if err != nil {
			return fmt.Errorf("frame %d: %w", p.Number, err)
		}
		seen[p.Number] = true
		observations = append(observations, o)
		return nil
	})
	if err != nil {
		return err
	}
	if len(seen) != len(wanted) {
		return fmt.Errorf("missing reference frames: matched %d of %d", len(seen), len(wanted))
	}
	// Validate the whole scope before creating an output, so a late missing or
	// ambiguous frame cannot leave a seemingly successful partial comparison.
	out, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		out.Close()
		if !complete {
			os.Remove(*output) // Only this command's exclusively created file.
		}
	}()
	enc := json.NewEncoder(out)
	for _, o := range observations {
		if err := enc.Encode(o); err != nil {
			return err
		}
	}
	if err := out.Close(); err != nil {
		return err
	}
	complete = true
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"capture_sha256": hex.EncodeToString(hash), "evidence_sha256": hex.EncodeToString(evidenceHash.Sum(nil)),
		"pdml_sha256": hex.EncodeToString(pdmlHash.Sum(nil)), "expected_frames": len(wanted), "matched_frames": len(seen),
		"unselected_reference_frames": unselected, "scope": "source-matched direct IPv4 UDP header observations; not application/session or checksum acceptance",
	})
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
