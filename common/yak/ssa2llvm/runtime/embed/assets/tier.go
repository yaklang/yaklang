package assets

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/tiers"
)

// A tier is a libyak.a built with a smaller module set, so the Go linker drops
// the unused modules' metadata as well as their code (see runtime/tiers). One
// tier is embedded in the compiler; the rest, if present, are read from disk.
//
// Resolution is best-effort by design: a missing tier archive costs binary
// size, never correctness. The compiler falls back to the next tier up and
// ultimately to the embedded one, which is why EmbeddedTier must always be the
// largest tier a build can be asked for.

// TierDirEnv overrides where tier archives are looked up.
const TierDirEnv = "SSA2LLVM_TIER_DIR"

// tierDirs lists the directories searched for <tier>/libyak.a, in order.
func tierDirs() []string {
	var dirs []string
	if d := os.Getenv(TierDirEnv); d != "" {
		dirs = append(dirs, d)
	}
	if exe, err := os.Executable(); err == nil {
		if exe, err = filepath.EvalSymlinks(exe); err == nil {
			dirs = append(dirs, filepath.Join(filepath.Dir(exe), "ssa2llvm-tiers"))
		}
	}
	if home := os.Getenv("YAKIT_HOME"); home != "" {
		dirs = append(dirs, filepath.Join(home, "ssa2llvm", "tiers"))
	} else if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, ".yakit", "ssa2llvm", "tiers"))
	}
	return dirs
}

// TierArchivePath returns the on-disk libyak.a for a tier, or "" when no
// directory in the search path has one.
func TierArchivePath(tier string) string {
	for _, dir := range tierDirs() {
		p := filepath.Join(dir, tier, "libyak.a")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() && st.Size() > 0 {
			return p
		}
	}
	return ""
}

// ResolvedTier records which tier's archive a link actually used and where it
// came from, for tracing and for the compile cache key.
type ResolvedTier struct {
	// Wanted is the smallest tier covering the script's modules.
	Wanted string
	// Used is the tier whose archive was linked. It is Wanted when that
	// archive was available, otherwise the first larger tier that was.
	Used string
	// Source is "embedded" or the on-disk path the archive came from.
	Source string
}

// Fallback reports whether size was given up because the wanted tier was not
// installed.
func (r ResolvedTier) Fallback() bool { return r.Wanted != r.Used }

// ReleaseTierTo releases the runtime assets like ReleaseTo, but resolves
// libyak.a to the smallest available tier at or above wanted. The returned
// paths are otherwise identical, and the archive is a private copy the caller
// may patch in place.
func ReleaseTierTo(dir, wanted string) (ReleasedPaths, ResolvedTier, error) {
	rp, err := ReleaseTo(dir)
	if err != nil {
		return rp, ResolvedTier{}, err
	}
	res := ResolvedTier{Wanted: wanted, Used: EmbeddedTier, Source: "embedded"}
	if wanted == "" || wanted == EmbeddedTier {
		return rp, res, nil
	}
	// Walk up from the wanted tier: the first installed archive is the
	// smallest one that still covers every module the script uses. Embedded
	// tier archives are the default; an on-disk archive (SSA2LLVM_TIER_DIR or
	// the search path) overrides the embedded one.
	for _, t := range tiers.AtLeast(wanted) {
		if t.Name == EmbeddedTier {
			break
		}
		if src := TierArchivePath(t.Name); src != "" {
			if err := copyFile(src, rp.Libyak); err != nil {
				return rp, res, fmt.Errorf("install tier %q archive: %w", t.Name, err)
			}
			res.Used, res.Source = t.Name, src
			break
		}
		if data, err := embeddedTierArchive(t.Name); err == nil && len(data) > 0 {
			if err := os.WriteFile(rp.Libyak, data, 0o644); err != nil {
				return rp, res, fmt.Errorf("install embedded tier %q archive: %w", t.Name, err)
			}
			res.Used, res.Source = t.Name, "embedded:"+t.Name
			break
		}
	}
	return rp, res, nil
}

// embeddedTierArchive returns the embedded libyak.a bytes for a tier name.
func embeddedTierArchive(name string) ([]byte, error) {
	return embeddedTiers.ReadFile("tiers/" + name + "/libyak.a")
}

// TierStateKey identifies the installed tier archives for the compile cache.
// The embedded archive is covered by its manifest SHA256, but a tier archive
// lives on disk and can be installed, removed or rebuilt between compiles;
// each of those produces a different binary from the same script. Size and
// mtime stand in for a content hash: rehashing hundreds of megabytes on every
// compile would cost more than the cache saves.
func TierStateKey() string {
	var b strings.Builder
	b.WriteString("embedded=" + EmbeddedTier + ";")
	for _, t := range tiers.All {
		b.WriteString(t.Name)
		b.WriteByte('=')
		p := TierArchivePath(t.Name)
		if p == "" {
			b.WriteString("absent;")
			continue
		}
		st, err := os.Stat(p)
		if err != nil {
			b.WriteString("absent;")
			continue
		}
		fmt.Fprintf(&b, "%d:%d;", st.Size(), st.ModTime().UnixNano())
	}
	return b.String()
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
