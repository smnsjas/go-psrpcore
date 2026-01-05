// Package main is the PSRP client entrypoint.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/outofproc"
	"github.com/smnsjas/go-psrpcore/runspace"
)

// ProcessPipes holds the stdin/stdout of a child process.
type ProcessPipes struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p *ProcessPipes) Close() error {
	_ = p.stdin.Close()
	_ = p.stdout.Close()
	return p.cmd.Wait()
}

func startProcess(command string, args ...string) (*ProcessPipes, error) {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start process: %w", err)
	}

	return &ProcessPipes{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func main() {
	// Trap Ctrl+C for clean shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("Starting pwsh -SSHServerMode process...")
	// SSHServerMode is the correct flag for PowerShell 7+ PSRP over stdio
	// Find pwsh: use PWSH_PATH env var if set, otherwise search PATH
	pwshPath := os.Getenv("PWSH_PATH")
	if pwshPath == "" {
		var err error
		pwshPath, err = exec.LookPath("pwsh")
		if err != nil {
			log.Fatalf("PowerShell (pwsh) not found in PATH. Set PWSH_PATH environment variable: %v", err)
		}
	}
	pipes, err := startProcess(pwshPath, "-SSHServerMode", "-NoLogo", "-NoProfile")
	if err != nil {
		log.Fatalf("Failed to start pwsh: %v", err)
	}
	defer func() {
		if err := pipes.Close(); err != nil {
			log.Printf("Warning: failed to close pipes: %v", err)
		}
	}()

	// Create OutOfProcess transport
	// The transport wraps the stdin/stdout and handles the XML framing protocol
	transport := outofproc.NewTransport(pipes.stdout, pipes.stdin)

	// Create adapter that provides io.ReadWriter interface for the runspace.
	// The adapter uses NullGUID for session-level operations (SESSION_CAPABILITY,
	// INIT_RUNSPACEPOOL, etc.) which is correct per the OutOfProcess protocol.
	// The runspaceID passed here is used for tracking only.
	runspaceID := uuid.New()
	adapter := outofproc.NewAdapter(transport, runspaceID)

	log.Println("Transport started. Initializing RunspacePool...")
	pool := runspace.New(adapter, runspaceID)

	// Set a minimal host to avoid nil pointer dereferences
	_ = pool.SetHost(host.NewNullHost())

	// Open the pool (connect/handshake)
	log.Println("Opening RunspacePool...")
	if err := pool.Open(ctx); err != nil {
		log.Fatalf("RunspacePool Open failed: %v", err)
	}

	log.Println("RunspacePool Opened Successfully!")

	// Execute a command
	// Using Invoke-Expression like Ruby template does
	log.Println("Executing via pipeline with Invoke-Expression...")
	pl, err := pool.CreatePipeline("Invoke-Expression")
	if err != nil {
		log.Fatalf("CreatePipeline failed: %v", err)
	}

	// Add -Command parameter like Ruby template
	pl.AddParameter("Command", "Get-Date")

	// Start consuming output/error in background before invoking
	go func() {
		for msg := range pl.Output() {
			log.Printf("PIPELINE OUTPUT [%d bytes]: %s", len(msg.Data), string(msg.Data))
		}
	}()
	go func() {
		for msg := range pl.Error() {
			log.Printf("PIPELINE ERROR [%d bytes]: %s", len(msg.Data), string(msg.Data))
		}
	}()

	if err := pl.Invoke(ctx); err != nil {
		log.Fatalf("Invoke failed: %v", err)
	}

	// Wait for completion or timeout
	log.Println("Waiting for command completion...")
	select {
	case <-time.After(5 * time.Second):
		log.Println("Command timed out.")
	case <-ctx.Done():
		log.Println("Interrupted.")
	case <-func() chan struct{} {
		c := make(chan struct{})
		go func() {
			_ = pl.Wait()
			close(c)
		}()
		return c
	}():
		log.Println("Command finished.")
	}

	log.Println("Closing RunspacePool...")
	_ = pool.Close(context.Background())
	log.Println("Client finished.")
}
