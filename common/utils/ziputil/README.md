# ZipUtil Package - Enhanced ZIP File Operations

## Overview

Enhanced ZIP utility package with advanced grep and extraction capabilities, supporting concurrent processing for high performance.

## Features

### Basic Operations
- ✅ Compress files and directories to ZIP
- ✅ Decompress ZIP archives
- ✅ Recursive traversal of ZIP contents
- ✅ Raw data compression/decompression

### Advanced Features (New)
- 🔍 **Grep Functionality**: Search ZIP contents with regex or substring
- 📦 **File Extraction**: Extract specific files or patterns from ZIP
- ⚡ **Concurrent Processing**: Parallel extraction for better performance
- 🎯 **Pattern Matching**: Wildcard-based file selection
- 📝 **Context Support**: Show surrounding lines for grep results

## API Reference

### Grep Functions

#### GrepRegexp
```go
func GrepRegexp(zipFile string, pattern string, opts ...GrepOption) ([]*GrepResult, error)
```
Search ZIP file using regular expressions.

#### GrepSubString
```go
func GrepSubString(zipFile string, substring string, opts ...GrepOption) ([]*GrepResult, error)
```
Search ZIP file using substring (case-insensitive by default).

#### GrepRawRegexp
```go
func GrepRawRegexp(raw interface{}, pattern string, opts ...GrepOption) ([]*GrepResult, error)
```
Search ZIP raw data using regular expressions.

#### GrepRawSubString
```go
func GrepRawSubString(raw interface{}, substring string, opts ...GrepOption) ([]*GrepResult, error)
```
Search ZIP raw data using substring.

### Extract Functions

#### ExtractFile
```go
func ExtractFile(zipFile string, targetFile string) ([]byte, error)
```
Extract a single file from ZIP archive.

#### ExtractFileFromRaw
```go
func ExtractFileFromRaw(raw interface{}, targetFile string) ([]byte, error)
```
Extract a single file from ZIP raw data.

#### ExtractFiles
```go
func ExtractFiles(zipFile string, targetFiles []string) ([]*ExtractResult, error)
```
Extract multiple files concurrently from ZIP archive.

#### ExtractFilesFromRaw
```go
func ExtractFilesFromRaw(raw interface{}, targetFiles []string) ([]*ExtractResult, error)
```
Extract multiple files concurrently from ZIP raw data.

#### ExtractByPattern
```go
func ExtractByPattern(zipFile string, pattern string) ([]*ExtractResult, error)
```
Extract files matching a pattern (supports wildcards: `*`, `?`).

#### ExtractByPatternFromRaw
```go
func ExtractByPatternFromRaw(raw interface{}, pattern string) ([]*ExtractResult, error)
```
Extract files matching a pattern from ZIP raw data.

### Options

#### WithGrepLimit
```go
func WithGrepLimit(limit int) GrepOption
```
Limit the number of grep results.

#### WithContext
```go
func WithContext(context int) GrepOption
```
Include N lines before and after each match.

#### WithGrepCaseSensitive
```go
func WithGrepCaseSensitive() GrepOption
```
Enable case-sensitive search.

## Data Structures

### GrepResult
```go
type GrepResult struct {
    FileName      string   // File name in ZIP
    LineNumber    int      // Line number of match
    Line          string   // Matched line
    ContextBefore []string // Lines before match
    ContextAfter  []string // Lines after match
}
```

### ExtractResult
```go
type ExtractResult struct {
    FileName string // File name in ZIP
    Content  []byte // File content
    Error    error  // Extraction error if any
}
```

## Performance

- **Concurrent Extraction**: Automatically uses CPU cores (max 8) for parallel processing
- **Memory Efficient**: Supports streaming and raw data operations
- **Smart Limiting**: Built-in result limiting to prevent memory overflow

## Examples

### Search for errors in logs
```go
results, _ := ziputil.GrepRegexp("logs.zip", `\[ERROR\]`, 
    ziputil.WithContext(2), 
    ziputil.WithGrepLimit(10))
```

### Extract specific files
```go
files := []string{"config.json", "app.log"}
results, _ := ziputil.ExtractFiles("app.zip", files)
```

### Extract by pattern
```go
// Extract all .txt files
results, _ := ziputil.ExtractByPattern("archive.zip", "*.txt")

// Extract all files in src/ directory
results, _ := ziputil.ExtractByPattern("archive.zip", "src/*")
```

## Testing

Run tests:
```bash
go test -v ./common/utils/ziputil/...
```

The package includes comprehensive test coverage for:
- Grep functionality (regex and substring)
- File extraction (single, batch, pattern-based)
- Concurrent operations
- Error handling
- Pattern matching

## CI Integration

Tests are integrated into the CI pipeline via `.github/workflows/essential-tests.yml`:
```yaml
go test -v -timeout 20s ./common/utils/ziputil/...
```

## Related

- [Yak ZIP Library Documentation](../../../yaklang-ai-training-materials/library-usage/zip/zip_usage.md)
- [Yak ZIP Practice Examples](../../../yaklang-ai-training-materials/library-usage/zip/zip-practice.yak)
- [Yak ZIP Advanced Examples](../../../yaklang-ai-training-materials/library-usage/zip/zip-advance.yak)

