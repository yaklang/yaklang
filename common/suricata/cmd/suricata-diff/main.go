package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/yaklang/yaklang/common/suricata/differential"
)

func main() {
	var (
		suricata       = flag.String("suricata", findExecutable("suricata"), "path to the Suricata executable")
		suricataConfig = flag.String("suricata-config", defaultSuricataConfig(), "path to suricata.yaml")
		snort          = flag.String("snort", findExecutable("snort"), "path to the Snort executable")
		workDir        = flag.String("work-dir", "", "directory for generated rules and PCAPs")
		keep           = flag.Bool("keep", false, "keep a temporary work directory")
		timeout        = flag.Duration("timeout", 30*time.Second, "timeout for each external engine and case")
		pretty         = flag.Bool("pretty", true, "indent JSON output")
		rulesPath      = flag.String("rules", "", "rules file for a single custom comparison")
		pcapPath       = flag.String("pcap", "", "PCAP file for a single custom comparison")
	)
	flag.Parse()

	options := differential.Options{
		External: differential.ExternalConfig{
			Suricata:       *suricata,
			SuricataConfig: *suricataConfig,
			Snort:          *snort,
		},
		Timeout: *timeout,
		WorkDir: *workDir,
		Keep:    *keep,
	}
	var results []differential.CaseResult
	var err error
	if *rulesPath != "" || *pcapPath != "" {
		if *rulesPath == "" || *pcapPath == "" {
			fmt.Fprintln(os.Stderr, "-rules and -pcap must be used together")
			os.Exit(2)
		}
		result := differential.RunPaths(
			context.Background(),
			*rulesPath,
			*pcapPath,
			[]differential.Engine{differential.EngineYaklang, differential.EngineSuricata, differential.EngineSnort},
			options,
		)
		results = []differential.CaseResult{result}
	} else {
		results, err = differential.RunBuiltin(context.Background(), options)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	var data []byte
	if *pretty {
		data, err = json.MarshalIndent(results, "", "  ")
	} else {
		data, err = json.Marshal(results)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	fmt.Println(string(data))

	for _, result := range results {
		if !result.Comparison.Equal {
			os.Exit(1)
		}
	}
}

func findExecutable(name string) string {
	path, err := exec.LookPath(name)
	if err == nil {
		return path
	}
	candidate := filepath.Join("/opt/homebrew/bin", name)
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

func defaultSuricataConfig() string {
	candidates := []string{
		"/opt/homebrew/etc/suricata/suricata.yaml",
		"/etc/suricata/suricata.yaml",
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return ""
}
