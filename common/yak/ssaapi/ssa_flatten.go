package ssaapi

import (
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/filesys"
	"github.com/yaklang/yaklang/common/yak/ssa/ssadb"
)

// SetDatabase sets the active SSA database from a DSN string (e.g.
// "sqlite:///data/ci-ssa/default-yakssa.db"). This is the script-facing
// equivalent of the CLI --database flag and is required so a .yak script can
// target a specific SSA DB file. Exported to scripts as ssa.SetDatabase.
func SetDatabase(raw string) error {
	return consts.SetGormSSAProjectDatabaseByInfo(raw)
}

// DeleteProgram removes a program (and all its code/index/source/type rows)
// from the active SSA database. Exported to scripts as ssa.DeleteProgram.
func DeleteProgram(programName string) error {
	if programName == "" {
		return utils.Errorf("program name is empty")
	}
	ssadb.DeleteProgram(ssadb.GetDB(), programName)
	return nil
}

// ListPrograms returns all program names in the active SSA database.
// Exported to scripts as ssa.ListPrograms.
func ListPrograms() []string {
	return ssadb.AllProgramNames(ssadb.GetDB())
}

// GetOverlayFiles extracts the aggregated (shadow-resolved, delete-applied)
// file system of an overlay program as a map of {path: content}. This is the
// bridge that lets a .yak script flatten a multi-layer overlay: the script
// writes these files to a temp directory, then re-compiles them as a single
// non-incremental program. Exported to scripts as ssa.GetOverlayFiles.
//
// If the program is not an overlay (single-layer base or plain program),
// the function returns an error so the caller can skip flattening.
func GetOverlayFiles(programName string) (map[string]string, error) {
	if programName == "" {
		return nil, utils.Errorf("program name is empty")
	}

	prog, err := FromDatabase(programName)
	if err != nil {
		return nil, utils.Wrapf(err, "failed to load program %s", programName)
	}
	if prog == nil {
		return nil, utils.Errorf("program %s not found", programName)
	}

	// Must be an overlay to be worth flattening.
	if !prog.IsIncrementalCompile() {
		return nil, utils.Errorf("program %s is not an overlay (single-layer), no flatten needed", programName)
	}

	overlay := prog.GetOverlay()
	if overlay == nil {
		return nil, utils.Errorf("program %s is marked incremental but has no overlay", programName)
	}

	aggregatedFS := overlay.GetAggregatedFileSystem()
	if aggregatedFS == nil {
		return nil, utils.Errorf("failed to build aggregated file system for %s", programName)
	}

	files := make(map[string]string)
	err = filesys.Recursive(".",
		filesys.WithFileSystem(aggregatedFS),
		filesys.WithStat(func(isDir bool, pathname string, _ os.FileInfo) error {
			if isDir {
				return nil
			}
			if pathname == "" {
				return nil
			}

			content, err := aggregatedFS.ReadFile(pathname)
			if err != nil {
				return nil // skip unreadable files
			}

			// Strip the program-name prefix so paths are relative.
			cleanPath := removeProgramNamePrefix(pathname, programName)
			if cleanPath == "" || cleanPath == "/" {
				return nil
			}
			cleanPath = strings.TrimPrefix(cleanPath, "/")

			if cleanPath != "" {
				files[cleanPath] = string(content)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, utils.Wrap(err, "failed to traverse aggregated file system")
	}

	if len(files) == 0 {
		return nil, utils.Errorf("aggregated file system for %s contains no files", programName)
	}

	return files, nil
}
