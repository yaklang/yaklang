package rule

import (
	"fmt"
	"strconv"
	"strings"
)

type ThresholdingConfig struct {
	Keyword       string `json:"keyword,omitempty"`
	Raw           string `json:"raw,omitempty"`
	Type          string `json:"type,omitempty"`
	ThresholdMode bool   `json:"threshold_mode,omitempty"`
	LimitMode     bool   `json:"limit_mode,omitempty"`
	Count         int    `json:"count"`
	Seconds       int    `json:"seconds"`
	Track         string `json:"track"`
}

func parseThresholding(keyword, raw string, parts []string) (*ThresholdingConfig, error) {
	config := &ThresholdingConfig{Keyword: keyword, Raw: raw}
	seen := make(map[string]bool)
	for _, part := range parts {
		fields := strings.Fields(strings.TrimSpace(part))
		if len(fields) != 2 {
			return nil, fmt.Errorf("%s field %q must use name value syntax", keyword, part)
		}
		name, value := strings.ToLower(fields[0]), strings.ToLower(fields[1])
		if seen[name] {
			return nil, fmt.Errorf("%s field %q is repeated", keyword, name)
		}
		seen[name] = true
		switch name {
		case "type":
			if keyword == "detection_filter" {
				return nil, fmt.Errorf("detection_filter does not accept a type field")
			}
			config.Type = value
			switch value {
			case "both":
				config.ThresholdMode = true
				config.LimitMode = true
			case "threshold":
				config.ThresholdMode = true
			case "limit":
				config.LimitMode = true
			default:
				return nil, fmt.Errorf("invalid threshold type %q", value)
			}
		case "track":
			switch value {
			case "by_src", "by_dst", "by_rule", "by_both":
				config.Track = value
			default:
				return nil, fmt.Errorf("invalid %s track %q", keyword, value)
			}
		case "count":
			count, err := strconv.Atoi(value)
			if err != nil || count <= 0 {
				return nil, fmt.Errorf("invalid %s count %q", keyword, value)
			}
			config.Count = count
		case "seconds":
			seconds, err := strconv.Atoi(value)
			if err != nil || seconds <= 0 {
				return nil, fmt.Errorf("invalid %s seconds %q", keyword, value)
			}
			config.Seconds = seconds
		default:
			return nil, fmt.Errorf("unknown %s field %q", keyword, name)
		}
	}
	if keyword == "threshold" && !seen["type"] {
		return nil, fmt.Errorf("threshold requires type")
	}
	for _, required := range []string{"track", "count", "seconds"} {
		if !seen[required] {
			return nil, fmt.Errorf("%s requires %s", keyword, required)
		}
	}
	return config, nil
}

func (t *ThresholdingConfig) Repeat() int {
	if t == nil {
		return 1
	}

	if t.Count > 0 {
		return t.Count
	}

	return 1
}
