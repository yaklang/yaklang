// Command format_scan_matrix prints a ScanReport projection for one frozen
// fixture directory. It is a comparison driver, not part of the SCA runtime.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/yaklang/yaklang/common/sca"
	"github.com/yaklang/yaklang/common/sca/dxtypes"
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

func cpuNs() int64 {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return -1
	}
	return ru.Utime.Nano() + ru.Stime.Nano()
}

type countedFS struct {
	fs.FS
	opens int64
	reads int64
}

func (c *countedFS) Open(name string) (fs.File, error) {
	f, err := c.FS.Open(name)
	if err != nil {
		return nil, err
	}
	c.opens++
	return &countedFile{File: f, owner: c}, nil
}

func (c *countedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	return fs.ReadDir(c.FS, name)
}

func (c *countedFS) Stat(name string) (fs.FileInfo, error) {
	return fs.Stat(c.FS, name)
}

type countedFile struct {
	fs.File
	owner *countedFS
}

func (f *countedFile) Read(p []byte) (int, error) {
	n, err := f.File.Read(p)
	f.owner.reads += int64(n)
	return n, err
}

func (f *countedFile) ReadAt(p []byte, off int64) (int, error) {
	r, ok := f.File.(io.ReaderAt)
	if !ok {
		return 0, fs.ErrInvalid
	}
	n, err := r.ReadAt(p, off)
	f.owner.reads += int64(n)
	return n, err
}

func (f *countedFile) ReadDir(n int) ([]fs.DirEntry, error) {
	rd, ok := f.File.(fs.ReadDirFile)
	if !ok {
		return nil, &fs.PathError{Op: "readdir", Path: ".", Err: fs.ErrInvalid}
	}
	return rd.ReadDir(n)
}

func inputHash(dir string) (sum string, files int, bytes int64, err error) {
	var paths []string
	err = filepath.Walk(dir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		paths = append(paths, p)
		return nil
	})
	if err != nil {
		return "", 0, 0, err
	}
	sort.Strings(paths)
	h := sha256.New()
	for _, p := range paths {
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return "", 0, 0, err
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return "", 0, 0, err
		}
		fmt.Fprintf(h, "%s %d\n", filepath.ToSlash(rel), len(b))
		h.Write(b)
		files++
		bytes += int64(len(b))
	}
	return hex.EncodeToString(h.Sum(nil)), files, bytes, nil
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
		"side":              "new",
		"case":              name,
		"dir":               dir,
		"rss_source":        "getrusage(RUSAGE_SELF).ru_maxrss; darwin bytes, linux KiB converted to bytes; process peak including Go runtime",
		"duration_source":   "wall time of ScanReport only",
		"cpu_source":        "getrusage(RUSAGE_SELF) utime+stime delta around ScanReport; includes Go runtime; not isolated parser CPU",
		"opens_source":      "fs.FS Open count during ScanReport; snapshot adapter, not host syscalls",
		"read_bytes_source": "bytes returned by fs.File.Read/ReadAt during ScanReport; snapshot adapter, not host syscalls",
		"temp_files":        0,
		"temp_files_source": "ScanReport uses the snapshot fs.FS; this driver creates no temp files. Not a host syscall trace",
		"gomaxprocs":        runtime.GOMAXPROCS(0),
		"num_cpu":           runtime.NumCPU(),
		"workers_default":   5,
	}
	sum, nfiles, nbytes, err := inputHash(dir)
	if err != nil {
		fail(out, fmt.Errorf("input walk/read: %w", err), 1)
	}
	out["input_sha256"] = sum
	out["input_files"] = nfiles
	out["input_bytes"] = nbytes
	snap := &countedFS{FS: os.DirFS(dir)}
	cpu0 := cpuNs()
	start := time.Now()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	alloc0, mallocs0 := ms.TotalAlloc, ms.Mallocs
	r, err := sca.ScanReport(context.Background(), snap, sca.WithSnapshotID("matrix-"+name))
	elapsed := time.Since(start)
	cpu1 := cpuNs()
	runtime.ReadMemStats(&ms)
	out["ns"] = elapsed.Nanoseconds()
	if cpu0 >= 0 && cpu1 >= cpu0 {
		out["cpu_ns"] = cpu1 - cpu0
	} else {
		out["cpu_ns"] = int64(-1)
	}
	out["alloc_bytes"] = ms.TotalAlloc - alloc0
	out["mallocs"] = ms.Mallocs - mallocs0
	out["rss_bytes"] = rssBytes()
	out["file_opens"] = snap.opens
	out["read_bytes"] = snap.reads
	keys := []string{}
	complete := r != nil && r.Complete && err == nil
	if r != nil {
		out["components"] = len(r.Components)
		out["observations"] = len(r.Observations)
		out["requirements"] = len(r.Requirements)
		out["diagnostics"] = len(r.Diagnostics)
		for _, c := range r.Components {
			if strings.TrimSpace(c.Key.Name) == "" {
				continue
			}
			keys = append(keys, c.Key.Name+"\t"+c.Key.Version)
		}
	} else {
		out["components"] = 0
		out["observations"] = 0
		out["requirements"] = 0
		out["diagnostics"] = 0
	}
	uh, mh, unique := projection(keys)
	out["projection"] = len(unique)
	out["projection_sha256"] = uh
	out["multiplicity_sha256"] = mh
	out["identities"] = unique
	if os.Getenv("SCA_MATRIX_FULL_FIELDS") == "1" && r != nil {
		out["report"] = r
		out["sbom"] = dxtypes.CreateCycloneDXSBOMFromReport(r)
	}
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
