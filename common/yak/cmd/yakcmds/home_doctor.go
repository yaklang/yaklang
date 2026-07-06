package yakcmds

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/urfavecli"
	"github.com/yaklang/yaklang/common/utils"
)

type homeDoctorPathEntry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	UnderHome  bool   `json:"under_home"`
	Intentional bool  `json:"intentional,omitempty"`
	Note       string `json:"note,omitempty"`
}

type homeDoctorResult struct {
	YakitHome string                `json:"yakit_home"`
	Entries   []homeDoctorPathEntry `json:"entries"`
	Issues    []string              `json:"issues"`
}

func isUnderYakitHome(path, yakitHome string) bool {
	if yakitHome == "" || path == "" {
		return false
	}
	absHome, err1 := filepath.Abs(yakitHome)
	absPath, err2 := filepath.Abs(path)
	if err1 != nil || err2 != nil {
		return strings.HasPrefix(filepath.Clean(path), filepath.Clean(yakitHome)+string(os.PathSeparator))
	}
	rel, err := filepath.Rel(absHome, absPath)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func probeOpenTempFileDir() string {
	f, err := utils.OpenTempFile("home-doctor-probe.tmp")
	if err != nil {
		return fmt.Sprintf("<error: %v>", err)
	}
	dir := filepath.Dir(f.Name())
	_ = f.Close()
	_ = os.Remove(f.Name())
	return dir
}

var HomeDoctorCommand = &cli.Command{
	Name:    "home-doctor",
	Aliases: []string{"doctor"},
	Usage:   "Print yak/yakit on-disk path roots and check YAKIT_HOME consistency",
	Flags: []cli.Flag{
		cli.BoolFlag{
			Name:  "json",
			Usage: "Output JSON",
		},
	},
	Action: func(c *cli.Context) error {
		yakitHome := consts.GetDefaultYakitBaseDir()
		baseHome := consts.GetDefaultBaseHomeDir()
		openTempDir := probeOpenTempFileDir()
		result := homeDoctorResult{
			YakitHome: yakitHome,
			Entries: []homeDoctorPathEntry{
				{Name: "yakit_home", Path: yakitHome, UnderHome: true},
				{Name: "base_home", Path: baseHome, UnderHome: false, Intentional: true, Note: "parent directory of yakit_home"},
				{Name: "temp", Path: consts.GetDefaultYakitBaseTempDir(), UnderHome: isUnderYakitHome(consts.GetDefaultYakitBaseTempDir(), yakitHome)},
				{Name: "engine", Path: consts.GetDefaultYakitEngineDir(), UnderHome: isUnderYakitHome(consts.GetDefaultYakitEngineDir(), yakitHome)},
				{Name: "projects", Path: consts.GetDefaultYakitProjectsDir(), UnderHome: isUnderYakitHome(consts.GetDefaultYakitProjectsDir(), yakitHome)},
				{Name: "payloads", Path: consts.GetDefaultYakitPayloadsDir(), UnderHome: isUnderYakitHome(consts.GetDefaultYakitPayloadsDir(), yakitHome)},
				{Name: "nuclei_templates", Path: consts.GetNucleiTemplatesDir(), UnderHome: isUnderYakitHome(consts.GetNucleiTemplatesDir(), yakitHome), Intentional: true, Note: "under base_home sibling to yakit_home"},
				{Name: "open_temp_file_dir", Path: openTempDir, UnderHome: isUnderYakitHome(openTempDir, yakitHome)},
				{Name: "machine_id", Path: filepath.Join(baseHome, ".ym-id"), UnderHome: isUnderYakitHome(filepath.Join(baseHome, ".ym-id"), yakitHome), Intentional: true, Note: "stored under base_home"},
			},
		}

		for i := range result.Entries {
			entry := &result.Entries[i]
			if entry.Intentional {
				continue
			}
			if !entry.UnderHome {
				result.Issues = append(result.Issues, fmt.Sprintf("%s is outside YAKIT_HOME: %s", entry.Name, entry.Path))
			}
		}

		if c.Bool("json") {
			raw, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(raw))
			return nil
		}

		fmt.Printf("YAKIT_HOME=%s\n", result.YakitHome)
		if envHome := os.Getenv("YAKIT_HOME"); envHome != "" {
			fmt.Printf("YAKIT_HOME (env)=%s\n", envHome)
		}
		for _, entry := range result.Entries {
			status := "ok"
			if !entry.UnderHome && !entry.Intentional {
				status = "OUTSIDE"
			}
			fmt.Printf("[%s] %s = %s", status, entry.Name, entry.Path)
			if entry.Note != "" {
				fmt.Printf(" (%s)", entry.Note)
			}
			fmt.Println()
		}
		if len(result.Issues) > 0 {
			fmt.Println("Issues:")
			for _, issue := range result.Issues {
				fmt.Printf("  - %s\n", issue)
			}
			return fmt.Errorf("found %d path issue(s)", len(result.Issues))
		}
		fmt.Println("All checked paths are under YAKIT_HOME (or marked intentional).")
		return nil
	},
}
