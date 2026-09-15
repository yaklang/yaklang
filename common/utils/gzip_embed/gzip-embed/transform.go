package main

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yaklang/yaklang/common/log"
)

// embedDirective matches //go:embed path
var embedDirective = regexp.MustCompile(`^//go:embed\s+(.+)$`)

// xorKeyConst matches `const xxxXorKey = "value"` or `const xxxKey = "value"`
var xorKeyConst = regexp.MustCompile(`const\s+\w*(?:XorKey|xorKey|XORKey|Key)\s*=\s*"([^"]+)"`)

// goGenerateGzipEmbed matches //go:generate gzip-embed ... and captures the full command
var goGenerateGzipEmbed = regexp.MustCompile(`//go:generate\s+gzip-embed\s+(.*)`)

// foundEmbed describes one //go:embed directory found in a file.
type foundEmbed struct {
	lineIdx   int    // line index of the //go:embed line
	embedPath string // cleaned path: "static" not "static/***"
	rawPath   string // original path: "static/***"
	gzName    string // "static.tar.gz"
	rootPath  bool   // use --root-path when packing
	// For multi-source (from go:generate):
	multiSources []string // source directories relative to file dir
	baseDir      string   // base directory for path computation
}

// parseGoGenerateGzipEmbed parses //go:generate gzip-embed ... parameters
// and returns sources, gzName, baseDir, xorKey.
func parseGoGenerateGzipEmbed(raw string) (sources []string, gzName, baseDir, xorKey string) {
	parts := strings.Fields(raw)
	for i := 0; i < len(parts); i++ {
		switch parts[i] {
		case "--source", "-s":
			if i+1 < len(parts) {
				sources = append(sources, parts[i+1])
				i++
			}
		case "--gz":
			if i+1 < len(parts) {
				gzName = parts[i+1]
				i++
			}
		case "--base", "-b":
			if i+1 < len(parts) {
				baseDir = parts[i+1]
				i++
			}
		case "--xor-key":
			if i+1 < len(parts) {
				xorKey = parts[i+1]
				i++
			}
		}
	}
	return
}

// detectRootPath determines whether to use --root-path when packing.
// It checks:
// 1. The gzip_embed.go counterpart file's NewGzipResourceMonitor pathPrefix (non-empty = root-path)
// 2. The go:generate gzip-embed directive for --root-path flag
// 3. Default: false
func detectRootPath(goFile string, src string, gzName string) bool {
	// Check go:generate in this file's source
	for _, line := range strings.Split(src, "\n") {
		gm := goGenerateGzipEmbed.FindStringSubmatch(strings.TrimSpace(line))
		if gm != nil {
			if strings.Contains(gm[1], "--root-path") || strings.Contains(gm[1], "-r") {
				return true
			}
		}
	}

	// Check go:generate in other files in the same directory
	fileDir := filepath.Dir(goFile)
	entries, err := os.ReadDir(fileDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
				continue
			}
			otherPath := filepath.Join(fileDir, entry.Name())
			if otherPath == goFile {
				continue
			}
			otherSrc, err := os.ReadFile(otherPath)
			if err != nil {
				continue
			}
			otherStr := string(otherSrc)
			for _, line := range strings.Split(otherStr, "\n") {
				gm := goGenerateGzipEmbed.FindStringSubmatch(strings.TrimSpace(line))
				if gm != nil {
					_, gGzName, _, _ := parseGoGenerateGzipEmbed(gm[1])
					if gGzName == gzName && (strings.Contains(gm[1], "--root-path") || strings.Contains(gm[1], " -r ")) {
						return true
					}
				}
			}
			// Check for NewGzipResourceMonitor with non-empty pathPrefix in counterpart file
			if strings.Contains(otherStr, "NewGzipResourceMonitor") {
				// Extract the third argument (pathPrefix)
				re := regexp.MustCompile(`NewGzipResourceMonitor\([^)]*"([^"]+)"[^)]*"([^"]*)"`)
				matches := re.FindAllStringSubmatch(otherStr, -1)
				for _, m := range matches {
					if len(m) >= 3 && m[1] == gzName && m[2] != "" {
						return true
					}
				}
			}
		}
	}

	return false
}

// runTransform implements the `gzip-embed transform` subcommand.
// It scans all .go files for //go:embed directives pointing to directories,
// packs each directory into tar.gz, and rewrites the Go source to read
// from the tar.gz via gzip_embed.PreprocessingEmbed instead of plain embed.FS.
func runTransform(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: gzip-embed transform [--dry-run] [--verbose] <path>...")
	}

	dryRun := false
	verbose := false
	var paths []string
	for _, a := range args {
		switch a {
		case "--dry-run":
			dryRun = true
		case "--verbose", "-v":
			verbose = true
		default:
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		return fmt.Errorf("no path specified")
	}

	// Collect all .go files (non-test)
	var goFiles []string
	for _, p := range paths {
		p = strings.TrimSuffix(p, "/...")
		err := filepath.Walk(p, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				goFiles = append(goFiles, path)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk %s: %w", p, err)
		}
	}

	if verbose {
		log.Infof("scanning %d .go files", len(goFiles))
	}

	transformed := 0
	deletedFiles := make(map[string]bool) // track files deleted by counterpart logic
	for _, goFile := range goFiles {
		if deletedFiles[goFile] {
			continue // file was deleted as a counterpart of another file
		}
		n, err := transformFile(goFile, dryRun, verbose)
		if err != nil {
			if os.IsNotExist(err) {
				continue // file was deleted by a previous transform
			}
			log.Errorf("transform %s: %v", goFile, err)
			continue
		}
		if n > 0 {
			transformed += n
			if verbose {
				log.Infof("transformed %d embed(s) in %s", n, goFile)
			}
		}
	}

	if transformed == 0 {
		if verbose {
			log.Infof("no directory embeds found to transform")
		}
	} else {
		log.Infof("transform complete: %d embed(s) transformed", transformed)
	}
	return nil
}

// transformFile processes a single .go file.
func transformFile(goFile string, dryRun, verbose bool) (int, error) {
	src, err := os.ReadFile(goFile)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // file was deleted by a previous counterpart transform
		}
		return 0, err
	}
	srcStr := string(src)
	lines := strings.Split(srcStr, "\n")

	// Phase 1: determine if this file is part of the gzip_embed ecosystem.
	// We auto-detect based on existing indicators — no markers needed.
	hasBuildTag := false      // //go:build gzip_embed or !gzip_embed
	hasGoGenerate := false    // //go:generate gzip-embed (in this file or sibling files)
	hasGzipEmbedCall := false // gzip_embed.NewPreprocessingEmbed
	hasResourceMonitor := false // resources_monitor.NewStandardResourceMonitor
	hasEmbedFSDir := false    // //go:embed pointing to a directory + filesys.NewEmbedFS

	// Check this file for indicators
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//go:build") && strings.Contains(trimmed, "gzip_embed") {
			hasBuildTag = true
		}
		if strings.HasPrefix(trimmed, "//go:generate") && strings.Contains(trimmed, "gzip-embed") {
			hasGoGenerate = true
		}
		if strings.Contains(line, "gzip_embed.NewPreprocessingEmbed") {
			hasGzipEmbedCall = true
		}
		if strings.Contains(line, "resources_monitor.NewStandardResourceMonitor") {
			hasResourceMonitor = true
		}
	}

	// Check sibling files in the same directory for //go:generate gzip-embed
	if !hasGoGenerate {
		fileDir := filepath.Dir(goFile)
		entries, err := os.ReadDir(fileDir)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				otherPath := filepath.Join(fileDir, entry.Name())
				if otherPath == goFile {
					continue
				}
				otherSrc, err := os.ReadFile(otherPath)
				if err != nil {
					continue
				}
				if strings.Contains(string(otherSrc), "//go:generate gzip-embed") {
					hasGoGenerate = true
					break
				}
			}
		}
	}

	// Check if this file has //go:embed pointing to a directory + filesys.NewEmbedFS or NewEmbedSubFS
	// This indicates a migrated file that needs transform in CI
	if strings.Contains(srcStr, "filesys.NewEmbedFS") || strings.Contains(srcStr, "filesys.NewEmbedSubFS") {
		for _, line := range lines {
			m := embedDirective.FindStringSubmatch(strings.TrimSpace(line))
			if m == nil {
				continue
			}
			rawPath := strings.TrimSpace(m[1])
			if strings.HasSuffix(rawPath, ".tar.gz") || strings.ContainsAny(rawPath, "$%") {
				continue
			}
			// Handle multi-path embed: "dir1 dir2 dir3" → check each path
			paths := strings.Fields(rawPath)
			fileDir := filepath.Dir(goFile)
			for _, p := range paths {
				cleanPath := p
				cleanPath = strings.TrimSuffix(cleanPath, "/***")
				cleanPath = strings.TrimSuffix(cleanPath, "/**")
				cleanPath = strings.TrimSuffix(cleanPath, "/*")
				cleanPath = strings.TrimSuffix(cleanPath, "/")
				if cleanPath == "" || cleanPath == "." {
					continue
				}
				base := filepath.Base(cleanPath)
				if strings.Contains(base, ".") {
					continue
				}
				fullPath := filepath.Join(fileDir, cleanPath)
				if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
					hasEmbedFSDir = true
					break
				}
			}
			if hasEmbedFSDir {
				break
			}
		}
	}

	// Only process files that are part of the gzip_embed ecosystem
	if !hasBuildTag && !hasGoGenerate && !hasGzipEmbedCall && !hasResourceMonitor && !hasEmbedFSDir {
		return 0, nil
	}

	// Auto-detect XOR key from the file's const declarations
	xorKey := detectXORKey(srcStr)
	if verbose && xorKey != "" {
		log.Infof("auto-detected XOR key: %s", xorKey)
	}

	// Phase 1b: find //go:embed directives pointing to directories
	var embeds []foundEmbed
	for i, line := range lines {
		m := embedDirective.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		rawPath := strings.TrimSpace(m[1])

		// Skip template placeholders
		if strings.ContainsAny(rawPath, "$%") {
			continue
		}

		// Skip if already tar.gz or placeholder
		if strings.HasSuffix(rawPath, ".tar.gz") || strings.ContainsAny(rawPath, "$%") {
			continue
		}

		// Check for multi-path embed (e.g. "dir1 dir2 dir3")
		// This happens when one //go:embed lists multiple directories
		pathParts := strings.Fields(rawPath)
		if len(pathParts) > 1 {
			// Multi-path: look for go:generate in sibling files to get gz name and sources
			fileDir := filepath.Dir(goFile)
			entries, _ := os.ReadDir(fileDir)
			for _, entry := range entries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				otherPath := filepath.Join(fileDir, entry.Name())
				otherSrc, err := os.ReadFile(otherPath)
				if err != nil {
					continue
				}
				for _, otherLine := range strings.Split(string(otherSrc), "\n") {
					gm := goGenerateGzipEmbed.FindStringSubmatch(strings.TrimSpace(otherLine))
					if gm == nil {
						continue
					}
					gSources, gGzName, gBase, gXorKey := parseGoGenerateGzipEmbed(gm[1])
					if len(gSources) > 0 && gGzName != "" {
						// Verify all source dirs exist
						allExist := true
						for _, s := range gSources {
							if _, err := os.Stat(filepath.Join(fileDir, s)); err != nil {
								allExist = false
								break
							}
						}
						if allExist {
							embeds = append(embeds, foundEmbed{
								lineIdx:      i,
								embedPath:    strings.TrimSuffix(gGzName, ".tar.gz"),
								rawPath:      rawPath,
								gzName:       gGzName,
								rootPath:     false,
								multiSources: gSources,
								baseDir:      gBase,
							})
							// Override xorKey if go:generate has one and we haven't detected one
							if xorKey == "" && gXorKey != "" {
								xorKey = gXorKey
							}
						}
						break
					}
				}
				if len(embeds) > 0 && embeds[len(embeds)-1].gzName != "" && len(embeds[len(embeds)-1].multiSources) > 0 {
					break
				}
			}
			continue
		}

		// Single path cases:
		// A) embed points to a directory (e.g. "static", "buildinforge/**") → pack and rewrite
		// B) embed points to a .tar.gz file (e.g. "static.tar.gz") → just re-generate the tar.gz

		if strings.HasSuffix(rawPath, ".tar.gz") {
			// Case B: file already embeds tar.gz, just need to re-generate it
			cleanPath := strings.TrimSuffix(rawPath, ".tar.gz")
			fileDir := filepath.Dir(goFile)
			fullPath := filepath.Join(fileDir, cleanPath)

			// Check if the source directory exists (single-source case)
			if info, err := os.Stat(fullPath); err == nil && info.IsDir() {
				rp := detectRootPath(goFile, srcStr, rawPath)
				embeds = append(embeds, foundEmbed{
					lineIdx:   i,
					embedPath: cleanPath,
					rawPath:   rawPath,
					gzName:    rawPath,
					rootPath:  rp,
				})
				continue
			}

			// Directory doesn't exist - check for //go:generate gzip-embed with multi-source
			// in any .go file in the same directory
			dirEntries, _ := os.ReadDir(fileDir)
			for _, entry := range dirEntries {
				if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
					continue
				}
				if strings.HasSuffix(entry.Name(), "_test.go") {
					continue
				}
				otherPath := filepath.Join(fileDir, entry.Name())
				otherSrc, err := os.ReadFile(otherPath)
				if err != nil {
					continue
				}
				otherLines := strings.Split(string(otherSrc), "\n")
				for _, otherLine := range otherLines {
					gm := goGenerateGzipEmbed.FindStringSubmatch(strings.TrimSpace(otherLine))
					if gm == nil {
						continue
					}
					gSources, gGzName, gBase, _ := parseGoGenerateGzipEmbed(gm[1])
					if gGzName == rawPath && len(gSources) > 0 {
						// Found matching go:generate directive with multi-source
						embeds = append(embeds, foundEmbed{
							lineIdx:      i,
							embedPath:    cleanPath,
							rawPath:      rawPath,
							gzName:       rawPath,
							multiSources: gSources,
							baseDir:      gBase,
						})
						break
					}
				}
				if len(embeds) > 0 && embeds[len(embeds)-1].gzName == rawPath {
					break
				}
			}
			continue
		}

		// Case A: embed points to a directory
		cleanPath := rawPath
		cleanPath = strings.TrimSuffix(cleanPath, "/***")
		cleanPath = strings.TrimSuffix(cleanPath, "/**")
		cleanPath = strings.TrimSuffix(cleanPath, "/*")
		cleanPath = strings.TrimSuffix(cleanPath, "/")
		if cleanPath == "" || cleanPath == "." {
			continue
		}

		// Check if the path looks like a file (has an extension)
		base := filepath.Base(cleanPath)
		if strings.Contains(base, ".") {
			continue
		}

		// Check if the directory actually exists on disk
		fileDir := filepath.Dir(goFile)
		fullPath := filepath.Join(fileDir, cleanPath)
		if info, err := os.Stat(fullPath); err != nil || !info.IsDir() {
			continue
		}

		gzName := cleanPath + ".tar.gz"
		rp := detectRootPath(goFile, srcStr, gzName)
		// For A-class files (files that had a gzip_embed.go counterpart or
		// use NewStandardResourceMonitor), default rootPath=true because
		// these files read WITH the directory prefix (e.g. "buildinforge/file.yak"
		// or "static/config.yaml"), so the tar.gz must preserve the prefix.
		// Also for files using NewEmbedFS (not NewEmbedSubFS), the caller reads
		// WITH the directory prefix, so rootPath must be true.
		// NewEmbedSubFS strips the prefix, so rootPath=false is correct for those.
		if !rp && (hasResourceMonitor || hasBuildTag || hasGzipEmbedCall) {
			rp = true
		}
		// Check if this file uses NewEmbedFS (without Sub) - those need rootPath=true
		if !rp && strings.Contains(srcStr, "filesys.NewEmbedFS(") && !strings.Contains(srcStr, "filesys.NewEmbedSubFS(") {
			rp = true
		}
		embeds = append(embeds, foundEmbed{
			lineIdx:   i,
			embedPath: cleanPath,
			rawPath:   rawPath,
			gzName:    gzName,
			rootPath:  rp,
		})
	}

	if len(embeds) == 0 {
		return 0, nil
	}

	// Detect build tag line to remove (//go:build !gzip_embed)
	buildTagIdx := -1
	for j, ln := range lines {
		trimmed := strings.TrimSpace(ln)
		if strings.HasPrefix(trimmed, "//go:build") && strings.Contains(trimmed, "!gzip_embed") {
			buildTagIdx = j
			break
		}
	}

	// Phase 2: pack directories to tar.gz
	fileDir := filepath.Dir(goFile)
	count := 0
	for _, em := range embeds {
		gzPath := filepath.Join(fileDir, em.gzName)

		var sources []string
		var baseDir string
		if len(em.multiSources) > 0 {
			// Multi-source: use go:generate parameters
			for _, s := range em.multiSources {
				sources = append(sources, filepath.Join(fileDir, s))
			}
			if em.baseDir != "" {
				baseDir = filepath.Join(fileDir, em.baseDir)
			}
		} else {
			// Single source
			sources = []string{filepath.Join(fileDir, em.embedPath)}
		}

		if verbose {
			log.Infof("packing %v → %s (xor=%v)", sources, gzPath, xorKey != "")
		}

		if !dryRun {
			if err := targz(sources, baseDir, gzPath, em.rootPath, false); err != nil {
				return count, fmt.Errorf("pack %v: %w", sources, err)
			}
			if xorKey != "" {
				if err := xorEncodeFile(gzPath, []byte(xorKey)); err != nil {
					return count, fmt.Errorf("xor encode %s: %w", gzPath, err)
				}
			}
		}
		count++
	}

	if dryRun {
		for _, em := range embeds {
			fmt.Fprintf(os.Stderr, "[dry-run] %s: %s → %s\n", goFile, em.embedPath, em.gzName)
		}
		return count, nil
	}

	// Phase 3a: if this was an A-class file with //go:build !gzip_embed,
	// delete the corresponding gzip_embed.go counterpart to avoid duplicate definitions.
	if buildTagIdx >= 0 {
		gzipEmbedFile := findGzipEmbedCounterpart(goFile)
		if gzipEmbedFile != "" {
			if verbose {
				log.Infof("deleting counterpart: %s", gzipEmbedFile)
			}
			if !dryRun {
				os.Remove(gzipEmbedFile)
			}
		}
	}

	// Phase 3b: rewrite source code
	// Remove build tag line (//go:build !gzip_embed)
	if buildTagIdx >= 0 {
		lines[buildTagIdx] = ""
	}

	// Don't change //go:embed paths yet - do it after AST rewrite
	// to avoid comment association issues with the AST printer.

	newSrc := strings.Join(lines, "\n")

	// AST rewrite for initialization code
	newSrc, err = rewriteInitCode(goFile, newSrc, embeds, xorKey)
	if err != nil {
		return count, fmt.Errorf("AST rewrite: %w", err)
	}

	// Now change //go:embed paths in the AST-printed source
	// (only for directory embeds, not already-tar.gz)
	for _, em := range embeds {
		if !strings.HasSuffix(em.rawPath, ".tar.gz") {
			newSrc = strings.Replace(newSrc, "//go:embed "+em.rawPath, "//go:embed "+em.gzName, 1)
		}
	}

	// Fix misplaced //go:embed directives that the AST printer may have
	// moved inside the import block. This happens when adding/removing
	// imports causes comment reassociation.
	newSrc = fixMisplacedEmbedDirectives(newSrc)

	// Format and write back
	formatted, err := format.Source([]byte(newSrc))
	if err != nil {
		log.Warnf("format failed for %s: %v (writing unformatted)", goFile, err)
		formatted = []byte(newSrc)
	}

	// Remove any blank lines between //go:embed directives and the following
	// var/declaration line (Go requires //go:embed to be directly above var,
	// though some compilers tolerate blank lines, it's safer to remove them).
	formattedStr := string(formatted)
	formattedStr = removeBlankLinesBetweenEmbedAndVar(formattedStr)
	formatted = []byte(formattedStr)

	if err := os.WriteFile(goFile, formatted, 0644); err != nil {
		return count, fmt.Errorf("write %s: %w", goFile, err)
	}

	return count, nil
}

// detectXORKey scans the source for a const declaration matching XOR key pattern.
func detectXORKey(src string) string {
	matches := xorKeyConst.FindAllStringSubmatch(src, -1)
	if len(matches) == 0 {
		return ""
	}
	// Return the first match
	return matches[0][1]
}

// targetInfo holds info about an embed variable that needs its init code rewritten.
type targetInfo struct {
	gzName  string
	xorKey  string
	pkgDir  string
	rootPath bool
}

// fixMisplacedEmbedDirectives fixes //go:embed directives that were
// accidentally moved inside the import block by the AST printer.
// It removes //go:embed directives from inside the import block and
// places them directly above the correct var declaration.
func fixMisplacedEmbedDirectives(src string) string {
	lines := strings.Split(src, "\n")
	
	// Find //go:embed directives inside the import block and remove them
	// Also find the var declaration that should have the embed
	var misplacedEmbed string
	var importCloseIdx int = -1
	inImportBlock := false
	
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "import (" {
			inImportBlock = true
			continue
		}
		if inImportBlock && trimmed == ")" {
			inImportBlock = false
			importCloseIdx = i
			continue
		}
		if inImportBlock && strings.HasPrefix(trimmed, "//go:embed ") {
			misplacedEmbed = trimmed
			lines[i] = "" // remove from import block
		}
	}
	
	if misplacedEmbed == "" {
		return src // nothing to fix
	}
	
	// Find the first var declaration after the import block
	// that has type embed.FS
	for i := importCloseIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "var ") && strings.Contains(trimmed, "embed.FS") {
			// Insert the //go:embed directive directly above this var
			lines = append(lines[:i], append([]string{misplacedEmbed}, lines[i:]...)...)
			break
		}
	}
	
	// Clean up empty lines left by removed embed directives
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" && len(result) > 0 && result[len(result)-1] == "" {
			continue // skip consecutive empty lines
		}
		result = append(result, line)
	}
	
	return strings.Join(result, "\n")
}

// removeBlankLinesBetweenEmbedAndVar removes blank lines between
// //go:embed directives and the following var/type declaration.
func removeBlankLinesBetweenEmbedAndVar(src string) string {
	lines := strings.Split(src, "\n")
	var result []string
	i := 0
	for i < len(lines) {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "//go:embed ") {
			result = append(result, lines[i])
			i++
			// Skip following blank lines
			for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
				i++
			}
			continue
		}
		result = append(result, lines[i])
		i++
	}
	return strings.Join(result, "\n")
}

// rewriteInitCode uses AST to find and replace initialization calls.
func rewriteInitCode(filename, src string, embeds []foundEmbed, xorKey string) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return src, fmt.Errorf("parse: %w", err)
	}

	// Map: embed var name → target info
	targets := make(map[string]targetInfo)

	for _, decl := range f.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.VAR {
			continue
		}

		for _, spec := range genDecl.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}

			// Look for //go:embed in comments
			var embedComment string
			if vs.Doc != nil {
				for _, c := range vs.Doc.List {
					if strings.Contains(c.Text, "//go:embed") {
						embedComment = c.Text
						break
					}
				}
			}
			if embedComment == "" && genDecl.Doc != nil {
				for _, c := range genDecl.Doc.List {
					if strings.Contains(c.Text, "//go:embed") {
						embedComment = c.Text
						break
					}
				}
			}
			if embedComment == "" {
				continue
			}

			em := embedDirective.FindStringSubmatch(embedComment)
			if em == nil {
				continue
			}
			embedPath := strings.TrimSpace(em[1])

			// Match against our embeds
			for _, e := range embeds {
				// Match against either the gzName (if path already changed)
				// or the rawPath (if path not yet changed)
				if embedPath == e.gzName || embedPath == e.rawPath {
					pkgDir := filepath.Base(e.embedPath)
					for _, name := range vs.Names {
						targets[name.Name] = targetInfo{
							gzName:   e.gzName,
							xorKey:   xorKey,
							pkgDir:   pkgDir,
							rootPath: e.rootPath,
						}
					}
				}
			}
		}
	}

	if len(targets) == 0 {
		return src, nil
	}

	// Walk AST and replace initialization calls
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}

		fnName := getCallExprName(call)
		if fnName == "" || len(call.Args) == 0 {
			return true
		}

		varName := getExprVarName(call.Args[0])
		if varName == "" {
			return true
		}

		target, ok := targets[varName]
		if !ok {
			return true
		}

		switch fnName {
		case "resources_monitor.NewStandardResourceMonitor":
			// → NewGzipResourceMonitor(&var, "gzName", "prefix")
			// prefix is the directory base name when rootPath=true, empty when false
			call.Args[0] = &ast.UnaryExpr{Op: token.AND, X: call.Args[0]}
			if len(call.Args) >= 2 {
				call.Args[1] = &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.gzName)}
			} else {
				call.Args = append(call.Args, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.gzName)})
			}
			prefix := target.pkgDir
			if !target.rootPath {
				prefix = ""
			}
			call.Args = append(call.Args, &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", prefix)})
			setCallExprName(call, "resources_monitor.NewGzipResourceMonitor")

		case "filesys.NewEmbedFS", "filesys.NewEmbedSubFS":
			// → gzip_embed.NewPreprocessingEmbed(&var, "gzName", true)
			// or → gzip_embed.NewPreprocessingEmbedWithXORKey(&var, "gzName", true, []byte("key"))
			// Wrapped in closure IIFE to handle (T, error) return
			call.Args[0] = &ast.UnaryExpr{Op: token.AND, X: call.Args[0]}
			cacheIdent := &ast.Ident{Name: "true"}

			var embedCall *ast.CallExpr
			if target.xorKey != "" {
				embedCall = &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   &ast.Ident{Name: "gzip_embed"},
						Sel: &ast.Ident{Name: "NewPreprocessingEmbedWithXORKey"},
					},
					Args: []ast.Expr{
						call.Args[0],
						&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.gzName)},
						cacheIdent,
						&ast.CallExpr{
							Fun:  &ast.ArrayType{Elt: &ast.Ident{Name: "byte"}},
							Args: []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.xorKey)}},
						},
					},
				}
			} else {
				embedCall = &ast.CallExpr{
					Fun: &ast.SelectorExpr{
						X:   &ast.Ident{Name: "gzip_embed"},
						Sel: &ast.Ident{Name: "NewPreprocessingEmbed"},
					},
					Args: []ast.Expr{
						call.Args[0],
						&ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.gzName)},
						cacheIdent,
					},
				}
			}

			// Replace with closure IIFE: func() fi.FileSystem { fs, _ := <call>; return fs }()
			*call = ast.CallExpr{
				Fun: &ast.FuncLit{
					Type: &ast.FuncType{
						Results: &ast.FieldList{
							List: []*ast.Field{{
								Type: &ast.SelectorExpr{
									X:   &ast.Ident{Name: "fi"},
									Sel: &ast.Ident{Name: "FileSystem"},
								},
							}},
						},
					},
					Body: &ast.BlockStmt{
						List: []ast.Stmt{
							&ast.AssignStmt{
								Lhs: []ast.Expr{
									&ast.Ident{Name: "fs"},
									&ast.Ident{Name: "_"},
								},
								Tok: token.DEFINE,
								Rhs: []ast.Expr{embedCall},
							},
							&ast.ReturnStmt{
								Results: []ast.Expr{&ast.Ident{Name: "fs"}},
							},
						},
					},
				},
			}

		case "gzip_embed.NewPreprocessingEmbed", "gzip_embed.NewPreprocessingEmbedWithXORKey":
			// Already using gzip_embed - just update the filename arg
			if len(call.Args) >= 2 {
				if lit, ok := call.Args[1].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					call.Args[1] = &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", target.gzName)}
				}
			}
		}

		return true
	})

	// Fix imports - only add gzip_embed if we generated direct gzip_embed calls
	needGzipEmbed := false
	ast.Inspect(f, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			name := getCallExprName(call)
			if strings.HasPrefix(name, "gzip_embed.") {
				needGzipEmbed = true
				return false
			}
		}
		return true
	})
	if needGzipEmbed {
		ensureImport(f, "github.com/yaklang/yaklang/common/utils/gzip_embed")
	}

	// Remove unused filesys import if filesys is no longer referenced
	needFilesys := false
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok2 := sel.X.(*ast.Ident); ok2 && ident.Name == "filesys" {
				needFilesys = true
				return false
			}
		}
		return true
	})
	if !needFilesys {
		removeImport(f, "github.com/yaklang/yaklang/common/utils/filesys")
	}

	// Check if we need fi.FileSystem (used in closure IIFE return types)
	// We scan AST for any SelectorExpr with X="fi"
	needFi := false
	ast.Inspect(f, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok {
			if ident, ok2 := sel.X.(*ast.Ident); ok2 && ident.Name == "fi" {
				needFi = true
				return false
			}
		}
		return true
	})
	if needFi {
		ensureImportAlias(f, "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface", "fi")
	}

	// Print modified AST
	var buf strings.Builder
	if err := printer.Fprint(&buf, fset, f); err != nil {
		return src, fmt.Errorf("print: %w", err)
	}
	return buf.String(), nil
}

// getCallExprName extracts the full function name from a CallExpr (e.g. "pkg.FuncName")
func getCallExprName(call *ast.CallExpr) string {
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		if ident, ok := fn.X.(*ast.Ident); ok {
			return ident.Name + "." + fn.Sel.Name
		}
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return ""
}

// setCallExprName sets the function name of a CallExpr
func setCallExprName(call *ast.CallExpr, fullName string) {
	parts := strings.SplitN(fullName, ".", 2)
	if len(parts) == 2 {
		call.Fun = &ast.SelectorExpr{
			X:   &ast.Ident{Name: parts[0]},
			Sel: &ast.Ident{Name: parts[1]},
		}
	} else {
		call.Fun = &ast.Ident{Name: parts[0]}
	}
}

// getExprVarName extracts the variable name from an expression (handles &var)
func getExprVarName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.UnaryExpr:
		if e.Op == token.AND {
			return getExprVarName(e.X)
		}
	}
	return ""
}

// findGzipEmbedCounterpart finds the gzip_embed.go file in the same directory
// that corresponds to an embed.go file with //go:build !gzip_embed.
func findGzipEmbedCounterpart(goFile string) string {
	dir := filepath.Dir(goFile)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if path == goFile {
			continue
		}
		// Check if this file has //go:build gzip_embed
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), "//go:build gzip_embed") {
			return path
		}
	}
	return ""
}

// ensureImportAlias adds an import with alias if not already present
func ensureImportAlias(f *ast.File, importPath, alias string) {
	for _, imp := range f.Imports {
		if strings.Contains(imp.Path.Value, importPath) {
			return
		}
	}

	var importDecl *ast.GenDecl
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			importDecl = gd
			break
		}
	}

	if importDecl == nil {
		importDecl = &ast.GenDecl{Tok: token.IMPORT}
		f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
	}

	importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
		Name: &ast.Ident{Name: alias},
		Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", importPath)},
	})
}

// removeImport removes an import by path
func removeImport(f *ast.File, importPath string) {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.IMPORT {
			continue
		}
		var newSpecs []ast.Spec
		for _, spec := range gd.Specs {
			imp, ok := spec.(*ast.ImportSpec)
			if !ok {
				newSpecs = append(newSpecs, spec)
				continue
			}
			if strings.Contains(imp.Path.Value, importPath) {
				continue // skip this import
			}
			newSpecs = append(newSpecs, spec)
		}
		gd.Specs = newSpecs
	}
}

// ensureImport adds an import if not already present
func ensureImport(f *ast.File, importPath string) {
	for _, imp := range f.Imports {
		if strings.Contains(imp.Path.Value, importPath) {
			return
		}
	}

	var importDecl *ast.GenDecl
	for _, decl := range f.Decls {
		if gd, ok := decl.(*ast.GenDecl); ok && gd.Tok == token.IMPORT {
			importDecl = gd
			break
		}
	}

	if importDecl == nil {
		importDecl = &ast.GenDecl{Tok: token.IMPORT}
		f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
	}

	importDecl.Specs = append(importDecl.Specs, &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: fmt.Sprintf("%q", importPath)},
	})
}
