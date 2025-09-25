package aitag

import (
	"bytes"
	"fmt"
	"io"
	"unicode"

	"github.com/yaklang/yaklang/common/log"
)

// CallbackFunc defines the function signature for tag content handlers
type CallbackFunc func(reader io.Reader)

// TagCallback represents a registered callback for a specific tag and nonce
type TagCallback struct {
	TagName  string
	Nonce    string
	Callback CallbackFunc
}

// Parser holds the configuration for parsing tagged content
type Parser struct {
	callbacks map[string]*TagCallback // key: tagname_nonce
}

// ParseOption defines configuration options for the parser
type ParseOption func(*Parser)

// WithCallback registers a callback for a specific tag name and nonce
func WithCallback(tagName, nonce string, callback CallbackFunc) ParseOption {
	return func(p *Parser) {
		key := fmt.Sprintf("%s_%s", tagName, nonce)
		p.callbacks[key] = &TagCallback{
			TagName:  tagName,
			Nonce:    nonce,
			Callback: callback,
		}
		log.Debugf("registered callback for tag: %s with nonce: %s", tagName, nonce)
	}
}

// NewParser creates a new parser instance
func NewParser(options ...ParseOption) *Parser {
	p := &Parser{
		callbacks: make(map[string]*TagCallback),
	}

	for _, option := range options {
		option(p)
	}

	return p
}

// Parse parses the input stream and triggers callbacks when matching tags are found
func Parse(reader io.Reader, options ...ParseOption) error {
	parser := NewParser(options...)
	return parser.parseStream(reader)
}

// Simple state constants
const (
	stateNormal = iota // Normal content
	stateInTag         // Inside a tag, collecting content
)

// parseStream performs the actual parsing using a simplified approach
// No nesting is supported - this makes the implementation much more reliable
func (p *Parser) parseStream(reader io.Reader) error {
	var currentState = stateNormal
	var activeTag *TagCallback
	var contentBuffer = &bytes.Buffer{}
	var buffer = &bytes.Buffer{}

	// Read entire stream into buffer first for simpler processing
	_, err := buffer.ReadFrom(reader)
	if err != nil {
		return fmt.Errorf("failed to read stream: %w", err)
	}

	content := buffer.String()
	i := 0

	for i < len(content) {
		switch currentState {
		case stateNormal:
			// Look for start tag pattern: <|TAGNAME_NONCE|>
			if i+2 < len(content) && content[i:i+2] == "<|" {
				// Try to parse a start tag
				tagStart := i
				tagEnd := p.findTagEnd(content, i)
				if tagEnd > tagStart {
					tagStr := content[tagStart : tagEnd+1]
					tagName, nonce := p.parseStartTag(tagStr)
					if tagName != "" && nonce != "" {
						// Valid start tag found
						key := fmt.Sprintf("%s_%s", tagName, nonce)
						if callback, exists := p.callbacks[key]; exists {
							log.Debugf("found start tag: %s with nonce: %s", tagName, nonce)
							activeTag = callback
							contentBuffer.Reset()
							currentState = stateInTag
							i = tagEnd + 1
							continue
						} else {
							log.Debugf("no callback registered for tag: %s with nonce: %s", tagName, nonce)
							// Skip the entire tag
							i = tagEnd + 1
							continue
						}
					} else {
						// Invalid tag format, skip the entire potential tag
						i = tagEnd + 1
						continue
					}
				} else {
					// No proper tag end found, skip just the "<|" and continue
					i += 2
					continue
				}
			}
			// Not a tag start, just skip character
			i++

		case stateInTag:
			// Look for matching end tag: <|TAGNAME_END_NONCE|>
			if i+2 < len(content) && content[i:i+2] == "<|" {
				tagStart := i
				tagEnd := p.findTagEnd(content, i)
				if tagEnd > tagStart {
					tagStr := content[tagStart : tagEnd+1]
					tagName, nonce := p.parseEndTag(tagStr)
					if tagName == activeTag.TagName && nonce == activeTag.Nonce {
						// Found matching end tag
						log.Debugf("found end tag: %s with nonce: %s", tagName, nonce)

						// Trigger callback
						contentReader := bytes.NewReader(contentBuffer.Bytes())
						activeTag.Callback(contentReader)

						// Reset state
						activeTag = nil
						contentBuffer.Reset()
						currentState = stateNormal
						i = tagEnd + 1
						continue
					}
				}
			}

			// Not an end tag, add character to content
			contentBuffer.WriteByte(content[i])
			i++
		}
	}

	// Check for unclosed tags
	if activeTag != nil {
		log.Warnf("stream ended with unclosed tag: %s_%s", activeTag.TagName, activeTag.Nonce)
	}

	return nil
}

// findTagEnd finds the end of a tag starting at position start
// It only searches within a reasonable range to avoid matching unrelated |>
func (p *Parser) findTagEnd(content string, start int) int {
	// Find the end of current line to limit search scope
	maxSearch := start + 200 // Reasonable limit for tag length
	if maxSearch > len(content) {
		maxSearch = len(content)
	}

	// Also limit by newlines - tags shouldn't span multiple lines typically
	for i := start; i < maxSearch && i < len(content); i++ {
		if content[i] == '\n' {
			// If we hit a newline without finding |>, this is likely an invalid tag
			maxSearch = i
			break
		}
	}

	for i := start; i < maxSearch-1; i++ {
		if content[i:i+2] == "|>" {
			return i + 1
		}
	}
	return -1
}

// parseStartTag parses a start tag and returns tagName and nonce
// Expected format: <|TAGNAME_NONCE|>
func (p *Parser) parseStartTag(tagStr string) (string, string) {
	if len(tagStr) < 5 || tagStr[:2] != "<|" || tagStr[len(tagStr)-2:] != "|>" {
		return "", ""
	}

	// Remove <| and |>
	inner := tagStr[2 : len(tagStr)-2]

	// Find the last underscore (to separate tag name from nonce)
	underscorePos := -1
	for i := len(inner) - 1; i >= 0; i-- {
		if inner[i] == '_' {
			underscorePos = i
			break
		}
	}

	if underscorePos <= 0 || underscorePos >= len(inner)-1 {
		return "", ""
	}

	tagName := inner[:underscorePos]
	nonce := inner[underscorePos+1:]

	// Validate tag name (letters, digits, underscore allowed)
	for _, ch := range tagName {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return "", ""
		}
	}

	return tagName, nonce
}

// parseEndTag parses an end tag and returns tagName and nonce
// Expected format: <|TAGNAME_END_NONCE|>
func (p *Parser) parseEndTag(tagStr string) (string, string) {
	if len(tagStr) < 9 || tagStr[:2] != "<|" || tagStr[len(tagStr)-2:] != "|>" {
		return "", ""
	}

	// Remove <| and |>
	inner := tagStr[2 : len(tagStr)-2]

	// Must contain "_END_"
	endPos := -1
	for i := 0; i <= len(inner)-4; i++ {
		if inner[i:i+4] == "_END" && i+4 < len(inner) && inner[i+4] == '_' {
			endPos = i
			break
		}
	}

	if endPos <= 0 || endPos+5 >= len(inner) {
		return "", ""
	}

	tagName := inner[:endPos]
	nonce := inner[endPos+5:] // Skip "_END_"

	// Validate tag name (letters, digits, underscore allowed)
	for _, ch := range tagName {
		if !unicode.IsLetter(ch) && !unicode.IsDigit(ch) && ch != '_' {
			return "", ""
		}
	}

	return tagName, nonce
}

// ParseWithCallbacks is a convenience function that allows passing multiple callbacks
func ParseWithCallbacks(reader io.Reader, callbacks map[string]map[string]CallbackFunc) error {
	options := make([]ParseOption, 0, len(callbacks))

	for tagName, nonceCallbacks := range callbacks {
		for nonce, callback := range nonceCallbacks {
			options = append(options, WithCallback(tagName, nonce, callback))
		}
	}

	return Parse(reader, options...)
}
