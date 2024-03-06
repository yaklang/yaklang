package yakurl

import "testing"

func TestDocumentScheme(t *testing.T) {
	t.Skip("temp non-implemented")

	rsp, err := LoadGetResource(`yakdocument://str/`)
	if err != nil {
		t.Fatal(err)
	}
	max := rsp.GetResources()
	if len(max) <= 0 {
		t.Fatal("empty result for yakdocument")
	}

	rsp, err = LoadGetResource(`yakdocument://str/calc`)
	if err != nil {
		t.Fatal(err)
	}
	lib := rsp.GetResources()
	if len(lib) <= 0 {
		t.Fatal("empty result for yakdocument")
	}

	if len(lib) >= len(max) {
		t.Fatal("unexpected result for yakdocument")
	}
}

func TestDocumentScheme2(t *testing.T) {
	t.Skip("temp non-implemented")
	
	rsp, err := LoadGetResource(`yakdocument:///`)
	if err != nil {
		t.Fatal(err)
	}
	max := rsp.GetResources()
	if len(max) <= 0 {
		t.Fatal("empty result for yakdocument")
	}
}
