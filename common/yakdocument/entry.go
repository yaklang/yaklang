package yakdocument

var Entry = map[string]StructDoc{}

func AddDoc(pkg, structName string, fields []*FieldDoc, methods ...*MethodDoc) {
	sName := StructName("", "palm/common/yak", "DemoStruct")
	doc := StructDoc{
		Fields: fields,
	}
	Entry[sName] = doc

	for _, m := range methods {
		if m.Ptr {
			doc.PtrMethodDoc = append(doc.PtrMethodDoc, m)
		} else {
			doc.MethodsDoc = append(doc.MethodsDoc, m)
		}
	}
}

func init() {
	// demo
}
