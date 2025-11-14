package ssaproject

var Exports = map[string]interface{}{
	"GetSSAProjectByNameAndURL": LoadSSAProjectByNameAndURL,
	"GetSSAProjectByID":         LoadSSAProjectByID,
	"NewSSAProject":             NewSSAProject,
}
