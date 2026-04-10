package aicommon

import (
	"strings"

	"github.com/yaklang/yaklang/common/ai/ytoken"
	"github.com/yaklang/yaklang/common/utils"
)

// ContentMeasurer abstracts content size measurement.
// All context budget checks in the aid/aireact system go through this interface,
// allowing a unified switch from byte-based to token-based accounting.
type ContentMeasurer interface {
	Measure(text string) int
	Shrink(text string, limit int) string
	ShrinkTextBlock(text string, limit int) string
}

// --- TokenMeasurer ---

type tokenMeasurer struct{}

var defaultTokenMeasurer ContentMeasurer = &tokenMeasurer{}

func NewTokenMeasurer() ContentMeasurer { return defaultTokenMeasurer }

func (t *tokenMeasurer) Measure(text string) int {
	return ytoken.CalcTokenCount(text)
}

func (t *tokenMeasurer) Shrink(text string, limit int) string {
	return shrinkByTokens(text, limit, false)
}

func (t *tokenMeasurer) ShrinkTextBlock(text string, limit int) string {
	return shrinkByTokens(text, limit, true)
}

// --- BytesMeasurer (legacy / fallback) ---

type bytesMeasurer struct{}

var defaultBytesMeasurer ContentMeasurer = &bytesMeasurer{}

func NewBytesMeasurer() ContentMeasurer { return defaultBytesMeasurer }

func (b *bytesMeasurer) Measure(text string) int {
	return len(text)
}

func (b *bytesMeasurer) Shrink(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 3 {
		if limit < 0 {
			limit = 0
		}
		return text[:limit]
	}
	return text[:limit-3] + "..."
}

func (b *bytesMeasurer) ShrinkTextBlock(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	if limit <= 6 {
		if limit < 0 {
			limit = 0
		}
		return text[:limit]
	}
	half := limit / 2
	return text[:half] + "..." + text[len(text)-half:]
}

// --- Token-based shrink implementation ---

func shrinkByTokens(text string, limit int, keepTail bool) string {
	if limit <= 0 {
		return ""
	}

	tokens := ytoken.Encode(text)
	if len(tokens) <= limit {
		return text
	}

	ellipsis := "..."
	ellipsisTokens := ytoken.Encode(ellipsis)
	reserveForEllipsis := len(ellipsisTokens)

	if keepTail {
		available := limit - reserveForEllipsis
		if available <= 0 {
			return ytoken.Decode(tokens[:limit])
		}
		half := available / 2
		tail := available - half
		head := ytoken.Decode(tokens[:half])
		end := ytoken.Decode(tokens[len(tokens)-tail:])
		return head + ellipsis + end
	}

	available := limit - reserveForEllipsis
	if available <= 0 {
		return ytoken.Decode(tokens[:limit])
	}
	return ytoken.Decode(tokens[:available]) + ellipsis
}

// --- Package-level helpers ---

// MeasureTokens returns the token count of text.
func MeasureTokens(text string) int {
	return ytoken.CalcTokenCount(text)
}

// ShrinkByTokens truncates text to fit within tokenLimit tokens (head only).
func ShrinkByTokens(text string, tokenLimit int) string {
	return shrinkByTokens(text, tokenLimit, false)
}

// ShrinkTextBlockByTokens truncates text keeping head+tail within tokenLimit tokens.
func ShrinkTextBlockByTokens(text string, tokenLimit int) string {
	return shrinkByTokens(text, tokenLimit, true)
}

// ShrinkStringByTokens matches the codec.ShrinkString pattern but uses token counts.
// Single-line output (newlines collapsed).
func ShrinkStringByTokens(r any, tokenLimit int) string {
	text := strings.TrimSpace(utils.InterfaceToString(r))
	result := shrinkByTokens(text, tokenLimit, false)
	result = strings.ReplaceAll(result, "\r", " ")
	result = strings.ReplaceAll(result, "\n", " ")
	result = strings.ReplaceAll(result, "\t", " ")
	return result
}

// ShrinkTextBlockByTokensAny matches the codec.ShrinkTextBlock pattern but uses token counts.
// Multiline preserved.
func ShrinkTextBlockByTokensAny(r any, tokenLimit int) string {
	text := strings.TrimSpace(utils.InterfaceToString(r))
	return shrinkByTokens(text, tokenLimit, true)
}
