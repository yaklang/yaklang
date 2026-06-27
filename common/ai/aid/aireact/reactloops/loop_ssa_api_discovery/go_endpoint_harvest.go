package loop_ssa_api_discovery

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

const (
	sourceStaticGoHTTP          = "static_go_http"
	defaultGoHTTPHarvestWorkers = 8
)

var (
	reGoPackage   = regexp.MustCompile(`(?m)^\s*package\s+([\w.]+)\s*$`)
	reGoVerbPath  = regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS)\s*\(\s*"([^"]+)"\s*,`)
	reGoHandleFun = regexp.MustCompile(`http\.HandleFunc\s*\(\s*"([^"]+)"\s*,`)
	reGoHandle    = regexp.MustCompile(`http\.Handle\s*\(\s*"([^"]+)"\s*,`)
)

// HarvestGoHTTPMappings 扫描 *.go（并发分文件）。
func HarvestGoHTTPMappings(codeRoot string) ([]HarvestedEndpoint, error) {
	return HarvestGoHTTPMappingsConcurrent(codeRoot, defaultGoHTTPHarvestWorkers)
}

// HarvestGoHTTPMappingsConcurrent workers<=1 时单线程遍历。
func HarvestGoHTTPMappingsConcurrent(codeRoot string, workers int) ([]HarvestedEndpoint, error) {
	if strings.TrimSpace(codeRoot) == "" {
		return nil, utils.Error("empty code root")
	}
	if workers <= 1 {
		return harvestGoSequential(codeRoot)
	}
	var goFiles []string
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		lp := strings.ToLower(path)
		if strings.HasSuffix(lp, ".go") && !strings.HasSuffix(lp, "_test.go") {
			goFiles = append(goFiles, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(goFiles) == 0 {
		return nil, nil
	}
	jobs := make(chan string, len(goFiles))
	var mu sync.Mutex
	var out []HarvestedEndpoint
	var wg sync.WaitGroup
	n := workers
	if n > len(goFiles) {
		n = len(goFiles)
	}
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for abs := range jobs {
				data, rerr := os.ReadFile(abs)
				if rerr != nil {
					continue
				}
				rel, _ := filepath.Rel(codeRoot, abs)
				rel = filepath.ToSlash(rel)
				eps := harvestGoFromSource(data, rel)
				if len(eps) == 0 {
					continue
				}
				mu.Lock()
				out = append(out, eps...)
				mu.Unlock()
			}
		}()
	}
	for _, p := range goFiles {
		jobs <- p
	}
	close(jobs)
	wg.Wait()
	return dedupeHarvested(out), nil
}

func harvestGoSequential(codeRoot string) ([]HarvestedEndpoint, error) {
	var out []HarvestedEndpoint
	err := filepath.Walk(codeRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if info.IsDir() {
			if skipDirForHarvest(info.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		lp := strings.ToLower(path)
		if !strings.HasSuffix(lp, ".go") || strings.HasSuffix(lp, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(codeRoot, path)
		rel = filepath.ToSlash(rel)
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		out = append(out, harvestGoFromSource(data, rel)...)
		return nil
	})
	return dedupeHarvested(out), err
}

func harvestGoFromSource(data []byte, fileRel string) []HarvestedEndpoint {
	var res []HarvestedEndpoint
	pkg := ""
	if m := reGoPackage.FindSubmatch(data); len(m) > 1 {
		pkg = string(m[1])
	}
	s := string(data)
	for _, m := range reGoVerbPath.FindAllStringSubmatch(s, -1) {
		if len(m) < 3 {
			continue
		}
		p := strings.TrimSpace(m[2])
		if p == "" {
			continue
		}
		res = append(res, HarvestedEndpoint{
			Method:        strings.ToUpper(m[1]),
			PathPattern:   p,
			HandlerClass:  pkg,
			HandlerMethod: "",
			Provenance:    sourceStaticGoHTTP,
			FileRelPath:   fileRel,
		})
	}
	for _, m := range reGoHandleFun.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		p := strings.TrimSpace(m[1])
		if p == "" {
			continue
		}
		res = append(res, HarvestedEndpoint{
			Method:        "GET",
			PathPattern:   p,
			HandlerClass:  pkg,
			HandlerMethod: "",
			Provenance:    sourceStaticGoHTTP,
			FileRelPath:   fileRel,
		})
	}
	for _, m := range reGoHandle.FindAllStringSubmatch(s, -1) {
		if len(m) < 2 {
			continue
		}
		p := strings.TrimSpace(m[1])
		if p == "" {
			continue
		}
		res = append(res, HarvestedEndpoint{
			Method:        "GET",
			PathPattern:   p,
			HandlerClass:  pkg,
			HandlerMethod: "",
			Provenance:    sourceStaticGoHTTP,
			FileRelPath:   fileRel,
		})
	}
	return res
}
