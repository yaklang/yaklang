package main

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const dataMagic = "YAKHNSW1"

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Data []struct {
		Embedding []float32 `json:"embedding"`
		Index     int       `json:"index"`
	} `json:"data"`
	Error any `json:"error"`
}

func main() {
	root := flag.String("root", ".", "repository root used as the real-text corpus")
	output := flag.String("output", "/tmp/yaklang-hnsw-real-1024.f32", "output vector file")
	endpoint := flag.String("endpoint", "http://127.0.0.1:11435/v1/embeddings", "OpenAI-compatible embeddings endpoint")
	baseCount := flag.Int("base", 5000, "number of index vectors")
	queryCount := flag.Int("queries", 200, "number of disjoint query vectors")
	batchSize := flag.Int("batch", 16, "embedding request batch size")
	flag.Parse()

	texts, err := collectChunks(*root, *baseCount+*queryCount)
	if err != nil {
		fatal(err)
	}
	if len(texts) < *baseCount+*queryCount {
		fatal(fmt.Errorf("only found %d chunks, need %d", len(texts), *baseCount+*queryCount))
	}

	started := time.Now()
	vectors := make([][]float32, 0, len(texts))
	client := &http.Client{Timeout: 2 * time.Minute}
	for start := 0; start < len(texts); start += *batchSize {
		end := min(start+*batchSize, len(texts))
		batch, err := embed(client, *endpoint, texts[start:end])
		if err != nil {
			fatal(fmt.Errorf("embed chunks %d..%d: %w", start, end, err))
		}
		vectors = append(vectors, batch...)
		if end%256 == 0 || end == len(texts) {
			fmt.Fprintf(os.Stderr, "embedded %d/%d chunks in %s\n", end, len(texts), time.Since(started).Round(time.Second))
		}
	}

	if err := writeVectors(*output, vectors, *baseCount, *queryCount); err != nil {
		fatal(err)
	}
	fmt.Printf("wrote %d base + %d query vectors (%d dimensions) to %s in %s\n",
		*baseCount, *queryCount, len(vectors[0]), *output, time.Since(started).Round(time.Second))
}

func collectChunks(root string, limit int) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			name := entry.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" || name == "static" || name == "dist" {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext == ".go" || ext == ".md" {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	chunks := make([]string, 0, limit)
	for _, path := range paths {
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		scanner := bufio.NewScanner(file)
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		var chunk strings.Builder
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "//go:") {
				continue
			}
			if chunk.Len() > 0 {
				chunk.WriteByte('\n')
			}
			chunk.WriteString(line)
			if chunk.Len() >= 600 {
				chunks = append(chunks, filepath.ToSlash(path)+"\n"+chunk.String())
				chunk.Reset()
				if len(chunks) == limit {
					file.Close()
					return chunks, nil
				}
			}
		}
		if err := scanner.Err(); err != nil {
			file.Close()
			return nil, err
		}
		if chunk.Len() >= 160 {
			chunks = append(chunks, filepath.ToSlash(path)+"\n"+chunk.String())
		}
		file.Close()
		if len(chunks) >= limit {
			return chunks[:limit], nil
		}
	}
	return chunks, nil
}

func embed(client *http.Client, endpoint string, texts []string) ([][]float32, error) {
	body, err := json.Marshal(embeddingRequest{Model: "embedding", Input: texts})
	if err != nil {
		return nil, err
	}
	request, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("embedding server returned %s: %s", response.Status, message)
	}
	var decoded embeddingResponse
	if err := json.NewDecoder(response.Body).Decode(&decoded); err != nil {
		return nil, err
	}
	if len(decoded.Data) != len(texts) {
		return nil, fmt.Errorf("embedding server returned %d vectors for %d texts (error=%v)", len(decoded.Data), len(texts), decoded.Error)
	}
	vectors := make([][]float32, len(texts))
	for _, item := range decoded.Data {
		if item.Index < 0 || item.Index >= len(vectors) {
			return nil, fmt.Errorf("invalid response index %d", item.Index)
		}
		if len(item.Embedding) != 1024 {
			return nil, fmt.Errorf("embedding dimension is %d, want 1024", len(item.Embedding))
		}
		vectors[item.Index] = item.Embedding
	}
	return vectors, nil
}

func writeVectors(path string, vectors [][]float32, baseCount, queryCount int) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	writer := bufio.NewWriterSize(file, 1024*1024)
	if _, err := writer.WriteString(dataMagic); err != nil {
		return err
	}
	header := []uint32{1024, uint32(baseCount), uint32(queryCount)}
	if err := binary.Write(writer, binary.LittleEndian, header); err != nil {
		return err
	}
	for _, vector := range vectors {
		if err := binary.Write(writer, binary.LittleEndian, vector); err != nil {
			return err
		}
	}
	return writer.Flush()
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "hnsw-realdata:", err)
	os.Exit(1)
}
