package state

func handleFileChanged(currentState *State, payload map[string]interface{}) {
	filePath := getString(payload, "path", "file")
	action := getString(payload, "action")
	if filePath != "" {
		currentState.Current = "Editing " + filePath
		currentState.LastKnownAction = "Modified " + filePath
		if action == "do_not_repeat" {
			if !containsString(currentState.DoNotRepeat, filePath) {
				currentState.DoNotRepeat = append(currentState.DoNotRepeat, filePath)
			}
		}
	}
}

func handleCommandExecuted(currentState *State, payload map[string]interface{}) {
	cmd := getString(payload, "command")
	if cmd != "" {
		currentState.Current = "Executed: " + cmd
		currentState.LastKnownAction = "Executed command"
		currentState.LastCommand = cmd
		if exitCode, ok := getInt(payload, "exit_code"); ok {
			if exitCode == 0 {
				currentState.LastCommandResult = "SUCCESS"
			} else {
				currentState.LastCommandResult = "FAILED"
			}
		} else {
			currentState.LastCommandResult = "EXECUTED"
		}
	}
	if next := getString(payload, "next_action"); next != "" {
		currentState.NextAction = next
	} else if exitCode, ok := getInt(payload, "exit_code"); ok && exitCode != 0 && cmd != "" {
		currentState.NextAction = "Investigate failed command: " + cmd
	}
}

func handleTestStarted(currentState *State, payload map[string]interface{}) {
	suite := getString(payload, "suite", "test")
	if suite != "" {
		currentState.Current = "Running tests: " + suite
		currentState.LastKnownAction = "Ran tests"
		currentState.LastCommand = "Test suite: " + suite
		currentState.LastCommandResult = "PENDING"
	}
}

func handleTestPassed(currentState *State, payload map[string]interface{}) {
	suite := getString(payload, "suite", "test")
	if suite != "" {
		currentState.Current = "Tests passed: " + suite
		currentState.LastKnownAction = "Passed tests"
		currentState.LastCommand = "Test suite: " + suite
		currentState.LastCommandResult = "PASSED"
	}
	if next := getString(payload, "next_action"); next != "" {
		currentState.NextAction = next
	}
}

func handleTestFailed(currentState *State, payload map[string]interface{}) {
	suite := getString(payload, "suite", "test")
	if suite != "" {
		currentState.Current = "Test failed: " + suite
		currentState.NextAction = "Fix failing tests: " + suite
		currentState.LastKnownAction = "Failed tests"
		currentState.LastCommand = "Test suite: " + suite
		currentState.LastCommandResult = "FAILED"
	}
	if errStr := getString(payload, "error"); errStr != "" {
		currentState.NextAction = "Fix failing test: " + errStr
		if currentState.LastCommandResult == "FAILED" {
			currentState.LastCommandResult = "FAILED: " + errStr
		}
	}
}
