package ssaproject

var SSAProjectExports = map[string]interface{}{
	"GetSSAProjectByName": LoadSSAProjectBuilderByName,
	"GetSSAProjectByID":   LoadSSAProjectBuilderByID,
	"NewSSAProject":       NewSSAProjectBuilder,
}
