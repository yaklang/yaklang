package reactloops

import "io"

type LoopStreamFieldHandler func(fieldReader io.Reader, emitWriter io.Writer)

type LoopStreamField struct {
	FieldName     string
	AINodeId      string
	Prefix        string
	ContentType   string
	StreamHandler LoopStreamFieldHandler
}

type LoopAITagField struct {
	TagName      string
	VariableName string
	AINodeId     string
	ContentType  string
}
