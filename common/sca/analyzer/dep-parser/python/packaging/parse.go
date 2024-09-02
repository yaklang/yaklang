package packaging

import (
	"bufio"
	"errors"
	"io"
	"net/textproto"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/sca/analyzer/dep-parser/types"
	"github.com/yaklang/yaklang/common/utils"
	fi "github.com/yaklang/yaklang/common/utils/filesys/filesys_interface"
)

type Parser struct{}

func NewParser() types.Parser {
	return &Parser{}
}

// Parse parses egg and wheel metadata.
// e.g. .egg-info/PKG-INFO and dist-info/METADATA
func (*Parser) Parse(fs fi.FileSystem, r types.ReadSeekerAt) ([]types.Library, []types.Dependency, error) {
	rd := textproto.NewReader(bufio.NewReader(r))
	h, err := rd.ReadMIMEHeader()
	if e := textproto.ProtocolError(""); errors.As(err, &e) {
		// A MIME header may contain bytes in the key or value outside the set allowed by RFC 7230.
		// cf. https://cs.opensource.google/go/go/+/a6642e67e16b9d769a0c08e486ba08408064df19
		// However, our required key/value could have been correctly parsed,
		// so we continue with the subsequent process.
		log.Debugf("MIME protocol error: %s", err)
	} else if err != nil && err != io.EOF {
		return nil, nil, utils.Errorf("read MIME error: %w", err)
	}

	name, version := h.Get("name"), h.Get("version")
	if name == "" || version == "" {
		return nil, nil, utils.Error("name or version is empty")
	}

	// "License-Expression" takes precedence as "License" is deprecated.
	// cf. https://peps.python.org/pep-0639/#deprecate-license-field
	var license string
	if l := h.Get("License-Expression"); l != "" {
		license = l
	} else if l := h.Get("License"); l != "" {
		license = l
	} else {
		for _, classifier := range h.Values("Classifier") {
			if strings.HasPrefix(classifier, "License :: ") {
				values := strings.Split(classifier, " :: ")
				license = values[len(values)-1]
				break
			}
		}
	}
	if license == "" && h.Get("License-File") != "" {
		license = "file://" + h.Get("License-File")
	}

	return []types.Library{
		{
			Name:    name,
			Version: version,
			License: license,
		},
	}, nil, nil
}
