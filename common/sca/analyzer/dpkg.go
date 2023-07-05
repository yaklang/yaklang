package analyzer

import (
	"bufio"
	"bytes"
	"io"
	"io/fs"
	"net/textproto"
	"strings"

	"github.com/yaklang/yaklang/common/sca/types"
	"github.com/yaklang/yaklang/common/utils"
)

const (
	statusFile = "var/lib/dpkg/status"
	statusDir  = "var/lib/dpkg/status.d/"
	// availableFile = "var/lib/dpkg/available"
	// infoDir       = "var/lib/dpkg/info/"
)

type dpkgAnalyzer struct {
}

// ReadBlock reads a data block from the underlying reader until a blank line is encountered.
func ReadBlock(r *bufio.Reader) ([]byte, error) {
	var block bytes.Buffer

	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}

		if line == "\n" {
			break
		}

		// block.WriteString(line)
		block.WriteString(line)
	}

	return block.Bytes(), nil
}

func NewDpkgAnalyzer() *dpkgAnalyzer {
	return &dpkgAnalyzer{}
}

func (a dpkgAnalyzer) parseStatus(s string) bool {
	for _, ss := range strings.Fields(s) {
		if ss == "deinstall" || ss == "purge" {
			return false
		}
	}
	return true
}
func (a dpkgAnalyzer) parseDpkgPkg(header textproto.MIMEHeader) *types.Package {
	if isInstalled := a.parseStatus(header.Get("Status")); !isInstalled {
		return nil
	}

	pkg := &types.Package{
		Name:    header.Get("Package"),
		Version: header.Get("Version"),
	}
	if pkg.Name == "" || pkg.Version == "" {
		return nil
	}

	return pkg
}

func (a dpkgAnalyzer) analyzeStatus(r io.Reader) ([]types.Package, error) {
	pkgs := make([]types.Package, 0)
	br := bufio.NewReader(r)
	for {
		block, err := ReadBlock(br)
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		if block == nil {
			break
		}
		reader := textproto.NewReader(bufio.NewReader(bytes.NewReader(block)))
		header, err := reader.ReadMIMEHeader()
		if err != nil && err != io.EOF {
			return nil, utils.Errorf("parse MIME header error: %v ", err)
		}
		pkg := a.parseDpkgPkg(header)
		pkgs = append(pkgs, *pkg)
	}

	return pkgs, nil
}

func (a dpkgAnalyzer) Match(path string, info fs.FileInfo) bool {
	if strings.HasPrefix(path, statusDir) || path == statusFile {
		// handler status
		return true
	}
	return false
}

func (a dpkgAnalyzer) Analyze(path string, r io.Reader) ([]types.Package, error) {
	if strings.HasPrefix(path, statusDir) || path == statusFile {
		// handler status
		return a.analyzeStatus(r)
	}

	// if path == availableFile {
	// 	// handler available file

	// }
	return nil, nil
}
