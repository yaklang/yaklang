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

func rssKB() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	return ru.Maxrss
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: format_scan_matrix <case> <dir>")
		os.Exit(2)
	}
	name, dir := os.Args[1], os.Args[2]
	h := sha256.New()
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		b, _ := os.ReadFile(p)
		fmt.Fprintf(h, "%s %d\n", filepath.Base(p), len(b))
		h.Write(b)
		return nil
	})
	start := time.Now()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc0, mallocs0 := ms.TotalAlloc, ms.Mallocs
	r, err := sca.ScanReport(context.Background(), os.DirFS(dir), sca.WithSnapshotID("matrix-"+name))
	elapsed := time.Since(start)
	runtime.ReadMemStats(&ms)
	keys := []string{}
	if r != nil {
		for _, c := range r.Components {
			if strings.TrimSpace(c.Key.Name) == "" {
				continue
			}
			keys = append(keys, c.Key.Name+"\t"+c.Key.Version)
		}
		sort.Strings(keys)
	}
	ph := sha256.Sum256([]byte(strings.Join(keys, "\n")))
	complete := r != nil && r.Complete && err == nil
	out := map[string]any{
		"side": "new", "case": name, "dir": dir,
		"input_sha256":      hex.EncodeToString(h.Sum(nil)),
		"ns":                elapsed.Nanoseconds(),
		"alloc_bytes":       ms.TotalAlloc - alloc0,
		"mallocs":           ms.Mallocs - mallocs0,
		"rss_kb":            rssKB(),
		"complete":          complete,
		"components":        0,
		"projection":        len(keys),
		"projection_sha256": hex.EncodeToString(ph[:]),
		"err":               fmt.Sprint(err),
		"ok":                complete,
	}
	if r != nil {
		out["components"] = len(r.Components)
	}
	_ = json.NewEncoder(os.Stdout).Encode(out)
}
