package ssaproject

var Exports = map[string]interface{}{
	"GetSSAProjectByNameAndURL":            LoadSSAProjectByNameAndURL,
	"GetSSAProjectByNameAndURLForBindMode": LoadSSAProjectByNameAndURLForBindMode,
	"GetSSAProjectByID":                    LoadSSAProjectByID,
	"NewSSAProject":                        NewSSAProject,
}
