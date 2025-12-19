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
	"github.com/smnsjas/go-psrpcore/runspace"
)

// ProcessTransport adapts a child process to io.ReadWriter
type ProcessTransport struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
}

func (p *ProcessTransport) Read(b []byte) (int, error) {
	n, err := p.stdout.Read(b)
	if n > 0 {
		// Trace logs are managed by runspace.go for fragments
	}
	return n, err
}

func (p *ProcessTransport) Write(b []byte) (int, error) {
	return p.stdin.Write(b)
}

func (p *ProcessTransport) Close() error {
	_ = p.stdin.Close()
	_ = p.stdout.Close()
	return p.cmd.Wait()
}

func startProcessTransport(command string, args ...string) (*ProcessTransport, error) {
	cmd := exec.Command(command, args...)
	cmd.Stderr = os.Stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("failed to get stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start process: %w", err)
	}

	return &ProcessTransport{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}, nil
}

func main() {
	// Trap Ctrl+C for clean shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	log.Println("Starting pwsh -ServerMode PSRP process...")
	// ServerMode PSRP allows talking to PowerShell via raw fragments on stdin/stdout
	transport, err := startProcessTransport("pwsh", "-ServerMode", "PSRP", "-NoLogo", "-NoProfile")
	if err != nil {
		log.Fatalf("Error starting transport: %v", err)
	}
	defer transport.Close()

	log.Println("Transport started. Initializing RunspacePool...")
	pool := runspace.New(transport, uuid.New())

	// Set a minimal host to avoid nil pointer dereferences
	pool.SetHost(host.NewNullHost())

	// Open the pool (connect/handshake)
	log.Println("Opening RunspacePool...")
	if err := pool.Open(ctx); err != nil {
		log.Fatalf("RunspacePool Open failed: %v", err)
	}

	log.Println("RunspacePool Opened Successfully!")

	// Execute a command
	log.Println("Executing 'Get-Date' via pipeline...")
	pl, err := pool.CreatePipeline("Get-Date")
	if err != nil {
		log.Fatalf("CreatePipeline failed: %v", err)
	}

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
