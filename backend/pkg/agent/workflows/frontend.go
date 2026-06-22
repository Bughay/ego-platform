package workflows

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/Bughay/egolifter/pkg/agent/agent"
	"github.com/Bughay/egolifter/pkg/agent/helper"
	"github.com/Bughay/egolifter/pkg/agent/prompts"
	tools "github.com/Bughay/egolifter/pkg/agent/tools/frontend_executer"
)

const (
	planningModel     = "grok-4.3"
	executeModel      = "grok-4.3"
	planningTokens    = 100000
	executeTokens     = 100000
	executeOnlyTokens = 250000
	planningTemp      = 0.2
	planningThinking  = true // reasoning mode for the research/plan phases; flip to disable
)

// runPlanning runs one planning-phase completion (the research or the plan step)
// through the provider-neutral LLM layer with the fixed planning model,
// temperature, and thinking settings. It centralizes the NewLLM error handling so
// the call sites stay a single line.
func runPlanning(systemPrompt, userPrompt string) (string, error) {
	llm, err := agent.NewLLM(agent.DeepSeek, agent.LLMParameters{
		Model:        planningModel,
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Temperature:  planningTemp,
		MaxTokens:    planningTokens,
		Thinking:     planningThinking,
	})
	if err != nil {
		return "", err
	}
	return llm.Complete(context.Background())
}

// ensureFrontendFiles creates index.html, styles.css, script.js, and plan.md
// with minimal stubs if they do not already exist in the working directory.
func ensureFrontendFiles() error {
	files := []struct {
		path    string
		content string
	}{
		{"index.html", "<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n  <meta charset=\"UTF-8\">\n  <meta name=\"viewport\" content=\"width=device-width, initial-scale=1.0\">\n  <title>App</title>\n  <link rel=\"stylesheet\" href=\"styles.css\">\n</head>\n<body>\n\n  <script src=\"script.js\"></script>\n</body>\n</html>\n"},
		{"styles.css", "/* styles */\n"},
		{"script.js", "// scripts\n"},
		{"plan.md", ""},
	}
	for _, f := range files {
		if err := helper.EnsureFile(f.path, f.content); err != nil {
			return err
		}
	}
	return nil
}

func VanillaFrontEnd() error {
	if err := ensureFrontendFiles(); err != nil {
		return err
	}

	UMessage, err := helper.Input("Welcome to Vanilla FrontEnd Agent\nPlease write your user request: ")
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	researchFiles, err := helper.ViewFiles([]string{"index.html", "styles.css", "script.js"})
	if err != nil {
		return fmt.Errorf("view files: %w", err)
	}

	slog.Info("researching", "model", planningModel)
	research, err := runPlanning(prompts.ProjectManager, UMessage+"\n\nHere are the files:\n\n"+researchFiles)
	if err != nil {
		return fmt.Errorf("research phase: %w", err)
	}
	fmt.Println(research)
	if _, err := helper.WriteToFile("plan.md", research); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}

	slog.Info("planning", "model", planningModel)
	plan, err := runPlanning(prompts.Teamlead, research+"\n\nHere are the files:\n\n"+researchFiles)
	if err != nil {
		return fmt.Errorf("planning phase: %w", err)
	}
	fmt.Println(plan)
	if _, err := helper.AppendToFile("plan.md", "\n\n###Here is the step by step plan\n\n"+plan); err != nil {
		return fmt.Errorf("append plan: %w", err)
	}

	return runExecuteAgent(planningModel, UMessage, executeTokens)
}

func VanillaFrontPlan() error {
	if err := ensureFrontendFiles(); err != nil {
		return err
	}

	UMessage, err := helper.Input("Welcome to Vanilla FrontEnd Agent\nPlease write your user request: ")
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	researchFiles, err := helper.ViewFiles([]string{"index.html", "styles.css", "script.js"})
	if err != nil {
		return fmt.Errorf("view files: %w", err)
	}

	slog.Info("researching", "model", planningModel)
	research, err := runPlanning(prompts.ProjectManager, UMessage+"\n\nHere are the files:\n\n"+researchFiles)
	if err != nil {
		return fmt.Errorf("research phase: %w", err)
	}
	fmt.Println(research)
	if _, err := helper.WriteToFile("plan.md", research); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}

	slog.Info("planning", "model", planningModel)
	plan, err := runPlanning(prompts.Teamlead, research+"\n\nHere are the files:\n\n"+researchFiles)
	if err != nil {
		return fmt.Errorf("planning phase: %w", err)
	}
	fmt.Println(plan)
	if _, err := helper.AppendToFile("plan.md", "\n\n###Here is the step by step plan\n\n"+plan); err != nil {
		return fmt.Errorf("append plan: %w", err)
	}

	return nil
}

func VanillaFrontExecute() error {
	if err := ensureFrontendFiles(); err != nil {
		return err
	}
	slog.Info("execute-only mode", "model", executeModel, "note", "using flash model for speed")
	return runExecuteAgent(executeModel, "I have prepared the plan.md file with all the instructions.", executeOnlyTokens)
}

func runExecuteAgent(model, userPrompt string, maxTokens int) error {
	a, err := agent.NewAgent(agent.DeepSeek, agent.AgentParameters{
		Model:        model,
		SystemPrompt: prompts.ExecuteAgent,
		UserPrompt:   userPrompt,
		Registry:     tools.FileFunctions(),
		SchemaData:   tools.SchemaJSON,
		MaxTokens:    maxTokens,
	})
	if err != nil {
		return fmt.Errorf("build execute agent: %w", err)
	}
	result, err := a.Run(context.Background())
	if err != nil {
		return fmt.Errorf("agent run: %w", err)
	}
	fmt.Println("\n=== Agent finished ===")
	fmt.Println(result)
	return nil
}
