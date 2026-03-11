package main

import (
	"context"
	"fmt"
	"os"

	"github.com/grovetools/tend/pkg/app"
	"github.com/grovetools/tend/pkg/harness"
)

func main() {
	// A list of all E2E scenarios
	scenarios := []*harness.Scenario{
		// Basic Scenarios
		BasicScenario(),
		SkillsScenario(),
		NotebookSkillsScenario(),
		TreeScenario(),
		// Agentic workflows
		SkillsSearchScenario(),
		SkillsIntegrateScenario(),
		// TUI Scenarios
		TUIScenario(),
		TUIWorkspaceScenario(),
	}

	// Execute the custom tend application with our scenarios
	if err := app.Execute(context.Background(), scenarios); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
