package pptparser

import (
	"testing"

	"github.com/yaklang/yaklang/common/utils/fileparser/resources"
)

func TestParsePPTX(t *testing.T) {
	// Parse the file
	content, err := resources.FS.ReadFile("test.pptx")
	if err != nil {
		t.Fatalf("Failed to read PPTX file: %v", err)
	}
	nodes, err := ParsePPTX(content)
	if err != nil {
		t.Fatalf("Failed to parse PPTX file: %v", err)
	}

	// Verify we got some nodes
	if len(nodes) == 0 {
		t.Errorf("No nodes extracted from PPTX")
	}
	// Test classification
	classifier := ClassifyNodes(nodes)
	files := classifier.DumpToFiles()

	// Verify we got some files
	if len(files) == 0 {
		t.Errorf("No files generated from PPTX nodes")
	}
}

func TestOpcPackage(t *testing.T) {
	// This test is focused just on the OPC implementation
	// It can run without requiring a full parse

	// Test creating a new package URI
	uri := NewPackURI("/ppt/slides/slide1.xml")

	// Test base URI
	baseURI := uri.BaseURI()
	if baseURI != "/ppt/slides" {
		t.Errorf("Expected base URI '/ppt/slides', got '%s'", baseURI)
	}

	// Test extension
	ext := uri.Ext()
	if ext != ".xml" {
		t.Errorf("Expected extension '.xml', got '%s'", ext)
	}

	// Test relative reference
	relRef := uri.RelativeRef("/ppt")
	expectedRef := "slides/slide1.xml"
	if relRef != expectedRef {
		t.Errorf("Expected relative reference '%s', got '%s'", expectedRef, relRef)
	}

	// Test from relative reference
	absURI := PackURI{}.FromRelRef("/ppt", "slides/slide1.xml")
	if absURI.URI != "/ppt/slides/slide1.xml" {
		t.Errorf("Expected absolute URI '/ppt/slides/slide1.xml', got '%s'", absURI.URI)
	}

	// Test relationships
	rels := NewRelationships("/ppt")

	// Create some mock parts
	pkg := NewOpcPackage("test.pptx")
	part1 := NewPart(NewPackURI("/ppt/slides/slide1.xml"), "application/xml", pkg, nil)
	part2 := NewPart(NewPackURI("/ppt/slides/slide2.xml"), "application/xml", pkg, nil)

	// Add relationships
	rId1 := rels.GetOrAdd(RelationshipType("http://test/rel1"), part1)
	_ = rels.GetOrAdd(RelationshipType("http://test/rel2"), part2)

	// Test getting relationship by ID
	rel1, found := rels.Get(rId1)
	if !found {
		t.Errorf("Relationship with ID '%s' not found", rId1)
	} else {
		if rel1.RelType() != RelationshipType("http://test/rel1") {
			t.Errorf("Expected rel type 'http://test/rel1', got '%s'", rel1.RelType())
		}
	}

	// Test finding by rel type
	foundPart, err := rels.PartWithRelType(RelationshipType("http://test/rel2"))
	if err != nil {
		t.Errorf("Failed to find part by rel type: %v", err)
	} else if foundPart != part2 {
		t.Errorf("Got wrong part when searching by rel type")
	}

	// Test external relationship
	extRId := rels.GetOrAddExtRel(RelationshipType("http://test/ext"), "https://example.com")
	extRel, found := rels.Get(extRId)
	if !found {
		t.Errorf("External relationship not found")
	} else if !extRel.IsExternal() {
		t.Errorf("Relationship should be external")
	}
}
