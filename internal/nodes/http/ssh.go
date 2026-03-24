package httpnodes

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/monoes/monoes-agent/internal/workflow"
)

// SSHNode runs a remote shell command over SSH.
// Type: "http.ssh"
type SSHNode struct{}

func (n *SSHNode) Type() string { return "http.ssh" }

func (n *SSHNode) Execute(ctx context.Context, input workflow.NodeInput, config map[string]interface{}) ([]workflow.NodeOutput, error) {
	host, _ := config["host"].(string)
	if host == "" {
		return nil, fmt.Errorf("http.ssh: 'host' is required")
	}

	port := 22
	if v, ok := config["port"].(float64); ok {
		port = int(v)
	}

	username, _ := config["username"].(string)
	if username == "" {
		return nil, fmt.Errorf("http.ssh: 'username' is required")
	}

	command, _ := config["command"].(string)
	if command == "" {
		return nil, fmt.Errorf("http.ssh: 'command' is required")
	}

	timeoutSecs := 30
	if v, ok := config["timeout_seconds"].(float64); ok {
		timeoutSecs = int(v)
	}
	timeout := time.Duration(timeoutSecs) * time.Second

	// Build auth methods
	var authMethods []ssh.AuthMethod

	privateKeyPEM, _ := config["private_key"].(string)
	password, _ := config["password"].(string)

	if privateKeyPEM != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKeyPEM))
		if err != nil {
			return nil, fmt.Errorf("http.ssh: failed to parse private key: %w", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if len(authMethods) == 0 {
		return nil, fmt.Errorf("http.ssh: either 'password' or 'private_key' is required")
	}

	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         timeout,
	}

	addr := fmt.Sprintf("%s:%d", host, port)

	// Dial with context
	dialer := &net.Dialer{Timeout: timeout}
	netConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("http.ssh: dial failed: %w", err)
	}

	sshConn, chans, reqs, err := ssh.NewClientConn(netConn, addr, sshConfig)
	if err != nil {
		netConn.Close()
		return nil, fmt.Errorf("http.ssh: handshake failed: %w", err)
	}
	client := ssh.NewClient(sshConn, chans, reqs)
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("http.ssh: session failed: %w", err)
	}
	defer session.Close()

	var stdoutBuf, stderrBuf bytes.Buffer
	session.Stdout = &stdoutBuf
	session.Stderr = &stderrBuf

	exitCode := 0
	runErr := session.Run(command)
	if runErr != nil {
		if exitErr, ok := runErr.(*ssh.ExitError); ok {
			exitCode = exitErr.ExitStatus()
		} else {
			exitCode = -1
		}
	}

	resultItem := workflow.NewItem(map[string]interface{}{
		"stdout":    stdoutBuf.String(),
		"stderr":    stderrBuf.String(),
		"exit_code": exitCode,
	})

	handle := "main"
	if exitCode != 0 {
		handle = "error"
	}

	return []workflow.NodeOutput{{Handle: handle, Items: []workflow.Item{resultItem}}}, nil
}
