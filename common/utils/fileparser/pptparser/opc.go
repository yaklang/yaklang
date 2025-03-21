package pptparser

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

// OPC related constants
const (
	ContentTypesURI = "[Content_Types].xml"
	PackageURI      = "/"
)

// RelationshipTargetMode defines the target mode for relationships
type RelationshipTargetMode string

const (
	RelationshipTargetModeInternal RelationshipTargetMode = "Internal"
	RelationshipTargetModeExternal RelationshipTargetMode = "External"
)

// RelationshipType defines common relationship types
type RelationshipType string

const (
	RelationshipTypeOfficeDocument RelationshipType = "http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument"
	// Add other relationship types as needed
)

// PackURI represents a package part URI
type PackURI struct {
	URI string
}

// NewPackURI creates a new PackURI
func NewPackURI(uri string) PackURI {
	return PackURI{URI: uri}
}

// BaseURI returns the base URI of the PackURI
func (p PackURI) BaseURI() string {
	return path.Dir(p.URI)
}

// Ext returns the extension of the URI
func (p PackURI) Ext() string {
	return strings.ToLower(path.Ext(p.URI))
}

// FromRelRef creates a PackURI from a relative reference
func (p PackURI) FromRelRef(baseURI, target string) PackURI {
	// Handle absolute URIs
	if strings.HasPrefix(target, "/") {
		return NewPackURI(target)
	}

	// Handle relative URIs
	return NewPackURI(path.Join(baseURI, target))
}

// RelativeRef returns the relative reference of this URI to a base URI
func (p PackURI) RelativeRef(baseURI string) string {
	// If baseURI and URI have the same directory, just return the filename
	if path.Dir(p.URI) == baseURI {
		return path.Base(p.URI)
	}

	// Otherwise, create a proper relative path
	rel, err := makeRelative(baseURI, p.URI)
	if err != nil {
		// Fallback to original URI if we can't make it relative
		return p.URI
	}
	return rel
}

// makeRelative creates a relative path from base to target
func makeRelative(base, target string) (string, error) {
	baseParts := splitPath(base)
	targetParts := splitPath(target)

	// Find common prefix
	i := 0
	for i < len(baseParts) && i < len(targetParts) && baseParts[i] == targetParts[i] {
		i++
	}

	// No common parts
	if i == 0 {
		return target, nil
	}

	// Build relative path
	var result strings.Builder

	// Add "../" for each remaining part in base
	for j := i; j < len(baseParts); j++ {
		result.WriteString("../")
	}

	// Add remaining parts from target
	for j := i; j < len(targetParts); j++ {
		if j > i {
			result.WriteString("/")
		}
		result.WriteString(targetParts[j])
	}

	return result.String(), nil
}

// splitPath splits a path into its components
func splitPath(p string) []string {
	// Clean the path first
	p = path.Clean(p)
	// Remove leading slash
	if strings.HasPrefix(p, "/") {
		p = p[1:]
	}
	// Handle empty string or "/"
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

// Relationship represents a relationship between parts
type Relationship struct {
	baseURI    string
	rID        string
	relType    RelationshipType
	targetMode RelationshipTargetMode
	target     interface{} // Can be a Part or a string
}

// NewRelationship creates a new relationship
func NewRelationship(baseURI, rID string, relType RelationshipType, targetMode RelationshipTargetMode, target interface{}) *Relationship {
	return &Relationship{
		baseURI:    baseURI,
		rID:        rID,
		relType:    relType,
		targetMode: targetMode,
		target:     target,
	}
}

// IsExternal returns true if the relationship is external
func (r *Relationship) IsExternal() bool {
	return r.targetMode == RelationshipTargetModeExternal
}

// RelType returns the relationship type
func (r *Relationship) RelType() RelationshipType {
	return r.relType
}

// RID returns the relationship ID
func (r *Relationship) RID() string {
	return r.rID
}

// TargetPart returns the target part of the relationship
func (r *Relationship) TargetPart() (*Part, error) {
	if r.IsExternal() {
		return nil, errors.New("target_part property on Relationship is undefined when target-mode is external")
	}
	part, ok := r.target.(*Part)
	if !ok {
		return nil, errors.New("target is not a Part")
	}
	return part, nil
}

// TargetRef returns the target reference of the relationship
func (r *Relationship) TargetRef() string {
	if r.IsExternal() {
		return r.target.(string)
	}

	part, err := r.TargetPart()
	if err != nil {
		return ""
	}

	partname := part.Partname()
	return partname.RelativeRef(r.baseURI)
}

// Relationships represents a collection of relationships
type Relationships struct {
	baseURI string
	rels    map[string]*Relationship
}

// NewRelationships creates a new relationships collection
func NewRelationships(baseURI string) *Relationships {
	return &Relationships{
		baseURI: baseURI,
		rels:    make(map[string]*Relationship),
	}
}

// Keys returns the keys of the relationships collection
func (r *Relationships) Keys() []string {
	keys := make([]string, 0, len(r.rels))
	for k := range r.rels {
		keys = append(keys, k)
	}
	return keys
}

// Values returns the values of the relationships collection
func (r *Relationships) Values() []*Relationship {
	values := make([]*Relationship, 0, len(r.rels))
	for _, v := range r.rels {
		values = append(values, v)
	}
	return values
}

// Get returns the relationship with the given ID
func (r *Relationships) Get(rID string) (*Relationship, bool) {
	rel, ok := r.rels[rID]
	return rel, ok
}

// Len returns the number of relationships
func (r *Relationships) Len() int {
	return len(r.rels)
}

// GetOrAdd adds a relationship or returns an existing one
func (r *Relationships) GetOrAdd(relType RelationshipType, targetPart *Part) string {
	// Check if relationship already exists
	for rID, rel := range r.rels {
		if rel.relType == relType && !rel.IsExternal() {
			part, err := rel.TargetPart()
			if err == nil && part == targetPart {
				return rID
			}
		}
	}

	// Add new relationship
	rID := r.nextRID()
	r.rels[rID] = NewRelationship(
		r.baseURI,
		rID,
		relType,
		RelationshipTargetModeInternal,
		targetPart,
	)
	return rID
}

// GetOrAddExtRel adds an external relationship or returns an existing one
func (r *Relationships) GetOrAddExtRel(relType RelationshipType, targetRef string) string {
	// Check if relationship already exists
	for rID, rel := range r.rels {
		if rel.relType == relType && rel.IsExternal() && rel.target.(string) == targetRef {
			return rID
		}
	}

	// Add new relationship
	rID := r.nextRID()
	r.rels[rID] = NewRelationship(
		r.baseURI,
		rID,
		relType,
		RelationshipTargetModeExternal,
		targetRef,
	)
	return rID
}

// PartWithRelType returns the part with the given relationship type
func (r *Relationships) PartWithRelType(relType RelationshipType) (*Part, error) {
	var matches []*Relationship

	for _, rel := range r.rels {
		if rel.relType == relType {
			matches = append(matches, rel)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no relationship of type '%s' in collection", relType)
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple relationships of type '%s' in collection", relType)
	}

	return matches[0].TargetPart()
}

// Pop removes a relationship and returns it
func (r *Relationships) Pop(rID string) (*Relationship, error) {
	rel, ok := r.rels[rID]
	if !ok {
		return nil, fmt.Errorf("no relationship with ID '%s'", rID)
	}

	delete(r.rels, rID)
	return rel, nil
}

// nextRID generates the next available relationship ID
func (r *Relationships) nextRID() string {
	// Find the highest rId number
	maxID := 0
	for rID := range r.rels {
		if strings.HasPrefix(rID, "rId") {
			// Try to parse the number after "rId"
			var id int
			_, err := fmt.Sscanf(rID, "rId%d", &id)
			if err == nil && id > maxID {
				maxID = id
			}
		}
	}

	// Next ID is one higher than the max
	return fmt.Sprintf("rId%d", maxID+1)
}

// LoadFromXML loads relationships from XML
func (r *Relationships) LoadFromXML(baseURI string, relXML []byte, parts map[PackURI]*Part) error {
	var rels struct {
		XMLName xml.Name `xml:"Relationships"`
		Rel     []struct {
			Id         string `xml:"Id,attr"`
			Type       string `xml:"Type,attr"`
			Target     string `xml:"Target,attr"`
			TargetMode string `xml:"TargetMode,attr"`
		} `xml:"Relationship"`
	}

	if err := xml.Unmarshal(relXML, &rels); err != nil {
		return err
	}

	// Clear existing relationships
	r.rels = make(map[string]*Relationship)

	// Load new relationships
	for _, rel := range rels.Rel {
		if rel.Id == "rId8" {
			log.Infof("rel: %+v", rel)
		}
		targetMode := RelationshipTargetModeInternal
		if rel.TargetMode == "External" {
			targetMode = RelationshipTargetModeExternal
		}

		var target interface{}
		if targetMode == RelationshipTargetModeExternal {
			target = rel.Target
		} else {
			// Convert target to part
			partname := PackURI{}.FromRelRef(baseURI, rel.Target)
			part, ok := parts[partname]
			if !ok {
				// Skip if part not found (could be NULL or similar)
				continue
			}
			target = part
		}

		r.rels[rel.Id] = NewRelationship(
			baseURI,
			rel.Id,
			RelationshipType(rel.Type),
			targetMode,
			target,
		)
	}

	return nil
}

// ToXML serializes relationships to XML
func (r *Relationships) ToXML() ([]byte, error) {
	type xmlRel struct {
		ID         string `xml:"Id,attr"`
		Type       string `xml:"Type,attr"`
		Target     string `xml:"Target,attr"`
		TargetMode string `xml:"TargetMode,attr,omitempty"`
	}

	type xmlRels struct {
		XMLName      xml.Name `xml:"Relationships"`
		Xmlns        string   `xml:"xmlns,attr"`
		Relationship []xmlRel `xml:"Relationship"`
	}

	// Create XML structure
	xmlData := xmlRels{
		Xmlns:        "http://schemas.openxmlformats.org/package/2006/relationships",
		Relationship: []xmlRel{},
	}

	// Sort relationships by ID
	sorted := r.Values()
	// TODO: Sort by ID if needed

	// Add relationships
	for _, rel := range sorted {
		xmlRel := xmlRel{
			ID:     rel.rID,
			Type:   string(rel.relType),
			Target: rel.TargetRef(),
		}

		if rel.targetMode == RelationshipTargetModeExternal {
			xmlRel.TargetMode = "External"
		}

		xmlData.Relationship = append(xmlData.Relationship, xmlRel)
	}

	// Marshal to XML
	output, err := xml.MarshalIndent(xmlData, "", "  ")
	if err != nil {
		return nil, err
	}

	// Add XML declaration
	return append([]byte(xml.Header), output...), nil
}

// Part represents a part in the package
type Part struct {
	partname    PackURI
	contentType string
	pkg         *OpcPackage
	blob        []byte
	rels        *Relationships
}

// NewPart creates a new part
func NewPart(partname PackURI, contentType string, pkg *OpcPackage, blob []byte) *Part {
	return &Part{
		partname:    partname,
		contentType: contentType,
		pkg:         pkg,
		blob:        blob,
	}
}

// Partname returns the part name
func (p *Part) Partname() PackURI {
	return p.partname
}

// SetPartname sets the part name
func (p *Part) SetPartname(partname PackURI) {
	p.partname = partname
}

// ContentType returns the content type
func (p *Part) ContentType() string {
	return p.contentType
}

// Package returns the package
func (p *Part) Package() *OpcPackage {
	return p.pkg
}

// Blob returns the blob data
func (p *Part) Blob() []byte {
	return p.blob
}

// SetBlob sets the blob data
func (p *Part) SetBlob(blob []byte) {
	p.blob = blob
}

// Rels returns the relationships
func (p *Part) Rels() *Relationships {
	if p.rels == nil {
		p.rels = NewRelationships(p.partname.BaseURI())
	}
	return p.rels
}

// LoadRelsFromXML loads relationships from XML
func (p *Part) LoadRelsFromXML(xml []byte, parts map[PackURI]*Part) error {
	return p.Rels().LoadFromXML(p.partname.BaseURI(), xml, parts)
}

// PartRelatedBy returns the part related by the given relationship type
func (p *Part) PartRelatedBy(relType RelationshipType) (*Part, error) {
	return p.Rels().PartWithRelType(relType)
}

// RelatedPart returns the related part with the given relationship ID
func (p *Part) RelatedPart(rID string) (*Part, error) {
	rel, ok := p.Rels().Get(rID)
	if !ok {
		return nil, fmt.Errorf("no relationship with ID '%s'", rID)
	}

	return rel.TargetPart()
}

// XmlPart represents a part containing XML
type XmlPart struct {
	*Part
	element interface{} // XML element
}

// NewXmlPart creates a new XML part
func NewXmlPart(partname PackURI, contentType string, pkg *OpcPackage, element interface{}) *XmlPart {
	return &XmlPart{
		Part:    NewPart(partname, contentType, pkg, nil),
		element: element,
	}
}

// Element returns the XML element
func (p *XmlPart) Element() interface{} {
	return p.element
}

// Blob returns the serialized XML
func (p *XmlPart) Blob() []byte {
	// Serialize XML
	output, err := xml.MarshalIndent(p.element, "", "  ")
	if err != nil {
		log.Errorf("Failed to serialize XML part: %v", err)
		return []byte{}
	}

	// Add XML declaration
	return append([]byte(xml.Header), output...)
}

// ContentTypeMap maps part names to content types
type ContentTypeMap struct {
	overrides map[string]string
	defaults  map[string]string
}

// NewContentTypeMap creates a new content type map
func NewContentTypeMap(overrides, defaults map[string]string) *ContentTypeMap {
	return &ContentTypeMap{
		overrides: overrides,
		defaults:  defaults,
	}
}

// GetContentType returns the content type for a part
func (c *ContentTypeMap) GetContentType(partname PackURI) (string, error) {
	// Check overrides
	if ct, ok := c.overrides[strings.ToLower(partname.URI)]; ok {
		return ct, nil
	}

	// Check defaults
	if ct, ok := c.defaults[strings.ToLower(strings.TrimLeft(partname.Ext(), "."))]; ok {
		return ct, nil
	}

	return "", fmt.Errorf("no content-type for partname '%s' in [Content_Types].xml", partname.URI)
}

// FromXML creates a ContentTypeMap from XML
func (c *ContentTypeMap) FromXML(contentTypesXML []byte) error {
	var types struct {
		XMLName  xml.Name `xml:"Types"`
		Override []struct {
			PartName    string `xml:"PartName,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Override"`
		Default []struct {
			Extension   string `xml:"Extension,attr"`
			ContentType string `xml:"ContentType,attr"`
		} `xml:"Default"`
	}

	if err := xml.Unmarshal(contentTypesXML, &types); err != nil {
		return err
	}

	// Initialize maps
	c.overrides = make(map[string]string)
	c.defaults = make(map[string]string)

	// Load overrides
	for _, o := range types.Override {
		c.overrides[strings.ToLower(o.PartName)] = o.ContentType
	}

	// Load defaults
	for _, d := range types.Default {
		c.defaults[strings.ToLower(d.Extension)] = d.ContentType
	}

	return nil
}

// PackageLoader loads a package from a file
type PackageLoader struct {
	pkgFile   interface{} // string (path) or io.Reader
	pkg       *OpcPackage
	reader    *zip.ReadCloser
	bufReader *bytes.Reader
}

// NewPackageLoader creates a new package loader
func NewPackageLoader(pkgFile interface{}, pkg *OpcPackage) *PackageLoader {
	return &PackageLoader{
		pkgFile: pkgFile,
		pkg:     pkg,
	}
}

// Load loads the package
func (p *PackageLoader) Load() error {
	var err error
	parts, xmlRels, err := p.loadParts()
	if err != nil {
		return err
	}

	// Load package relationships
	pkgURI := NewPackURI(PackageURI)
	err = p.pkg.Rels().LoadFromXML(pkgURI.BaseURI(), xmlRels[pkgURI], parts)
	if err != nil {
		return err
	}

	// Load relationships for each part
	for partname, part := range parts {
		if relXML, ok := xmlRels[partname]; ok {
			err = part.LoadRelsFromXML(relXML, parts)
			if err != nil {
				continue
			}
		}
	}

	return nil
}

// close closes any open readers
func (p *PackageLoader) close() {
	if p.reader != nil {
		p.reader.Close()
		p.reader = nil
	}
}

// loadParts loads all parts from the package
func (p *PackageLoader) loadParts() (map[PackURI]*Part, map[PackURI][]byte, error) {
	// Open the package
	err := p.openPackage()
	if err != nil {
		return nil, nil, err
	}
	defer p.close()

	// Read content types
	contentTypes, err := p.readContentTypes()
	if err != nil {
		return nil, nil, err
	}

	// Read all relationships
	xmlRels, err := p.readAllRelationships()
	if err != nil {
		return nil, nil, err
	}

	// Create parts
	parts := make(map[PackURI]*Part)
	for partname := range xmlRels {
		if partname.URI == PackageURI {
			continue
		}

		// Read part data
		data, err := p.readItem(partname.URI)
		if err != nil {
			log.Warnf("Failed to read part '%s': %v", partname.URI, err)
			continue
		}

		// Get content type
		contentType, err := contentTypes.GetContentType(partname)
		if err != nil {
			log.Warnf("Failed to get content type for part '%s': %v", partname.URI, err)
			continue
		}

		// Create part
		parts[partname] = NewPart(partname, contentType, p.pkg, data)
	}

	return parts, xmlRels, nil
}

// openPackage opens the package file
func (p *PackageLoader) openPackage() error {
	switch v := p.pkgFile.(type) {
	case string:
		// Open from file path
		reader, err := zip.OpenReader(v)
		if err != nil {
			return err
		}
		p.reader = reader
		return nil

	case io.Reader:
		// Read all data
		data, err := io.ReadAll(v)
		if err != nil {
			return err
		}

		// Create buffer reader
		p.bufReader = bytes.NewReader(data)

		// Open zip from reader
		reader, err := zip.NewReader(p.bufReader, int64(len(data)))
		if err != nil {
			return err
		}

		// For this case, we need a custom wrapper since we can't create a direct ReadCloser
		// We'll keep the reader and create a custom close method
		p.reader = &zip.ReadCloser{
			Reader: *reader,
		}

		return nil

	default:
		return fmt.Errorf("unsupported package file type: %T", p.pkgFile)
	}
}

// readContentTypes reads the content types from the package
func (p *PackageLoader) readContentTypes() (*ContentTypeMap, error) {
	// Read content types XML
	data, err := p.readItem(ContentTypesURI)
	if err != nil {
		return nil, err
	}

	// Parse content types
	contentTypes := NewContentTypeMap(nil, nil)
	err = contentTypes.FromXML(data)
	if err != nil {
		return nil, err
	}

	return contentTypes, nil
}

// readAllRelationships reads all relationships from the package
func (p *PackageLoader) readAllRelationships() (map[PackURI][]byte, error) {
	result := make(map[PackURI][]byte)
	visited := make(map[string]bool)

	// Start with package relationships
	pkgURI := NewPackURI(PackageURI)
	pkgRels, err := p.readRelationships(pkgURI)
	if err != nil {
		return nil, err
	}
	result[pkgURI] = pkgRels
	processTaskListStack := utils.NewStack[PackURI]()
	processTaskListStack.Push(pkgURI)
	// Process all relationships recursively
	for !processTaskListStack.IsEmpty() {
		partname := processTaskListStack.Pop()
		relXML, ok := result[partname]
		if !ok {
			continue
		}
		// Parse relationships
		var rels struct {
			XMLName xml.Name `xml:"Relationships"`
			Rel     []struct {
				Type       string `xml:"Type,attr"`
				Target     string `xml:"Target,attr"`
				TargetMode string `xml:"TargetMode,attr"`
			} `xml:"Relationship"`
		}

		if err := xml.Unmarshal(relXML, &rels); err != nil {
			// return nil, err
		}

		// Process each relationship
		baseURI := partname.BaseURI()
		for _, rel := range rels.Rel {
			if rel.TargetMode == "External" {
				continue
			}

			// Get target part name
			targetPartname := PackURI{}.FromRelRef(baseURI, rel.Target)
			if visited[targetPartname.URI] {
				continue
			}
			visited[targetPartname.URI] = true

			// Read target part relationships
			targetRels, err := p.readRelationships(targetPartname)
			if err != nil {
				// targetRels, err = p.readItem(targetPartname.URI)
				// Just log the error and continue
				log.Warnf("Failed to read relationships for part '%s': %v", targetPartname.URI, err)
				continue
			}

			result[targetPartname] = targetRels
			processTaskListStack.Push(targetPartname)
		}
	}

	return result, nil
}

// readRelationships reads the relationships for a part
func (p *PackageLoader) readRelationships(partname PackURI) ([]byte, error) {
	// Determine rels file path
	var relsPath string
	if partname.URI == PackageURI {
		relsPath = "_rels/.rels"
	} else {
		dir := path.Dir(partname.URI)
		filename := path.Base(partname.URI)
		relsPath = path.Join(dir, "_rels", filename+".rels")
	}

	// Read rels file
	data, err := p.readItem(relsPath)
	if err != nil {
		// If not found, return empty relationships
		return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"></Relationships>`), nil
	}

	return data, nil
}

// readItem reads an item from the package
func (p *PackageLoader) readItem(itemPath string) ([]byte, error) {
	// Remove leading slash
	if strings.HasPrefix(itemPath, "/") {
		itemPath = itemPath[1:]
	}

	// Find file in zip
	for _, f := range p.reader.File {
		if f.Name == itemPath {
			// Open file
			rc, err := f.Open()
			if err != nil {
				return nil, err
			}
			defer rc.Close()

			// Read file
			return io.ReadAll(rc)
		}
	}

	return nil, fmt.Errorf("item not found: %s", itemPath)
}

// OpcPackage represents a PPTX package
type OpcPackage struct {
	pkgFile string // Path to the package file
	rels    *Relationships
}

// NewOpcPackage creates a new package
func NewOpcPackage(pkgFile interface{}) *OpcPackage {
	return &OpcPackage{
		rels: NewRelationships(PackageURI),
	}
}

// Open opens a package from a file
func OpenOpcPackage(pkgFile interface{}) (*OpcPackage, error) {
	pkg := NewOpcPackage(pkgFile)

	// Load package
	loader := NewPackageLoader(pkgFile, pkg)
	err := loader.Load()
	if err != nil {
		return nil, err
	}

	return pkg, nil
}

// Rels returns the package relationships
func (p *OpcPackage) Rels() *Relationships {
	return p.rels
}

// MainDocumentPart returns the main document part
func (p *OpcPackage) MainDocumentPart() (*Part, error) {
	return p.PartRelatedBy(RelationshipTypeOfficeDocument)
}

// PartRelatedBy returns the part related by the given relationship type
func (p *OpcPackage) PartRelatedBy(relType RelationshipType) (*Part, error) {
	return p.rels.PartWithRelType(relType)
}

// IterParts iterates over all parts in the package
func (p *OpcPackage) IterParts() []*Part {
	visited := make(map[*Part]bool)
	var parts []*Part

	// Iterate over all relationships
	for _, rel := range p.rels.Values() {
		if rel.IsExternal() {
			continue
		}

		part, err := rel.TargetPart()
		if err != nil {
			continue
		}

		if visited[part] {
			continue
		}

		parts = append(parts, part)
		visited[part] = true

		// Add parts from this part's relationships
		for _, childRel := range part.Rels().Values() {
			if childRel.IsExternal() {
				continue
			}

			childPart, err := childRel.TargetPart()
			if err != nil {
				continue
			}

			if visited[childPart] {
				continue
			}

			parts = append(parts, childPart)
			visited[childPart] = true
		}
	}

	return parts
}

// Save saves the package to a file
func (p *OpcPackage) Save(pkgFile interface{}) error {
	// TODO: Implement package saving
	return nil
}
