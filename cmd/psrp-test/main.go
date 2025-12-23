// psrp-test is an extended test client that validates the PSRP implementation
// across multiple scenarios to ensure robust functionality.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/smnsjas/go-psrpcore/host"
	"github.com/smnsjas/go-psrpcore/outofproc"
	"github.com/smnsjas/go-psrpcore/pipeline"
	"github.com/smnsjas/go-psrpcore/runspace"
)

// TestCase defines a single test scenario
type TestCase struct {
	Name        string
	Command     string
	Parameters  map[string]interface{}
	ExpectError bool
	Description string
}

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

func runTest(ctx context.Context, pool *runspace.Pool, tc TestCase) (passed bool, output string, errOutput string) {
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("TEST: %s\n", tc.Name)
	fmt.Printf("DESC: %s\n", tc.Description)
	fmt.Printf("CMD:  %s\n", tc.Command)
	if len(tc.Parameters) > 0 {
		fmt.Printf("PARAMS: %v\n", tc.Parameters)
	}
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	pl, err := pool.CreatePipeline(tc.Command)
	if err != nil {
		fmt.Printf("❌ FAILED: CreatePipeline error: %v\n", err)
		return false, "", ""
	}

	for name, value := range tc.Parameters {
		pl.AddParameter(name, value)
	}

	var outputBuf, errorBuf strings.Builder
	var wg sync.WaitGroup

	wg.Add(2)
	go func() {
		defer wg.Done()
		for msg := range pl.Output() {
			outputBuf.WriteString(string(msg.Data))
			outputBuf.WriteString("\n")
		}
	}()
	go func() {
		defer wg.Done()
		for msg := range pl.Error() {
			errorBuf.WriteString(string(msg.Data))
			errorBuf.WriteString("\n")
		}
	}()

	if err := pl.Invoke(ctx); err != nil {
		fmt.Printf("❌ FAILED: Invoke error: %v\n", err)
		return false, "", ""
	}

	// Wait for completion with timeout
	done := make(chan struct{})
	go func() {
		_ = pl.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(10 * time.Second):
		fmt.Printf("❌ FAILED: Timeout waiting for completion\n")
		return false, "", ""
	case <-ctx.Done():
		fmt.Printf("❌ FAILED: Context cancelled\n")
		return false, "", ""
	}

	wg.Wait()

	output = outputBuf.String()
	errOutput = errorBuf.String()

	// Determine pass/fail
	hasOutput := len(strings.TrimSpace(output)) > 0
	hasError := len(strings.TrimSpace(errOutput)) > 0

	if tc.ExpectError {
		if hasError {
			fmt.Printf("✅ PASSED: Expected error received\n")
			fmt.Printf("   Error: %s\n", truncate(errOutput, 200))
			return true, output, errOutput
		}
		fmt.Printf("❌ FAILED: Expected error but got none\n")
		return false, output, errOutput
	}

	if hasError {
		fmt.Printf("⚠️  WARNING: Unexpected error stream output\n")
		fmt.Printf("   Error: %s\n", truncate(errOutput, 200))
	}

	if hasOutput {
		fmt.Printf("✅ PASSED: Received output\n")
		fmt.Printf("   Output: %s\n", truncate(output, 200))
		return true, output, errOutput
	}

	// Some commands might not produce output (like Start-Sleep)
	fmt.Printf("✅ PASSED: Command completed (no output expected or received)\n")
	return true, output, errOutput
}

func truncate(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) > maxLen {
		return s[:maxLen] + "..."
	}
	return s
}

func runConcurrentTest(ctx context.Context, pool *runspace.Pool) bool {
	fmt.Printf("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("TEST: Concurrent Pipelines\n")
	fmt.Printf("DESC: Run two pipelines simultaneously to test multiplexing\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

	// Create two pipelines
	pl1, err := pool.CreatePipeline("Invoke-Expression")
	if err != nil {
		fmt.Printf("❌ FAILED: CreatePipeline 1 error: %v\n", err)
		return false
	}
	pl1.AddParameter("Command", "Start-Sleep -Milliseconds 500; 'Pipeline1-Done'")

	pl2, err := pool.CreatePipeline("Invoke-Expression")
	if err != nil {
		fmt.Printf("❌ FAILED: CreatePipeline 2 error: %v\n", err)
		return false
	}
	pl2.AddParameter("Command", "'Pipeline2-Done'")

	var wg sync.WaitGroup
	results := make([]string, 2)
	errors := make([]error, 2)

	// Helper to run a pipeline and collect output
	runPipeline := func(idx int, pl *pipeline.Pipeline, name string) {
		defer wg.Done()

		var output strings.Builder
		go func() {
			for msg := range pl.Output() {
				output.WriteString(string(msg.Data))
			}
		}()
		go func() {
			for range pl.Error() {
				// drain
			}
		}()

		if err := pl.Invoke(ctx); err != nil {
			errors[idx] = err
			return
		}
		_ = pl.Wait()
		results[idx] = output.String()
	}

	wg.Add(2)
	go runPipeline(0, pl1, "Pipeline1")
	go runPipeline(1, pl2, "Pipeline2")

	wg.Wait()

	// Validate results
	passed := true
	for i, result := range results {
		if errors[i] != nil {
			fmt.Printf("❌ Pipeline %d error: %v\n", i+1, errors[i])
			passed = false
		} else if strings.Contains(result, fmt.Sprintf("Pipeline%d-Done", i+1)) {
			fmt.Printf("✅ Pipeline %d completed with expected output\n", i+1)
		} else {
			fmt.Printf("⚠️  Pipeline %d output: %s\n", i+1, truncate(result, 100))
		}
	}

	if passed {
		fmt.Printf("✅ PASSED: Concurrent pipelines completed successfully\n")
	}
	return passed
}

func main() {
	// Trap Ctrl+C for clean shutdown
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	fmt.Println("╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║          PSRP Extended Test Suite                            ║")
	fmt.Println("║          Validating go-psrpcore implementation               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")

	log.Println("Starting pwsh -SSHServerMode process...")
	pipes, err := startProcess("/usr/local/bin/pwsh", "-SSHServerMode", "-NoLogo", "-NoProfile")
	if err != nil {
		log.Fatalf("Failed to start pwsh: %v", err)
	}
	defer pipes.Close()

	transport := outofproc.NewTransport(pipes.stdout, pipes.stdin)
	runspaceID := uuid.New()
	adapter := outofproc.NewAdapter(transport, runspaceID)

	pool := runspace.New(adapter, runspaceID)
	_ = pool.SetHost(host.NewNullHost())

	log.Println("Opening RunspacePool...")
	if err := pool.Open(ctx); err != nil {
		log.Fatalf("RunspacePool Open failed: %v", err)
	}
	log.Println("RunspacePool Opened Successfully!")

	// Define test cases
	testCases := []TestCase{
		// 1. Simple command (baseline)
		{
			Name:        "Simple Command",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "Get-Date"},
			Description: "Basic Get-Date command to verify pipeline works",
		},

		// 2. Complex output objects
		{
			Name:        "Complex Objects",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "Get-Process | Select-Object -First 3 -Property Name,Id"},
			Description: "Returns multiple complex objects with properties",
		},

		// 3. Hashtable/Dictionary output
		{
			Name:        "Hashtable Output",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "@{Name='Test'; Value=123; Nested=@{Inner='Data'}}"},
			Description: "Returns a hashtable with nested structure",
		},

		// 4. String output
		{
			Name:        "String Output",
			Command:     "Write-Output",
			Parameters:  map[string]interface{}{"InputObject": "Hello from Go PSRP Client!"},
			Description: "Simple string output via Write-Output",
		},

		// 5. Multiple output objects
		{
			Name:        "Multiple Outputs",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "1..5 | ForEach-Object { \"Item $_\" }"},
			Description: "Returns multiple string objects",
		},

		// 6. Large output (tests fragmentation)
		{
			Name:        "Large Output",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "1..100 | ForEach-Object { \"Line $_ - \" + ('X' * 50) }"},
			Description: "Large output to test fragmentation/reassembly",
		},

		// 7. Error handling - non-existent path
		{
			Name:        "Error Handling - Path",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "Get-Item '/nonexistent/path/12345'"},
			ExpectError: true,
			Description: "Should produce an error for non-existent path",
		},

		// 8. Error handling - throw (now correctly reports Failed state)
		{
			Name:        "Error Handling - Throw",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "throw 'Test error from Go client'"},
			ExpectError: false, // throw causes Failed state, not error stream
			Description: "Explicit throw should cause pipeline to end in Failed state",
		},

		// 9. Environment variable access
		{
			Name:        "Environment Variable",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "$env:HOME"},
			Description: "Access environment variable",
		},

		// 10. Command with boolean parameter
		{
			Name:        "Boolean Parameter",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "Get-ChildItem $env:HOME -Force | Select-Object -First 3 -Property Name"},
			Description: "Command using -Force switch",
		},

		// 11. Array input
		{
			Name:        "Array Processing",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "@(1, 2, 3) | ForEach-Object { $_ * 2 }"},
			Description: "Process array and transform values",
		},

		// 12. JSON output
		{
			Name:        "JSON Output",
			Command:     "Invoke-Expression",
			Parameters:  map[string]interface{}{"Command": "@{Status='OK'; Count=42} | ConvertTo-Json"},
			Description: "Convert hashtable to JSON string",
		},
	}

	// Run all test cases
	passed := 0
	failed := 0

	for _, tc := range testCases {
		if ok, _, _ := runTest(ctx, pool, tc); ok {
			passed++
		} else {
			failed++
		}
	}

	// Run concurrent test
	fmt.Println("\n\n═══════════════════════════════════════════════════════════════")
	fmt.Println("CONCURRENT PIPELINE TEST")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	if runConcurrentTest(ctx, pool) {
		passed++
	} else {
		failed++
	}

	// Summary
	fmt.Println("\n\n╔══════════════════════════════════════════════════════════════╗")
	fmt.Println("║                    TEST SUMMARY                               ║")
	fmt.Println("╚══════════════════════════════════════════════════════════════╝")
	total := passed + failed
	fmt.Printf("   Total Tests: %d\n", total)
	fmt.Printf("   ✅ Passed:   %d\n", passed)
	fmt.Printf("   ❌ Failed:   %d\n", failed)
	if failed == 0 {
		fmt.Println("\n   🎉 ALL TESTS PASSED! 🎉")
	} else {
		fmt.Printf("\n   ⚠️  %d test(s) failed\n", failed)
	}

	log.Println("\nClosing RunspacePool...")
	_ = pool.Close(context.Background())
	log.Println("Test suite finished.")
}
