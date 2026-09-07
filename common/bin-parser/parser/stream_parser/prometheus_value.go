package stream_parser

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type PrometheusLabel struct {
	Name  string
	Value string
}

// PrometheusLine exposes every text-format field without discarding source
// spelling, label order, comments, blank lines or exceptional float values.
type PrometheusLine struct {
	Text          string
	Kind          string
	Name          string
	Help          string
	MetricType    string
	Labels        []PrometheusLabel
	ValueText     string
	Value         float64
	HasTimestamp  bool
	Timestamp     int64
	TimestampText string
}

type prometheusCursor struct {
	text string
	pos  int
}

func (c *prometheusCursor) blanks() int {
	start := c.pos
	for c.pos < len(c.text) && (c.text[c.pos] == ' ' || c.text[c.pos] == '\t') {
		c.pos++
	}
	return c.pos - start
}

func (c *prometheusCursor) take(b byte) bool {
	if c.pos == len(c.text) || c.text[c.pos] != b {
		return false
	}
	c.pos++
	return true
}

func (c *prometheusCursor) token() string {
	start := c.pos
	for c.pos < len(c.text) && c.text[c.pos] != ' ' && c.text[c.pos] != '\t' {
		c.pos++
	}
	return c.text[start:c.pos]
}

func (c *prometheusCursor) quoted() (string, error) {
	if !c.take('"') {
		return "", fmt.Errorf("prometheus: missing quoted value")
	}
	var value strings.Builder
	for c.pos < len(c.text) {
		b := c.text[c.pos]
		c.pos++
		if b == '"' {
			return value.String(), nil
		}
		if b == '\\' {
			if c.pos == len(c.text) {
				break
			}
			b = c.text[c.pos]
			c.pos++
			switch b {
			case '\\', '"':
			case 'n':
				b = '\n'
			default:
				return "", fmt.Errorf("prometheus: invalid quoted escape")
			}
		}
		value.WriteByte(b)
	}
	return "", fmt.Errorf("prometheus: unterminated quoted value")
}

func (c *prometheusCursor) name(label bool, quoted bool) (string, error) {
	if quoted && c.pos < len(c.text) && c.text[c.pos] == '"' {
		name, err := c.quoted()
		if err != nil || name == "" {
			return "", fmt.Errorf("prometheus: invalid quoted name")
		}
		return name, nil
	}
	start := c.pos
	for c.pos < len(c.text) {
		b := c.text[c.pos]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' || !label && b == ':' || c.pos > start && b >= '0' && b <= '9' {
			c.pos++
		} else {
			break
		}
	}
	if c.pos == start {
		return "", fmt.Errorf("prometheus: invalid identifier")
	}
	return c.text[start:c.pos], nil
}

func prometheusHelp(text string) (string, error) {
	var value strings.Builder
	for i := 0; i < len(text); i++ {
		b := text[i]
		if b == '\\' {
			i++
			if i == len(text) {
				return "", fmt.Errorf("prometheus: incomplete HELP escape")
			}
			b = text[i]
			if b == 'n' {
				b = '\n'
			} else if b != '\\' {
				return "", fmt.Errorf("prometheus: invalid HELP escape")
			}
		}
		value.WriteByte(b)
	}
	return value.String(), nil
}

func parsePrometheusLine(text string) (*PrometheusLine, error) {
	line := &PrometheusLine{Text: text}
	c := &prometheusCursor{text: text}
	c.blanks()
	if c.pos == len(text) {
		line.Kind = "blank"
		return line, nil
	}
	if c.take('#') {
		line.Kind = "comment"
		c.blanks()
		kind := c.token()
		if kind != "HELP" && kind != "TYPE" {
			return line, nil
		}
		line.Kind = strings.ToLower(kind)
		c.blanks()
		var err error
		line.Name, err = c.name(false, true)
		if err != nil {
			return nil, err
		}
		if c.pos < len(text) && c.blanks() == 0 {
			return nil, fmt.Errorf("prometheus: missing metadata separator")
		}
		if kind == "HELP" {
			line.Help, err = prometheusHelp(strings.TrimRight(text[c.pos:], " \t"))
			return line, err
		}
		line.MetricType = c.token()
		c.blanks()
		switch line.MetricType {
		case "counter", "gauge", "histogram", "summary", "untyped":
		default:
			return nil, fmt.Errorf("prometheus: unknown TYPE")
		}
		if c.pos != len(text) {
			return nil, fmt.Errorf("prometheus: extra TYPE fields")
		}
		return line, nil
	}
	line.Kind = "sample"
	var err error
	braced := c.take('{')
	if braced {
		c.blanks()
		line.Name, err = c.quoted()
		if err != nil || line.Name == "" {
			return nil, fmt.Errorf("prometheus: missing quoted metric name inside braces")
		}
		c.blanks()
		if c.pos < len(text) && text[c.pos] != '}' && !c.take(',') {
			return nil, fmt.Errorf("prometheus: missing metric/label separator")
		}
	} else {
		line.Name, err = c.name(false, false)
		if err != nil {
			return nil, err
		}
		// Whitespace before a label set is legal. It also separates an
		// unlabelled metric name from its sample value.
		separator := c.blanks()
		braced = c.take('{')
		if !braced && separator == 0 {
			return nil, fmt.Errorf("prometheus: missing sample value separator")
		}
	}
	if braced {
		seen := map[string]bool{}
		for {
			c.blanks()
			if c.take('}') {
				break
			}
			name, err := c.name(true, true)
			if err != nil || name == "__name__" || seen[name] {
				return nil, fmt.Errorf("prometheus: duplicate, reserved or invalid label name")
			}
			seen[name] = true
			c.blanks()
			if !c.take('=') {
				return nil, fmt.Errorf("prometheus: missing label equals sign")
			}
			c.blanks()
			value, err := c.quoted()
			if err != nil {
				return nil, err
			}
			line.Labels = append(line.Labels, PrometheusLabel{Name: name, Value: value})
			c.blanks()
			if c.take('}') {
				break
			}
			if !c.take(',') {
				return nil, fmt.Errorf("prometheus: missing label separator or closing brace")
			}
		}
	}
	c.blanks()
	line.ValueText = c.token()
	line.Value, err = strconv.ParseFloat(line.ValueText, 64)
	if err != nil {
		return nil, fmt.Errorf("prometheus: invalid sample value")
	}
	c.blanks()
	if c.pos < len(text) {
		line.HasTimestamp = true
		line.TimestampText = c.token()
		line.Timestamp, err = strconv.ParseInt(line.TimestampText, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("prometheus: invalid millisecond timestamp")
		}
		c.blanks()
	}
	if c.pos != len(text) {
		return nil, fmt.Errorf("prometheus: trailing sample fields")
	}
	return line, nil
}

// decodePrometheusText reads the Prometheus text exposition format, not
// OpenMetrics or protobuf. Histogram/summary components stay explicit samples;
// this decoder does not infer or validate cross-sample aggregate arithmetic.
func decodePrometheusText(text string, lineLimit int) ([]*PrometheusLine, error) {
	if len(text) > 1<<20 || !utf8.ValidString(text) || lineLimit < 1 || lineLimit > 1<<20 {
		return nil, fmt.Errorf("prometheus: invalid input size, encoding or line limit")
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		return nil, fmt.Errorf("prometheus: missing final line feed")
	}
	var lines []*PrometheusLine
	types, help, samples := map[string]bool{}, map[string]bool{}, map[string]bool{}
	series := map[string]bool{}
	for text != "" {
		end := strings.IndexByte(text, '\n')
		if end+1 > lineLimit || len(lines) >= 100000 {
			return nil, fmt.Errorf("prometheus: line or record limit exceeded")
		}
		line, err := parsePrometheusLine(text[:end])
		if err != nil {
			return nil, fmt.Errorf("prometheus: line %d: %w", len(lines)+1, err)
		}
		switch line.Kind {
		case "help":
			if help[line.Name] || samples[line.Name] {
				return nil, fmt.Errorf("prometheus: duplicate or late HELP")
			}
			help[line.Name] = true
		case "type":
			if types[line.Name] || samples[line.Name] {
				return nil, fmt.Errorf("prometheus: duplicate or late TYPE")
			}
			types[line.Name] = true
		case "sample":
			samples[line.Name] = true
			labels := append([]PrometheusLabel(nil), line.Labels...)
			sort.Slice(labels, func(i, j int) bool { return labels[i].Name < labels[j].Name })
			var key strings.Builder
			key.WriteString(strconv.Quote(line.Name))
			for _, label := range labels {
				key.WriteString(strconv.Quote(label.Name))
				key.WriteString(strconv.Quote(label.Value))
			}
			if series[key.String()] {
				return nil, fmt.Errorf("prometheus: duplicate sample series")
			}
			series[key.String()] = true
		}
		lines = append(lines, line)
		text = text[end+1:]
	}
	return lines, nil
}
