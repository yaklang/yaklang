package aireact

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"strings"
)

func (r *ReAct) handleFreeValue(userInput string) error {
	if userInput == "" || strings.TrimSpace(userInput) == "" {
		return utils.Errorf("user input cannot be empty")
	}
	if r.config.debugEvent {
		log.Infof("Using free input: %s", userInput)
	}
	// Reset session state if needed
	r.config.mu.Lock()
	r.config.finished = false
	r.config.currentIteration = 0
	r.config.mu.Unlock()
	if r.config.debugEvent {
		log.Infof("Reset ReAct session for new input")
	}
	// Execute the main ReAct loop using the new schema-based approach
	if r.config.debugEvent {
		log.Infof("Executing main loop with user input: %s", userInput)
	}
	return r.executeMainLoop(userInput)
}
