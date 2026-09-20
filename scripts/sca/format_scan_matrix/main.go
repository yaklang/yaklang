// Command format_scan_matrix prints a ScanReport projection for one frozen
// fixture directory. It is a comparison driver, not part of the SCA runtime.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/sca"
)

// rssBytes is the peak RSS of this process as reported by getrusage.
// Darwin/iOS ru_maxrss is bytes; Linux (and other Unix) ru_maxrss is KiB.
// The value includes the Go runtime, not an isolated parser heap. Duration
// recorded next to it is wall time of ScanReport only.
func rssBytes() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	n := int64(ru.Maxrss)
	if n < 0 {
		return -1
	}
	switch runtime.GOOS {
	case "darwin", "ios":
		return n
	default:
		return n * 1024
	}
}

func inputHash(dir string) (string, error) {
	var files []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, p := range files {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return "", err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s %d\n", filepath.ToSlash(rel), len(b))
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func projection(keys []string) (uniqueHash, multiHash string, unique []string) {
	counts := map[string]int{}
	for _, k := range keys {
		counts[k]++
	}
	unique = make([]string, 0, len(counts))
	for k := range counts {
		unique = append(unique, k)
	}
	sort.Strings(unique)
	uh := sha256.Sum256([]byte(strings.Join(unique, "\n")))
	multi := make([]string, 0, len(unique))
	for _, k := range unique {
		multi = append(multi, fmt.Sprintf("%s\t%d", k, counts[k]))
	}
	mh := sha256.Sum256([]byte(strings.Join(multi, "\n")))
	return hex.EncodeToString(uh[:]), hex.EncodeToString(mh[:]), unique
}

func fail(payload map[string]any, err error, code int) {
	payload["ok"] = false
	payload["complete"] = false
	payload["err"] = err.Error()
	_ = json.NewEncoder(os.Stdout).Encode(payload)
	os.Exit(code)
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: format_scan_matrix <case> <dir>")
		os.Exit(2)
	}
	name, dir := os.Args[1], os.Args[2]
	out := map[string]any{
		"side": "new", "case": name, "dir": dir,
		"rss_source":      "getrusage(RUSAGE_SELF).ru_maxrss; darwin bytes, linux KiB converted to bytes; process peak including Go runtime",
		"duration_source": "wall time of ScanReport only",
	}
	sum, err := inputHash(dir)
	if err != nil {
		fail(out, fmt.Errorf("input walk/read: %w", err), 1)
	}
	out["input_sha256"] = sum
	start := time.Now()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc0, mallocs0 := ms.TotalAlloc, ms.Mallocs
	r, err := sca.ScanReport(context.Background(), os.DirFS(dir), sca.WithSnapshotID("matrix-"+name))
	elapsed := time.Since(start)
	runtime.ReadMemStats(&ms)
	out["ns"] = elapsed.Nanoseconds()
	out["alloc_bytes"] = ms.TotalAlloc - alloc0
	out["mallocs"] = ms.Mallocs - mallocs0
	out["rss_bytes"] = rssBytes()
	keys := []string{}
	complete := r != nil && r.Complete && err == nil
	if r != nil {
		out["components"] = len(r.Components)
		for _, c := range r.Components {
			if strings.TrimSpace(c.Key.Name) == "" {
				continue
			}
			keys = append(keys, c.Key.Name+"\t"+c.Key.Version)
		}
	} else {
		out["components"] = 0
	}
	uh, mh, unique := projection(keys)
	out["projection"] = len(unique)
	out["projection_sha256"] = uh
	out["multiplicity_sha256"] = mh
	out["identities"] = unique
	out["complete"] = complete
	out["ok"] = complete
	if err != nil {
		out["err"] = err.Error()
	} else {
		out["err"] = ""
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
	if !complete {
		os.Exit(1)
	}
}
