package aireact

import "context"

func (r *ReAct) invokeWriteYaklangCode(ctx context.Context, approach string) error {
	// start to write code:
	// * (optional)query code snippets
	// * write to file
	// * check syntax and lint
	// * (optional) run test cases
	// * (query document && modify to file)
	// * return

	satisfied := false
	iterationCount := 0
	maxIterations := 10
	currentCode := ""
	errorMessages := ""
	lastAction := ""

	userQuery := ""
	if r.config.memory != nil {
		userQuery = r.config.memory.Query
	}

	for !satisfied && iterationCount < maxIterations {
		prompt, err := r.promptManager.GenerateYaklangCodeActionLoop(
			userQuery,      // userQuery
			currentCode,    // currentCode
			errorMessages,  // errorMessages
			lastAction,     // lastAction
			iterationCount, // iterationCount
			maxIterations,  // maxIterations
		)
		if err != nil {
			return err
		}

		// TODO: Use the prompt to generate next action
		// For now, just increment iteration and break after first iteration
		_ = prompt // Avoid unused variable error
		iterationCount++
		satisfied = true // Temporary: break after one iteration
	}

	return nil
}
