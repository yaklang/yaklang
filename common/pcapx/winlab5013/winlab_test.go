package main

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCapturesParseTwiceAndRejectDamage(t *testing.T) {
	for _, b := range allBundles() {
		pcaps, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatalf("%s: %v", b.Name, err)
		}
		again, _, err := bundleCritical(b)
		if err != nil {
			t.Fatalf("%s second: %v", b.Name, err)
		}
		second, err := verifyNamed(b.Caps[0].File, again[b.Caps[0].File])
		if err != nil {
			t.Fatal(err)
		}
		first, err := verifyNamed(b.Caps[0].File, pcaps[b.Caps[0].File])
		if err != nil || first != second {
			t.Fatalf("%s not stable", b.Name)
		}
		readme := renderREADME(b, pcaps, critical)
		if !strings.Contains(readme, "Wireshark") || !strings.Contains(readme, "BEGIN CRITICAL") || !strings.Contains(readme, critical) {
			t.Fatalf("%s readme missing critical or wireshark notes", b.Name)
		}
		for _, c := range b.Caps {
			if !strings.Contains(c.Shark, "过滤器") && !strings.Contains(c.Shark, "`") {
				t.Fatalf("%s missing shark note", c.File)
			}
			if c.Unfinished == "" || c.Gap == "" || c.Metrics == "" {
				t.Fatalf("%s incomplete doc", c.File)
			}
			if _, err := verifyNamed(c.File, nil); err == nil {
				t.Fatalf("%s accepted a missing capture", c.File)
			}
			damaged := append([]byte{}, pcaps[c.File]...)
			if len(damaged) < 32 {
				t.Fatalf("%s too small", c.File)
			}
			damaged = damaged[:len(damaged)/2]
			if _, err := verifyNamed(c.File, damaged); err == nil {
				t.Fatalf("%s accepted a truncated capture", c.File)
			}
		}
		if strings.Contains(b.Name, "ctf-") {
			for _, raw := range pcaps {
				out, err := verifyNamed(onlyName(b), raw)
				if err != nil {
					t.Fatal(err)
				}
				flag := flagOf(out)
				if flag == "" || flag == "flag{not-this}" || flag == "flag{not-the-ftp-flag}" || bytes.Contains(raw, []byte(flag)) {
					t.Fatalf("%s flag %q is missing, a decoy, or still contiguous in the file", b.Name, flag)
				}
			}
		}
	}
}

func TestCommittedArchives(t *testing.T) {
	for _, b := range allBundles() {
		raw, err := os.ReadFile(filepath.Join("zips", b.Name))
		if err != nil {
			t.Fatalf("committed %s: %v", b.Name, err)
		}
		if len(raw) >= 100*1024*1024 {
			t.Fatalf("%s is %d bytes", b.Name, len(raw))
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		readme, err := readZipREADME(zr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := criticalFromREADME(readme)
		if err != nil {
			t.Fatal(err)
		}
		_, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatal(err)
		}
		if got != critical {
			t.Fatalf("committed %s critical does not match the parser", b.Name)
		}
		for _, c := range b.Caps {
			f, err := findZip(zr, "captures/"+c.File)
			if err != nil {
				t.Fatal(err)
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			buf, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(buf, buildCapture(c)) {
				t.Fatalf("committed %s is not the generated capture", c.File)
			}
		}
	}
}

func onlyName(b zipBundle) string { return b.Caps[0].File }

func flagOf(out string) string {
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "flag=") {
			return strings.TrimPrefix(line, "flag=")
		}
	}
	return ""
}

func TestZipVerifierMatchesCapture(t *testing.T) {
	for _, b := range allBundles() {
		raw, err := zipBytes(b)
		if err != nil {
			t.Fatalf("pack %s: %v", b.Name, err)
		}
		if len(raw) >= 100*1024*1024 {
			t.Fatalf("%s too big", b.Name)
		}
		zr, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		readme, err := readZipREADME(zr)
		if err != nil {
			t.Fatal(err)
		}
		got, err := criticalFromREADME(readme)
		if err != nil {
			t.Fatal(err)
		}
		_, critical, err := bundleCritical(b)
		if err != nil {
			t.Fatal(err)
		}
		if got != critical {
			t.Fatalf("%s readme critical drifted", b.Name)
		}
		dir := t.TempDir()
		for _, f := range zr.File {
			target := filepath.Join(dir, filepath.FromSlash(f.Name))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				t.Fatal(err)
			}
			rc, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			buf, err := io.ReadAll(rc)
			rc.Close()
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(target, buf, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		goBin := filepath.Join(runtime.GOROOT(), "bin", "go.exe")
		if _, err := os.Stat(goBin); err != nil {
			goBin = filepath.Join(runtime.GOROOT(), "bin", "go")
		}
		var last string
		for i := 0; i < 2; i++ {
			cmd := exec.Command(goBin, "run", ".")
			cmd.Dir = dir
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("%s run %d: %v\n%s", b.Name, i+1, err, out)
			}
			if i == 1 && string(out) != last {
				t.Fatalf("%s runs differ", b.Name)
			}
			last = string(out)
		}
		if last != critical {
			t.Fatalf("%s go run output != parsed critical\n%s", b.Name, last)
		}
		capture := filepath.Join(dir, "captures", b.Caps[0].File)
		cut, err := os.ReadFile(capture)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(capture, cut[:len(cut)/2], 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(goBin, "run", ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err == nil {
			t.Fatalf("%s still succeeded after truncation:\n%s", b.Name, out)
		}
	}
}
