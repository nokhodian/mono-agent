package system

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// ExecuteCommandNode runs a shell command and captures output.
// Type: "system.execute_command"
type ExecuteCommandNode struct{}

func (n *ExecuteCommandNode) Type() string { return "system.execute_command" }

func (n *ExecuteCommandNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	command, _ := config["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("system.execute_command: 'command' is required")
	}

	var args []string
	if rawArgs, ok := config["args"].([]interface{}); ok {
		for _, a := range rawArgs {
			args = append(args, fmt.Sprintf("%v", a))
		}
	}

	workingDir, _ := config["working_dir"].(string)

	timeoutSecs := 30
	if v, ok := config["timeout_seconds"].(float64); ok {
		timeoutSecs = int(v)
	}

	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSecs)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, command, args...)

	if workingDir != "" {
		cmd.Dir = workingDir
	}

	// Extra environment variables
	if envMap, ok := config["env"].(map[string]interface{}); ok {
		for k, v := range envMap {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%v", k, v))
		}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	exitCode := 0
	runErr := cmd.Run()
	if runErr != nil {
		if exitErr, ok := runErr.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	resultItem := workflow.NewItem(map[string]interface{}{
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
		"exit_code": exitCode,
	})

	var outputs []workflow.NodeOutput
	outputs = append(outputs, workflow.NodeOutput{Handle: "main", Items: []workflow.Item{resultItem}})
	if exitCode != 0 {
		outputs = append(outputs, workflow.NodeOutput{Handle: "error", Items: []workflow.Item{resultItem}})
	}
	return outputs, nil
}
