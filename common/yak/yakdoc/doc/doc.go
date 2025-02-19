package doc

import (
	"bytes"
	_ "embed"
	"encoding/gob"
	"sync"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/yakdoc"
)

//go:embed doc.gob.gzip
var embedDocument []byte

var (
	defaultDocumentHelper *yakdoc.DocumentHelper
	once                  sync.Once
)

func GetDefaultDocumentHelper() *yakdoc.DocumentHelper {
	once.Do(func() {
		buf, err := utils.GzipDeCompress(embedDocument)
		if err != nil {
			log.Warnf("load embed yak document error: %v", err)
		}

		decoder := gob.NewDecoder(bytes.NewReader(buf))
		if err := decoder.Decode(&defaultDocumentHelper); err != nil {
			log.Warnf("load embed yak document error: %v", err)
		}
	})
	return defaultDocumentHelper
}

func GetDocumentFunctions(libName string) map[string]*yakdoc.FuncDecl {
	helper := GetDefaultDocumentHelper()
	if libName == "" {
		return helper.Functions
	}
	lib, ok := helper.Libs[libName]
	if !ok {
		return nil
	}
	return lib.Functions
}

func GetDocumentInstances(libName string) map[string]*yakdoc.LibInstance {
	helper := GetDefaultDocumentHelper()
	if libName == "" {
		return helper.Instances
	}
	lib, ok := helper.Libs[libName]
	if !ok {
		return nil
	}
	return lib.Instances
}

func GetDocumentFunction(libName, funcName string) *yakdoc.FuncDecl {
	helper := GetDefaultDocumentHelper()
	if libName == "" {
		return helper.Functions[funcName]
	}
	lib, ok := helper.Libs[libName]
	if !ok {
		return nil
	}
	return lib.Functions[funcName]
}

func GetDocumentInstance(libName, instanceName string) *yakdoc.LibInstance {
	helper := GetDefaultDocumentHelper()
	if libName == "" {
		return helper.Instances[instanceName]
	}
	lib, ok := helper.Libs[libName]
	if !ok {
		return nil
	}
	return lib.Instances[instanceName]
}
