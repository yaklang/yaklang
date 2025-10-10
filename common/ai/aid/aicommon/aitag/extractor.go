package aitag

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
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

// parseStream performs the actual parsing using true streaming approach
// No nesting is supported - this makes the implementation much more reliable
func (p *Parser) parseStream(reader io.Reader) error {
	var currentState = stateNormal
	var activeTag *TagCallback
	var contentPipe io.Writer
	var contentReader io.Reader
	var lookAheadBuffer = make([]byte, 1)
	var pendingBytes = &bytes.Buffer{} // Buffer to hold bytes before we write them
	var wg sync.WaitGroup              // Wait for all callbacks to complete
	var skipFirstNewline = false       // Flag to skip first newline after start tag
	var lastFlushTime = time.Now()     // Track last flush time for timeout-based flushing

	// Helper function to flush pending bytes if they form complete UTF-8 characters
	flushPending := func(force bool) {
		if pendingBytes.Len() == 0 {
			return
		}

		data := pendingBytes.Bytes()
		if force {
			// Force flush everything
			contentPipe.Write(data)
			pendingBytes.Reset()
			return
		}

		// Keep last incomplete UTF-8 sequence OR last newline for block text formatting
		// Find the last complete UTF-8 character boundary
		keepBytes := 0

		// Check if we should flush due to timeout (for better streaming performance)
		timeSinceLastFlush := time.Since(lastFlushTime)
		shouldFlushDueToTimeout := timeSinceLastFlush > 200*time.Millisecond

		// First, check if last byte is a newline - if so, keep it for potential removal
		// But only if we haven't exceeded the timeout threshold
		// This enables block text formatting while maintaining streaming performance
		if len(data) > 0 && data[len(data)-1] == '\n' && !shouldFlushDueToTimeout {
			keepBytes = 1
		}

		// Also check for incomplete UTF-8 sequences (max 4 bytes for UTF-8)
		for i := len(data) - 1; i >= 0 && i >= len(data)-4; i-- {
			if utf8.RuneStart(data[i]) {
				// Check if this starts a complete rune
				_, size := utf8.DecodeRune(data[i:])
				if i+size > len(data) {
					// This rune is incomplete, keep it
					incomplete := len(data) - i
					if incomplete > keepBytes {
						keepBytes = incomplete
					}
				}
				break
			}
		}

		if keepBytes >= len(data) {
			// All bytes should be kept
			return
		}

		// Write all complete UTF-8 characters (except potentially last newline)
		toWrite := data[:len(data)-keepBytes]
		if len(toWrite) > 0 {
			contentPipe.Write(toWrite)
			lastFlushTime = time.Now() // Update flush time
			// Keep incomplete sequence and/or trailing newline
			pendingBytes.Reset()
			if keepBytes > 0 {
				pendingBytes.Write(data[len(data)-keepBytes:])
			}
		}
	}

	// For streaming, we need to process byte by byte
	for {
		n, err := reader.Read(lookAheadBuffer)
		if n > 0 {
			ch := lookAheadBuffer[0]

			switch currentState {
			case stateNormal:
				// Look for start tag pattern: <|
				pendingBytes.WriteByte(ch)

				// Check if we have a potential tag start
				if pendingBytes.Len() >= 2 {
					tail := pendingBytes.Bytes()[pendingBytes.Len()-2:]
					if string(tail) == "<|" {
						// We have a potential tag, need to scan until we find |>
						tagBuffer := &bytes.Buffer{}
						tagBuffer.WriteString("<|")

						// Scan until we find |> or newline or reach reasonable limit
						found := false
						for i := 0; i < 200; i++ {
							n, err := reader.Read(lookAheadBuffer)
							if n > 0 {
								tagBuffer.WriteByte(lookAheadBuffer[0])

								// Check for end of tag
								if tagBuffer.Len() >= 4 {
									lastTwo := tagBuffer.Bytes()[tagBuffer.Len()-2:]
									if string(lastTwo) == "|>" {
										found = true
										break
									}
								}

								// Stop at newline (tags shouldn't span multiple lines)
								if lookAheadBuffer[0] == '\n' {
									break
								}
							}
							if err != nil {
								if err == io.EOF {
									break
								}
								return fmt.Errorf("failed to read stream: %w", err)
							}
						}

						if found {
							tagStr := tagBuffer.String()
							tagName, nonce := p.parseStartTag(tagStr)
							if tagName != "" && nonce != "" {
								key := fmt.Sprintf("%s_%s", tagName, nonce)
								if callback, exists := p.callbacks[key]; exists {
									log.Debugf("found start tag: %s with nonce: %s", tagName, nonce)
									activeTag = callback
									currentState = stateInTag

									// Create buffered pipe for streaming content to callback
									// This avoids blocking on Write calls
									pr, pw := utils.NewPipe()
									contentReader = pr
									contentPipe = pw

									// Start callback in goroutine
									wg.Add(1)
									go func(cb CallbackFunc, r io.Reader) {
										defer wg.Done()
										cb(r)
									}(callback.Callback, contentReader)

									pendingBytes.Reset()
									skipFirstNewline = true // Skip first newline for block text formatting
									continue
								} else {
									log.Debugf("no callback registered for tag: %s with nonce: %s", tagName, nonce)
								}
							}
						}

						// If we didn't find a valid start tag, continue scanning
						// Reset and keep the unmatched tag content
						pendingBytes.Reset()
						pendingBytes.Write(tagBuffer.Bytes())
					}
				}

			case stateInTag:
				// We're inside a tag, need to stream content to callback
				// But we also need to watch for end tag

				// For block text formatting: skip first newline after start tag
				if skipFirstNewline {
					skipFirstNewline = false
					if ch == '\n' {
						// Skip this newline
						continue
					}
					// Not a newline, process it normally
				}

				// Strategy: buffer a small amount to detect end tags
				// If we see <|, we need to lookahead to see if it's an end tag

				if ch == '<' {
					// Potential start of end tag, peek ahead
					// From now on, buffer content instead of writing immediately
					// This allows us to remove trailing newline if this is indeed an end tag
					pendingBytes.WriteByte(ch)

					nextByte := make([]byte, 1)
					n, err := reader.Read(nextByte)
					if n > 0 {
						pendingBytes.WriteByte(nextByte[0])
						if nextByte[0] == '|' {
							// We have <|, now scan for the full end tag
							tagBuffer := &bytes.Buffer{}
							tagBuffer.WriteString("<|")

							found := false
							for i := 0; i < 200; i++ {
								n, err := reader.Read(lookAheadBuffer)
								if n > 0 {
									tagBuffer.WriteByte(lookAheadBuffer[0])
									pendingBytes.WriteByte(lookAheadBuffer[0])

									if tagBuffer.Len() >= 4 {
										lastTwo := tagBuffer.Bytes()[tagBuffer.Len()-2:]
										if string(lastTwo) == "|>" {
											found = true
											break
										}
									}

									if lookAheadBuffer[0] == '\n' {
										break
									}
								}
								if err != nil {
									if err == io.EOF {
										break
									}
									return fmt.Errorf("failed to read stream: %w", err)
								}
							}

							if found {
								tagStr := tagBuffer.String()
								tagName, nonce := p.parseEndTag(tagStr)
								if tagName == activeTag.TagName && nonce == activeTag.Nonce {
									// Found matching end tag!
									log.Debugf("found end tag: %s with nonce: %s", tagName, nonce)

									// Write any pending bytes (everything before the end tag)
									// For block text formatting: remove trailing newline before end tag
									if pendingBytes.Len() > 0 {
										content := pendingBytes.Bytes()
										// Remove the end tag itself from content
										tagLen := len(tagStr)
										if len(content) >= tagLen {
											content = content[:len(content)-tagLen]
											// Now check if the last byte before end tag is a newline
											if len(content) > 0 && content[len(content)-1] == '\n' {
												// Remove the trailing newline
												content = content[:len(content)-1]
											}
										}
										if len(content) > 0 {
											contentPipe.Write(content)
										}
										pendingBytes.Reset()
									}

									// Close the pipe to signal end of content
									if closer, ok := contentPipe.(io.Closer); ok {
										closer.Close()
									}

									// Reset state
									activeTag = nil
									contentPipe = nil
									contentReader = nil
									currentState = stateNormal
									continue
								}
							}

							// Not a matching end tag, write everything as content
							if pendingBytes.Len() > 0 {
								contentPipe.Write(pendingBytes.Bytes())
								pendingBytes.Reset()
							}

						} else {
							// Not a tag start, write pending content (includes <ch>)
							contentPipe.Write(pendingBytes.Bytes())
							pendingBytes.Reset()
						}
					} else if err == io.EOF {
						// EOF after '<', write pending content
						contentPipe.Write(pendingBytes.Bytes())
						pendingBytes.Reset()
					}
				} else {
					// Regular character, buffer and flush on UTF-8 boundaries
					pendingBytes.WriteByte(ch)
					// Flush complete UTF-8 characters, keeping incomplete sequences
					flushPending(false)
				}
			}
		}

		if err != nil {
			if err == io.EOF {
				// Handle end of stream
				if currentState == stateInTag {
					// Flush remaining content
					if pendingBytes.Len() > 0 {
						contentPipe.Write(pendingBytes.Bytes())
					}
					if closer, ok := contentPipe.(io.Closer); ok {
						closer.Close()
					}
					log.Warnf("stream ended with unclosed tag: %s_%s", activeTag.TagName, activeTag.Nonce)
				}
				break
			}
			return fmt.Errorf("failed to read stream: %w", err)
		}
	}

	// Wait for all callbacks to complete
	wg.Wait()

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
