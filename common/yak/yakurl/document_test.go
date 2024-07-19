package yakurl

import (
	"testing"

	"github.com/yaklang/yaklang/common/log"
)

func TestDocumentScheme(t *testing.T) {
	rsp, err := LoadGetResource(`yakdocument://str/`)
	if err != nil {
		t.Fatal(err)
	}
	max := rsp.GetResources()
	if len(max) <= 0 {
		t.Fatal("empty result for yakdocument")
	}

	rsp, err = LoadGetResource(`yakdocument://str.calc`)
	if err != nil {
		t.Fatal(err)
	}
	lib := rsp.GetResources()
	if len(lib) <= 0 {
		t.Fatal("empty result for yakdocument")
	}
}

func TestDocumentScheme2(t *testing.T) {
	rsp, err := LoadGetResource(`yakdocument:///`)
	if err != nil {
		t.Fatal(err)
	}
	max := rsp.GetResources()
	if len(max) <= 0 {
		t.Fatal("empty result for yakdocument")
	}
}

func TestDocument_Description(t *testing.T) {
	rsp, err := LoadGetResource(`yakdocument://str.calc`)
	if err != nil {
		t.Fatal(err)
	}
	lib := rsp.GetResources()
	log.Infof("lib: %v", lib)
	if len(lib) <= 0 {
		t.Fatal("empty result for yakdocument")
	}

	if len(lib[0].Extra) <= 0 {
		t.Fatal("empty result for yakdocument")
	}

	if lib[0].Extra[0].Key != "Content" {
		t.Fatal("unexpected result for yakdocument")
	}
	if len(lib[0].Extra[0].Value) <= 0 {
		t.Fatal("empty ")
	}
}
