package inputresolver

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

const maxAndroidManifestBytes = 2 << 20
const maxAndroidEntries = 2048
const maxAndroidExpandedBytes = 128 << 20
const androidNamespace = "http://schemas.android.com/apk/res/android"

// ParseAndroidPackage inspects ZIP metadata and the manifest in process. It
// never extracts files, loads bytecode, verifies signatures or executes an APK.
func (w *Workspace) ParseAndroidPackage(ctx context.Context, name string) (map[string]any, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	data, result, err := w.readBinaryLocked(ctx, name)
	if err != nil {
		return nil, err
	}
	invalid := func() (map[string]any, error) {
		return nil, fail("input_android_invalid", result["resource_id"].(string))
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil || len(archive.File) == 0 || len(archive.File) > maxAndroidEntries {
		return invalid()
	}
	var manifest *zip.File
	var expanded uint64
	seen := map[string]bool{}
	dexCount, nativeCount := 0, 0
	for _, entry := range archive.File {
		if err := w.check(ctx); err != nil {
			return nil, err
		}
		n := entry.Name
		clean := strings.TrimSuffix(n, "/")
		if clean == "" || len(n) > 1024 || strings.ContainsAny(n, "\\\x00:") || strings.HasPrefix(n, "/") || path.Clean(clean) != clean || clean == ".." || strings.HasPrefix(clean, "../") {
			return invalid()
		}
		if entry.Mode()&os.ModeType != 0 && !entry.Mode().IsDir() {
			return invalid()
		}
		if seen[n] || entry.Flags&1 != 0 || entry.UncompressedSize64 > maxAndroidExpandedBytes || expanded > maxAndroidExpandedBytes-entry.UncompressedSize64 {
			return invalid()
		}
		seen[n] = true
		expanded += entry.UncompressedSize64
		if n == "AndroidManifest.xml" {
			manifest = entry
		}
		if strings.HasSuffix(n, ".dex") {
			dexCount++
		}
		if strings.HasPrefix(n, "lib/") && strings.HasSuffix(n, ".so") {
			nativeCount++
		}
	}
	if manifest == nil || manifest.UncompressedSize64 > maxAndroidManifestBytes {
		return invalid()
	}
	reader, err := manifest.Open()
	if err != nil {
		return invalid()
	}
	content, err := io.ReadAll(io.LimitReader(reader, maxAndroidManifestBytes+1))
	closeErr := reader.Close()
	if err != nil || closeErr != nil || len(content) > maxAndroidManifestBytes {
		return invalid()
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	elements, format, err := androidElements(ctx, content)
	if err != nil {
		if ctx.Err() != nil {
			return nil, fail("input_cancelled", result["resource_id"].(string))
		}
		return invalid()
	}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	permissions := []string{}
	metadata := map[string]string{}
	found := false
	for _, e := range elements {
		if e.Name.Space != "" {
			continue
		}
		if e.Name.Local == "manifest" {
			if found {
				return invalid()
			}
			found = true
			for _, a := range e.Attr {
				if (a.Name.Local == "package" && a.Name.Space == "") || ((a.Name.Local == "versionName" || a.Name.Local == "versionCode") && a.Name.Space == androidNamespace) {
					metadata[a.Name.Local] = a.Value
				}
			}
		}
		if e.Name.Local == "uses-sdk" || e.Name.Local == "application" {
			for _, a := range e.Attr {
				if a.Name.Space != androidNamespace {
					continue
				}
				switch a.Name.Local {
				case "minSdkVersion", "targetSdkVersion", "debuggable", "allowBackup", "usesCleartextTraffic":
					metadata[a.Name.Local] = a.Value
				}
			}
		}
		if e.Name.Local == "uses-permission" || e.Name.Local == "uses-permission-sdk-23" {
			for _, a := range e.Attr {
				if a.Name.Local == "name" && a.Name.Space == androidNamespace {
					if len(permissions) >= 512 {
						return invalid()
					}
					permissions = append(permissions, a.Value)
				}
			}
		}
	}
	if !found || metadata["package"] == "" {
		return invalid()
	}
	result["format"] = "apk"
	result["manifest_format"] = format
	result["manifest"] = metadata
	result["permissions"] = permissions
	result["archive_entries"] = len(archive.File)
	result["declared_expanded_bytes"] = expanded
	result["dex_files"] = dexCount
	result["native_libraries"] = nativeCount
	result["limitations"] = []string{"manifest and archive metadata only; no bytecode analysis, execution or signature verification", "resource references remain unresolved; permissions are declarations, not confirmed behavior"}
	if err := w.check(ctx); err != nil {
		return nil, err
	}
	w.event("input.file.access", Event{ResourceID: result["resource_id"].(string), Path: result["path"].(string), Operation: "parse_android_package", BytesRead: int64(len(data))})
	return result, nil
}

func androidElements(ctx context.Context, data []byte) ([]xml.StartElement, string, error) {
	bad := fmt.Errorf("invalid Android manifest")
	result := []xml.StartElement{}
	extractedBytes := 0
	add := func(e xml.StartElement) error {
		if len(result) >= 4096 || len(e.Attr) > 256 || len(e.Name.Local) > 1024 {
			return bad
		}
		for _, a := range e.Attr {
			if len(a.Name.Local) > 1024 || len(a.Value) > 1024 {
				return bad
			}
			extractedBytes += len(a.Name.Local) + len(a.Value)
		}
		extractedBytes += len(e.Name.Local)
		if extractedBytes > 1<<20 {
			return bad
		}
		result = append(result, e)
		return nil
	}
	if len(data) >= 8 && binary.LittleEndian.Uint16(data) == 3 {
		if binary.LittleEndian.Uint16(data[2:]) != 8 || int(binary.LittleEndian.Uint32(data[4:])) != len(data) {
			return nil, "", bad
		}
		var pool []string
		stack := []string{}
		for pos := 8; pos < len(data); {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			if len(data)-pos < 8 {
				return nil, "", bad
			}
			h := int(binary.LittleEndian.Uint16(data[pos+2:]))
			n := int(binary.LittleEndian.Uint32(data[pos+4:]))
			if h < 8 || n < h || n > len(data)-pos {
				return nil, "", bad
			}
			chunk := data[pos : pos+n]
			str := func(index uint32) (string, error) {
				if index >= uint32(len(pool)) {
					return "", bad
				}
				return pool[index], nil
			}
			switch binary.LittleEndian.Uint16(chunk) {
			case 1:
				if pool != nil {
					return nil, "", bad
				}
				var err error
				pool, err = androidStringPool(chunk)
				if err != nil {
					return nil, "", err
				}
			case 0x102:
				if h != 16 || n < 36 {
					return nil, "", bad
				}
				tag, err := str(binary.LittleEndian.Uint32(chunk[20:]))
				if err != nil {
					return nil, "", bad
				}
				if len(stack) == 0 && (len(result) != 0 || tag != "manifest") {
					return nil, "", bad
				}
				start := 16 + int(binary.LittleEndian.Uint16(chunk[24:]))
				size := int(binary.LittleEndian.Uint16(chunk[26:]))
				count := int(binary.LittleEndian.Uint16(chunk[28:]))
				if start < 36 || size < 20 || count > 256 || start > n || count > (n-start)/size {
					return nil, "", bad
				}
				e := xml.StartElement{Name: xml.Name{Local: tag}}
				if namespace := binary.LittleEndian.Uint32(chunk[16:]); namespace != 0xffffffff {
					var err error
					e.Name.Space, err = str(namespace)
					if err != nil {
						return nil, "", bad
					}
				}
				for i := 0; i < count; i++ {
					a := chunk[start+i*size:]
					key, err := str(binary.LittleEndian.Uint32(a[4:]))
					if err != nil {
						return nil, "", bad
					}
					raw := binary.LittleEndian.Uint32(a[8:])
					value := ""
					if raw != 0xffffffff {
						value, err = str(raw)
					} else {
						v := binary.LittleEndian.Uint32(a[16:])
						switch a[15] {
						case 3:
							value, err = str(v)
						case 0x10:
							value = strconv.FormatUint(uint64(v), 10)
						case 0x12:
							value = strconv.FormatBool(v != 0)
						case 1:
							value = fmt.Sprintf("@0x%08x", v)
						default:
							value = fmt.Sprintf("0x%08x", v)
						}
					}
					if err != nil {
						return nil, "", bad
					}
					attributeName := xml.Name{Local: key}
					if namespace := binary.LittleEndian.Uint32(a); namespace != 0xffffffff {
						attributeName.Space, err = str(namespace)
						if err != nil {
							return nil, "", bad
						}
					}
					e.Attr = append(e.Attr, xml.Attr{Name: attributeName, Value: value})
				}
				if err := add(e); err != nil {
					return nil, "", err
				}
				stack = append(stack, tag)
				if len(stack) > 128 {
					return nil, "", bad
				}
			case 0x103:
				if h != 16 || n < 24 || len(stack) == 0 {
					return nil, "", bad
				}
				tag, err := str(binary.LittleEndian.Uint32(chunk[20:]))
				if err != nil || tag != stack[len(stack)-1] {
					return nil, "", bad
				}
				stack = stack[:len(stack)-1]
			case 0x100, 0x101:
				if h != 16 || n < 24 {
					return nil, "", bad
				}
			case 0x180: // resource IDs do not resolve resources.arsc
			default:
				return nil, "", bad
			}
			pos += n
		}
		if len(stack) != 0 || len(result) == 0 {
			return nil, "", bad
		}
		return result, "android_binary_xml", nil
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	depth := 0
	for {
		if ctx.Err() != nil {
			return nil, "", ctx.Err()
		}
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "", bad
		}
		switch e := token.(type) {
		case xml.StartElement:
			if depth == 0 && (len(result) != 0 || e.Name.Local != "manifest") {
				return nil, "", bad
			}
			depth++
			if depth > 128 {
				return nil, "", bad
			}
			if err := add(e); err != nil {
				return nil, "", err
			}
		case xml.EndElement:
			depth--
		case xml.Directive:
			return nil, "", bad
		}
	}
	if len(result) == 0 || depth != 0 {
		return nil, "", bad
	}
	return result, "xml", nil
}

func androidStringPool(data []byte) ([]string, error) {
	bad := fmt.Errorf("invalid Android string pool")
	if len(data) < 28 {
		return nil, bad
	}
	h := int(binary.LittleEndian.Uint16(data[2:]))
	count := int(binary.LittleEndian.Uint32(data[8:]))
	start := int(binary.LittleEndian.Uint32(data[20:]))
	flags := binary.LittleEndian.Uint32(data[16:])
	if h < 28 || h > len(data) || count > 8192 || count > (len(data)-h)/4 || start < h+count*4 || start > len(data) {
		return nil, bad
	}
	result := make([]string, 0, count)
	for i := 0; i < count; i++ {
		off := int(binary.LittleEndian.Uint32(data[h+i*4:]))
		if off >= len(data)-start {
			return nil, bad
		}
		b := data[start+off:]
		if flags&0x100 != 0 {
			length := func() (int, bool) {
				if len(b) < 1 {
					return 0, false
				}
				v := int(b[0])
				b = b[1:]
				if v&0x80 != 0 {
					if len(b) < 1 {
						return 0, false
					}
					v = (v&0x7f)<<8 | int(b[0])
					b = b[1:]
				}
				return v, true
			}
			_, ok := length()
			if !ok {
				return nil, bad
			}
			n, ok := length()
			if !ok || n > 1024 || n >= len(b) || b[n] != 0 || !utf8.Valid(b[:n]) {
				return nil, bad
			}
			result = append(result, string(b[:n]))
		} else {
			if len(b) < 2 {
				return nil, bad
			}
			n := int(binary.LittleEndian.Uint16(b))
			b = b[2:]
			if n&0x8000 != 0 {
				if len(b) < 2 {
					return nil, bad
				}
				n = (n&0x7fff)<<16 | int(binary.LittleEndian.Uint16(b))
				b = b[2:]
			}
			if n > 1024 || n*2+2 > len(b) || binary.LittleEndian.Uint16(b[n*2:]) != 0 {
				return nil, bad
			}
			units := make([]uint16, n)
			for j := range units {
				units[j] = binary.LittleEndian.Uint16(b[j*2:])
			}
			s := string(utf16.Decode(units))
			if len(s) > 1024 {
				return nil, bad
			}
			result = append(result, s)
		}
	}
	return result, nil
}
