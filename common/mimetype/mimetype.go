// Package mimetype uses magic number signatures to detect the MIME type of a file.
//
// File formats are stored in a hierarchy with application/octet-stream at its root.
// For example, the hierarchy for HTML format is application/octet-stream ->
// text/plain -> text/html.
package mimetype

import (
	"io"
	"mime"
	"os"
	"sync/atomic"
	"unicode/utf8"
)

var defaultLimit uint32 = 3072

// readLimit is the maximum number of bytes from the input used when detecting.
var readLimit uint32 = defaultLimit

// Detect returns the MIME type found from the provided byte slice.
//
// The result is always a valid MIME type, with application/octet-stream
// returned when identification failed.
func Detect(raw []byte) *MIME {
	// Using atomic because readLimit can be written at the same time in other goroutine.
	l := atomic.LoadUint32(&readLimit)

	in := raw
	if l > 0 && len(in) > int(l) {
		in = in[:l]
	}

	// try to ensure the boundary is a complete UTF-8 character
	// dont cut bad
	if uint32(len(raw)) > l {
		count := 0
		for {
			count++
			if count > 4 {
				break
			}
			last, size := utf8.DecodeLastRune(in)
			if last == utf8.RuneError {
				in = raw[:len(in)-size]
			} else {
				break
			}
		}
	}

	mu.RLock()
	defer mu.RUnlock()

	mimeType := root.match(in, l)
	// is utf8
	if mimeType.NeedCharset() && mimeType.Charset() == "utf-8" {
		if ret := uint32(len(raw)); ret > l && ret <= 1024*1024 {
			var newSampleIndex uint32 = 0
			for _idx := l; _idx < ret; {
				r, size := utf8.DecodeRune(raw[_idx:])
				if r == utf8.RuneError {
					newSampleIndex = _idx
					break
				}
				_idx += uint32(size)
			}
			if newSampleIndex > 0 {
				in = raw[newSampleIndex:]
				mimeType = root.match(in, 1024*1024)
			}
		}
	}
	return mimeType
}

// DetectReader returns the MIME type of the provided reader.
//
// The result is always a valid MIME type, with application/octet-stream
// returned when identification failed with or without an error.
// Any error returned is related to the reading from the input reader.
//
// DetectReader assumes the reader offset is at the start. If the input is an
// io.ReadSeeker you previously read from, it should be rewinded before detection:
//
//	reader.Seek(0, io.SeekStart)
func DetectReader(r io.Reader) (*MIME, error) {
	var in []byte
	var err error

	// Using atomic because readLimit can be written at the same time in other goroutine.
	l := atomic.LoadUint32(&readLimit)
	if l == 0 {
		in, err = io.ReadAll(r)
		if err != nil {
			return errMIME, err
		}
	} else {
		var n int
		in = make([]byte, l)
		// io.UnexpectedEOF means len(r) < len(in). It is not an error in this case,
		// it just means the input file is smaller than the allocated bytes slice.
		n, err = io.ReadFull(r, in)
		if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
			return errMIME, err
		}
		in = in[:n]
	}

	mu.RLock()
	defer mu.RUnlock()
	return root.match(in, l), nil
}

// DetectFile returns the MIME type of the provided file.
//
// The result is always a valid MIME type, with application/octet-stream
// returned when identification failed with or without an error.
// Any error returned is related to the opening and reading from the input file.
func DetectFile(path string) (*MIME, error) {
	f, err := os.Open(path)
	if err != nil {
		return errMIME, err
	}
	defer f.Close()

	return DetectReader(f)
}

// EqualsAny reports whether s MIME type is equal to any MIME type in mimes.
// MIME type equality test is done on the "type/subtype" section, ignores
// any optional MIME parameters, ignores any leading and trailing whitespace,
// and is case insensitive.
func EqualsAny(s string, mimes ...string) bool {
	s, _, _ = mime.ParseMediaType(s)
	for _, m := range mimes {
		m, _, _ = mime.ParseMediaType(m)
		if s == m {
			return true
		}
	}

	return false
}

// SetLimit sets the maximum number of bytes read from input when detecting the MIME type.
// Increasing the limit provides better detection for file formats which store
// their magical numbers towards the end of the file: docx, pptx, xlsx, etc.
// During detection data is read in a single block of size limit, i.e. it is not buffered.
// A limit of 0 means the whole input file will be used.
func SetLimit(limit uint32) {
	// Using atomic because readLimit can be read at the same time in other goroutine.
	atomic.StoreUint32(&readLimit, limit)
}

// GetLimit returns the maximum number of bytes read from input when detecting the MIME type.
func GetLimit() int {
	// Using atomic because readLimit can be read at the same time in other goroutine.
	return int(atomic.LoadUint32(&readLimit))
}

// Extend adds detection for other file formats.
// It is equivalent to calling Extend() on the root mime type "application/octet-stream".
func Extend(detector func(raw []byte, limit uint32) bool, mime, extension string, aliases ...string) {
	root.Extend(detector, mime, extension, aliases...)
}

// Lookup finds a MIME object by its string representation.
// The representation can be the main mime type, or any of its aliases.
func Lookup(mime string) *MIME {
	mu.RLock()
	defer mu.RUnlock()
	return root.lookup(mime)
}
