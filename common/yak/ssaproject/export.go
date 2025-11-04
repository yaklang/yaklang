package ssaproject

var Exports = map[string]interface{}{
	"GetSSAProjectByName": LoadSSAProjectByName,
	"GetSSAProjectByID":   LoadSSAProjectByID,
	"NewSSAProject":       NewSSAProject,
}
