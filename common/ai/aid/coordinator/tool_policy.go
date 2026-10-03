package coordinator

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// AllowedTool is an explicit built-in allowlist. Unknown tools (including MCP,
// script runners and command wrappers) do not acquire coordinator privileges.
func AllowedTool(name string, params aitool.InvokeParams, workdir string) bool {
	switch name {
	case "read_file", "read_file_lines", "list_dir", "list_files", "find_file", "find_files", "tree", "grep", "grep_files", "search_files", "search_knowledge", "query_knowledge_base", "yakdoc":
		return true
	case "write_file":
		path := params.GetString("file")
		if path == "" {
			path = params.GetString("path")
		}
		if path == "" {
			path = params.GetString("file_path")
		}
		if path == "" || workdir == "" {
			return false
		}
		root, err := filepath.Abs(filepath.Join(workdir, "artifacts"))
		if err != nil {
			return false
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(workdir, path)
		}
		path, err = filepath.Abs(path)
		if err != nil {
			return false
		}
		if !within(root, path) || !strings.EqualFold(filepath.Ext(path), ".md") {
			return false
		}
		// Resolve existing parents too: an artifacts directory/file symlink must
		// not grant write access to business files outside the work directory.
		base, err := resolveParents(workdir)
		if err != nil {
			return false
		}
		resolved, err := resolveParents(path)
		return err == nil && within(filepath.Join(base, "artifacts"), resolved)
	}
	return false
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != "." && rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func resolveParents(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(path); err == nil {
		return filepath.EvalSymlinks(path)
	} else if !os.IsNotExist(err) {
		return "", err
	}
	parent, err := resolveParents(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}
