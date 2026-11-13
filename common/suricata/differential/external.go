package differential

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type ExternalConfig struct {
	Suricata       string
	SuricataConfig string
	Snort          string
}

type eveEvent struct {
	EventType string `json:"event_type"`
	Alert     struct {
		GID       int    `json:"gid"`
		SID       int    `json:"signature_id"`
		Rev       int    `json:"rev"`
		Signature string `json:"signature"`
	} `json:"alert"`
	Stats struct {
		Detect struct {
			Engines []struct {
				Loaded  int `json:"rules_loaded"`
				Failed  int `json:"rules_failed"`
				Skipped int `json:"rules_skipped"`
			} `json:"engines"`
		} `json:"detect"`
	} `json:"stats"`
}

type snortAlert struct {
	Rule string `json:"rule"`
	Msg  string `json:"msg"`
}

func ParseSuricataEVE(data []byte) ([]Alert, error) {
	alerts, _, err := parseSuricataEVE(data)
	return alerts, err
}

func parseSuricataEVE(data []byte) ([]Alert, *RuleStats, error) {
	var alerts []Alert
	var ruleStats *RuleStats
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		var event eveEvent
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, nil, fmt.Errorf("parse Suricata EVE: %w", err)
		}
		switch event.EventType {
		case "alert":
			alerts = append(alerts, Alert{
				GID: event.Alert.GID,
				SID: event.Alert.SID,
				Rev: event.Alert.Rev,
				Msg: event.Alert.Signature,
			})
		case "stats":
			if len(event.Stats.Detect.Engines) > 0 {
				stats := event.Stats.Detect.Engines[0]
				ruleStats = &RuleStats{Loaded: stats.Loaded, Failed: stats.Failed, Skipped: stats.Skipped}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("read Suricata EVE: %w", err)
	}
	return normalizeAlerts(alerts), ruleStats, nil
}

func ParseSnortJSON(data []byte) ([]Alert, error) {
	var alerts []Alert
	scanner := bufio.NewScanner(bytes.NewReader(data))
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	for scanner.Scan() {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var event snortAlert
		if err := json.Unmarshal(line, &event); err != nil {
			return nil, fmt.Errorf("parse Snort alert JSON: %w", err)
		}
		parts := strings.Split(event.Rule, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("parse Snort rule id %q", event.Rule)
		}
		gid, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("parse Snort gid %q: %w", parts[0], err)
		}
		sid, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("parse Snort sid %q: %w", parts[1], err)
		}
		rev, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("parse Snort rev %q: %w", parts[2], err)
		}
		alerts = append(alerts, Alert{GID: gid, SID: sid, Rev: rev, Msg: event.Msg})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read Snort alert JSON: %w", err)
	}
	return normalizeAlerts(alerts), nil
}

func (config ExternalConfig) RunSuricata(ctx context.Context, rulesPath, pcapPath string) Result {
	result := Result{Engine: EngineSuricata}
	result.Version = commandVersion(ctx, config.Suricata, "--build-info")
	if config.Suricata == "" {
		result.Error = "Suricata executable is not configured"
		return result
	}
	if config.SuricataConfig == "" {
		result.Error = "Suricata configuration is not configured"
		return result
	}

	logDir, err := os.MkdirTemp("", "yak-suricata-diff-")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer os.RemoveAll(logDir)

	cmd := exec.CommandContext(ctx, config.Suricata,
		"-r", pcapPath,
		"-c", config.SuricataConfig,
		"-S", rulesPath,
		"-l", logDir,
		"--runmode", "single",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.ExitCode = commandExitCode(err)
	if err != nil {
		result.Error = err.Error()
		return result
	}

	eve, err := os.ReadFile(filepath.Join(logDir, "eve.json"))
	if err != nil {
		result.Error = fmt.Sprintf("read Suricata EVE output: %v", err)
		return result
	}
	result.Alerts, result.RuleStats, err = parseSuricataEVE(eve)
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func (config ExternalConfig) RunSnort(ctx context.Context, rulesPath, pcapPath string) Result {
	result := Result{Engine: EngineSnort}
	result.Version = commandVersion(ctx, config.Snort, "--dump-version")
	if config.Snort == "" {
		result.Error = "Snort executable is not configured"
		return result
	}

	logDir, err := os.MkdirTemp("", "yak-snort-diff-")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer os.RemoveAll(logDir)

	cmd := exec.CommandContext(ctx, config.Snort,
		"-q",
		"-r", pcapPath,
		"-R", rulesPath,
		"-A", "alert_json",
		"-L", "none",
		"-l", logDir,
		"-k", "none",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.ExitCode = commandExitCode(err)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	result.Alerts, err = ParseSnortJSON(stdout.Bytes())
	if err != nil {
		result.Error = err.Error()
	}
	return result
}

func commandVersion(ctx context.Context, executable string, args ...string) string {
	if executable == "" {
		return ""
	}
	output, err := exec.CommandContext(ctx, executable, args...).Output()
	if err != nil {
		return ""
	}
	line, _, _ := strings.Cut(strings.TrimSpace(string(output)), "\n")
	return line
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return exitError.ExitCode()
	}
	return -1
}
