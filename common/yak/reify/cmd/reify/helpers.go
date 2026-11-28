package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/yak/reify/core"
)

func splitPassNames(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if name := strings.TrimSpace(part); name != "" {
			result = append(result, name)
		}
	}
	return result
}

func targetSummary(target core.Target) string {
	summary := fmt.Sprintf("%s/%s, %d-bit, %s-endian", target.OS, target.Architecture, target.Bits, target.ByteOrder)
	if target.ABI != "" {
		summary += ", abi " + target.ABI
	}
	if target.Machine != "" {
		summary += ", machine " + target.Machine
	}
	if target.Mode != "" {
		summary += ", mode " + target.Mode
	}
	return summary
}

func parserSummary(parser *core.ParserInfo) string {
	if parser == nil {
		return "none"
	}
	return fmt.Sprintf("%s (%s)", parser.Name, decoderState(parser.Available))
}

func decoderState(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func writeJSON(w io.Writer, value any) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeIndentedJSONValue(bw *bufio.Writer, buf *bytes.Buffer, prefix string, value any) error {
	buf.Reset()
	encoder := json.NewEncoder(buf)
	encoder.SetIndent(prefix, "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	raw := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	_, err := bw.Write(raw)
	return err
}

func writeJSONArray[T any](bw *bufio.Writer, buf *bytes.Buffer, keyPrefix, elemIndent string, key string, items []T, omitempty bool) (bool, error) {
	if omitempty && len(items) == 0 {
		return false, nil
	}
	if _, err := fmt.Fprintf(bw, ",\n%s%q: ", keyPrefix, key); err != nil {
		return true, err
	}
	if items == nil {
		_, err := bw.WriteString("null")
		return true, err
	}
	if len(items) == 0 {
		_, err := bw.WriteString("[]")
		return true, err
	}
	if _, err := bw.WriteString("[\n"); err != nil {
		return true, err
	}
	for i := range items {
		if _, err := bw.WriteString(elemIndent); err != nil {
			return true, err
		}
		if err := writeIndentedJSONValue(bw, buf, elemIndent, &items[i]); err != nil {
			return true, err
		}
		if i+1 < len(items) {
			if _, err := bw.WriteString(",\n"); err != nil {
				return true, err
			}
		} else {
			if err := bw.WriteByte('\n'); err != nil {
				return true, err
			}
		}
	}
	if _, err := fmt.Fprintf(bw, "%s]", keyPrefix); err != nil {
		return true, err
	}
	return true, nil
}

// writeInspectionJSON streams inspectionReport into w using bounded memory so
// large symbol/relocation tables do not double-buffer hundreds of megabytes.
func writeInspectionJSON(w io.Writer, report inspectionReport) error {
	if report.Artifact == nil {
		return writeJSON(w, report)
	}
	bw := bufio.NewWriterSize(w, 256*1024)
	var buf bytes.Buffer

	if _, err := bw.WriteString("{\n  \"scope\": "); err != nil {
		return err
	}
	if err := writeIndentedJSONValue(bw, &buf, "  ", report.Scope); err != nil {
		return err
	}
	art := report.Artifact
	if _, err := bw.WriteString(",\n  \"artifact\": {\n    \"origin\": "); err != nil {
		return err
	}
	if err := writeIndentedJSONValue(bw, &buf, "    ", art.Origin); err != nil {
		return err
	}
	if _, err := bw.WriteString(",\n    \"kind\": "); err != nil {
		return err
	}
	if err := writeIndentedJSONValue(bw, &buf, "    ", art.Kind); err != nil {
		return err
	}
	if _, err := bw.WriteString(",\n    \"format\": "); err != nil {
		return err
	}
	if err := writeIndentedJSONValue(bw, &buf, "    ", art.Format); err != nil {
		return err
	}
	if _, err := bw.WriteString(",\n    \"target\": "); err != nil {
		return err
	}
	if err := writeIndentedJSONValue(bw, &buf, "    ", art.Target); err != nil {
		return err
	}
	if img := art.Image; img != nil {
		if _, err := bw.WriteString(",\n    \"image\": {\n      \"object_type\": "); err != nil {
			return err
		}
		if err := writeIndentedJSONValue(bw, &buf, "      ", img.ObjectType); err != nil {
			return err
		}
		if img.Base != 0 {
			if _, err := bw.WriteString(",\n      \"image_base\": "); err != nil {
				return err
			}
			if err := writeIndentedJSONValue(bw, &buf, "      ", img.Base); err != nil {
				return err
			}
		}
		if img.Entry != nil {
			if _, err := bw.WriteString(",\n      \"entry_point\": "); err != nil {
				return err
			}
			if err := writeIndentedJSONValue(bw, &buf, "      ", img.Entry); err != nil {
				return err
			}
		}
		if _, err := writeJSONArray(bw, &buf, "      ", "        ", "sections", img.Sections, false); err != nil {
			return err
		}
		if _, err := writeJSONArray(bw, &buf, "      ", "        ", "symbols", img.Symbols, true); err != nil {
			return err
		}
		if _, err := writeJSONArray(bw, &buf, "      ", "        ", "relocations", img.Relocations, true); err != nil {
			return err
		}
		if _, err := bw.WriteString("\n    }"); err != nil {
			return err
		}
	}
	if _, err := writeJSONArray(bw, &buf, "    ", "      ", "diagnostics", art.Diagnostics, true); err != nil {
		return err
	}
	if art.Language != nil {
		if _, err := bw.WriteString(",\n    \"language\": "); err != nil {
			return err
		}
		if err := writeIndentedJSONValue(bw, &buf, "    ", art.Language); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("\n  }"); err != nil {
		return err
	}
	if report.Parser != nil {
		if _, err := bw.WriteString(",\n  \"parser\": "); err != nil {
			return err
		}
		if err := writeIndentedJSONValue(bw, &buf, "  ", report.Parser); err != nil {
			return err
		}
	}
	if _, err := bw.WriteString("\n}\n"); err != nil {
		return err
	}
	return bw.Flush()
}
