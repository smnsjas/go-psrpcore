# Host Callback Handling

This package implements PowerShell host callback handling for interactive PSRP sessions.

## Overview

PowerShell scripts can interact with the user through the host interface (prompts, credentials, progress reporting, etc.). When PowerShell needs user interaction, it sends host callback messages to the client, which must respond appropriately.

The PSRP protocol defines two types of host callback messages:

- **RUNSPACEPOOL_HOST_CALL / RUNSPACEPOOL_HOST_RESPONSE**: Host callbacks at the runspace pool level
- **PIPELINE_HOST_CALL / PIPELINE_HOST_RESPONSE**: Host callbacks during pipeline execution

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                  PowerShell Server                      │
│  ┌────────────────────────────────────────────────┐     │
│  │  Script needs user input                      │     │
│  │  (e.g., Read-Host, Get-Credential)            │     │
│  └───────────────────┬────────────────────────────┘     │
└────────────────────┼─────────────────────────────────────┘
                     │
                     │ RUNSPACEPOOL_HOST_CALL
                     │ (MethodID, Parameters)
                     ▼
┌─────────────────────────────────────────────────────────┐
│                    Go Client                            │
│  ┌───────────────────────────────────────────────┐      │
│  │  CallbackHandler.HandleCall()                │      │
│  │  • Decodes RemoteHostCall                    │      │
│  │  • Dispatches to Host.UI() method            │      │
│  │  • Encodes RemoteHostResponse                │      │
│  └───────────────────┬───────────────────────────┘      │
└────────────────────┼─────────────────────────────────────┘
                     │
                     │ RUNSPACEPOOL_HOST_RESPONSE
                     │ (CallID, ReturnValue)
                     ▼
┌─────────────────────────────────────────────────────────┐
│                  PowerShell Server                      │
│  Script continues execution with response               │
└─────────────────────────────────────────────────────────┘
```

## Host Method IDs

The following method IDs map to `PSHostUserInterface` methods:

| Method ID | Method Name            | Parameters                                                | Return Value          |
|-----------|------------------------|-----------------------------------------------------------|-----------------------|
| 1         | Read                   | None                                                      | KeyInfo               |
| 2         | ReadLine               | None                                                      | String                |
| 3         | WriteErrorLine         | String (message)                                          | None                  |
| 4         | Write                  | String (text)                                             | None                  |
| 5         | WriteDebugLine         | String (message)                                          | None                  |
| 6         | WriteVerboseLine       | String (message)                                          | None                  |
| 7         | WriteWarningLine       | String (message)                                          | None                  |
| 8         | WriteInformation       | String (message)                                          | None                  |
| 9         | Prompt                 | String (caption), String (message), []FieldDescription    | Map[String]Interface  |
| 10        | PromptForCredential    | String (caption), String (message), String (user), String (target) | PSCredential |
| 11        | PromptForChoice        | String (caption), String (message), []ChoiceDescription, Int (default) | Int (selected) |
| 12        | PromptForPassword      | String (caption), String (message)                        | SecureString          |

## Usage

### Default (Non-Interactive) Host

For non-interactive scenarios, use the `NullHost`:

```go
import (
    "github.com/jasonmfehr/go-psrp/runspace"
    "github.com/jasonmfehr/go-psrp/host"
)

// Create pool with default NullHost
pool := runspace.New(transport, poolID)

// NullHost will respond to all callbacks with safe defaults:
// - ReadLine returns empty string
// - Write* methods are no-ops
// - Prompts return default values
```

### Custom Interactive Host

Implement the `Host` and `HostUI` interfaces for interactive sessions:

```go
package main

import (
    "bufio"
    "fmt"
    "os"

    "github.com/jasonmfehr/go-psrp/host"
    "github.com/jasonmfehr/go-psrp/objects"
)

// InteractiveHost implements host.Host for terminal interaction
type InteractiveHost struct {
    scanner *bufio.Scanner
}

func NewInteractiveHost() *InteractiveHost {
    return &InteractiveHost{
        scanner: bufio.NewScanner(os.Stdin),
    }
}

func (h *InteractiveHost) GetName() string           { return "go-psrp-interactive" }
func (h *InteractiveHost) GetVersion() host.Version  { return host.Version{Major: 1, Minor: 0} }
func (h *InteractiveHost) GetInstanceId() string     { return "custom-host-id" }
func (h *InteractiveHost) GetCurrentCulture() string { return "en-US" }
func (h *InteractiveHost) GetCurrentUICulture() string { return "en-US" }
func (h *InteractiveHost) UI() host.HostUI           { return &InteractiveHostUI{scanner: h.scanner} }

// InteractiveHostUI implements host.HostUI for terminal interaction
type InteractiveHostUI struct {
    scanner *bufio.Scanner
}

func (ui *InteractiveHostUI) ReadLine() (string, error) {
    fmt.Print("PS> ")
    if ui.scanner.Scan() {
        return ui.scanner.Text(), nil
    }
    return "", ui.scanner.Err()
}

func (ui *InteractiveHostUI) ReadLineAsSecureString() (*objects.SecureString, error) {
    // In production, use terminal.ReadPassword for secure input
    fmt.Print("Password: ")
    if ui.scanner.Scan() {
        return objects.NewSecureString(ui.scanner.Text()), nil
    }
    return nil, ui.scanner.Err()
}

func (ui *InteractiveHostUI) Write(text string) {
    fmt.Print(text)
}

func (ui *InteractiveHostUI) WriteLine(text string) {
    fmt.Println(text)
}

func (ui *InteractiveHostUI) WriteErrorLine(text string) {
    fmt.Fprintf(os.Stderr, "ERROR: %s\n", text)
}

func (ui *InteractiveHostUI) WriteDebugLine(text string) {
    fmt.Printf("DEBUG: %s\n", text)
}

func (ui *InteractiveHostUI) WriteVerboseLine(text string) {
    fmt.Printf("VERBOSE: %s\n", text)
}

func (ui *InteractiveHostUI) WriteWarningLine(text string) {
    fmt.Printf("WARNING: %s\n", text)
}

func (ui *InteractiveHostUI) WriteProgress(sourceId int64, record *objects.ProgressRecord) {
    fmt.Printf("Progress: %d%%\n", record.PercentComplete)
}

func (ui *InteractiveHostUI) Prompt(caption, message string, descriptions []host.FieldDescription) (map[string]interface{}, error) {
    fmt.Println(caption)
    fmt.Println(message)

    results := make(map[string]interface{})
    for _, desc := range descriptions {
        fmt.Printf("%s: ", desc.Label)
        if ui.scanner.Scan() {
            results[desc.Name] = ui.scanner.Text()
        }
    }
    return results, nil
}

func (ui *InteractiveHostUI) PromptForCredential(caption, message, userName, targetName string, allowedCredentialTypes host.CredentialTypes, options host.CredentialUIOptions) (*objects.PSCredential, error) {
    fmt.Println(caption)
    fmt.Println(message)

    var user string
    if userName != "" {
        user = userName
    } else {
        fmt.Print("Username: ")
        if ui.scanner.Scan() {
            user = ui.scanner.Text()
        }
    }

    // In production, use terminal.ReadPassword
    fmt.Print("Password: ")
    var password string
    if ui.scanner.Scan() {
        password = ui.scanner.Text()
    }

    return objects.NewPSCredential(user, objects.NewSecureString(password)), nil
}

func (ui *InteractiveHostUI) PromptForChoice(caption, message string, choices []host.ChoiceDescription, defaultChoice int) (int, error) {
    fmt.Println(caption)
    fmt.Println(message)

    for i, choice := range choices {
        fmt.Printf("[%d] %s\n", i, choice.Label)
    }

    fmt.Printf("Choice [%d]: ", defaultChoice)
    if ui.scanner.Scan() {
        text := ui.scanner.Text()
        if text == "" {
            return defaultChoice, nil
        }

        var choice int
        if _, err := fmt.Sscanf(text, "%d", &choice); err != nil {
            return defaultChoice, err
        }
        return choice, nil
    }

    return defaultChoice, ui.scanner.Err()
}

// Use the interactive host
func main() {
    pool := runspace.New(transport, poolID)
    pool.SetHost(NewInteractiveHost())

    ctx := context.Background()
    if err := pool.Open(ctx); err != nil {
        log.Fatal(err)
    }
    defer pool.Close(ctx)

    // Now when PowerShell scripts call Read-Host or Get-Credential,
    // they will interact with the terminal
}
```

## Message Flow Example

### PowerShell Script with Read-Host

```powershell
$name = Read-Host "Enter your name"
Write-Host "Hello, $name"
```

### PSRP Message Exchange

```
Client                                   Server
  │                                         │
  ├─── CREATE_PIPELINE ───────────────────>│
  │    (script: Read-Host "Enter...")      │
  │                                         │
  │<─── RUNSPACEPOOL_HOST_CALL ────────────┤
  │    CallID: 1                            │
  │    MethodID: 2 (ReadLine)              │
  │    Parameters: []                       │
  │                                         │
  │    [Client calls Host.UI().ReadLine()]  │
  │    [User types: "Alice"]                │
  │                                         │
  ├─── RUNSPACEPOOL_HOST_RESPONSE ───────>│
  │    CallID: 1                            │
  │    ExceptionRaised: false               │
  │    ReturnValue: "Alice"                 │
  │                                         │
  │<─── PIPELINE_OUTPUT ────────────────────┤
  │    Data: "Hello, Alice"                 │
  │                                         │
  │<─── PIPELINE_STATE ─────────────────────┤
  │    State: Completed                     │
```

## CLIXML Encoding

### RemoteHostCall Structure

```xml
<Obj RefId="0">
  <TN RefId="0">
    <T>Microsoft.PowerShell.Remoting.Internal.RemoteHostCall</T>
  </TN>
  <MS>
    <I64 N="ci">1</I64>              <!-- CallID -->
    <I32 N="mi">2</I32>              <!-- MethodID (ReadLine) -->
    <LST N="mp"/>                    <!-- Empty parameters -->
  </MS>
</Obj>
```

### RemoteHostResponse Structure

```xml
<Obj RefId="0">
  <TN RefId="0">
    <T>Microsoft.PowerShell.Remoting.Internal.RemoteHostResponse</T>
  </TN>
  <MS>
    <I64 N="ci">1</I64>              <!-- CallID -->
    <B N="er">false</B>              <!-- ExceptionRaised -->
    <S N="rv">Alice</S>              <!-- ReturnValue -->
  </MS>
</Obj>
```

## Testing

Run host callback tests:

```bash
go test ./host/... -v
```

Run integration test with runspace pool:

```bash
go test ./runspace/... -v -run TestHostCallback
```

## Common PowerShell Commands That Trigger Callbacks

- `Read-Host "prompt"` → ReadLine
- `Get-Credential` → PromptForCredential
- `Write-Error "message"` → WriteErrorLine
- `Write-Verbose "message"` → WriteVerboseLine
- `Write-Warning "message"` → WriteWarningLine
- `Write-Debug "message"` → WriteDebugLine
- `$Host.UI.PromptForChoice(...)` → PromptForChoice
- `Write-Progress ...` → WriteProgress

## Reference

- **MS-PSRP Section 2.2.3.17**: Host Call Messages
- **MS-PSRP Section 3.2.2**: RunspacePool State Machine
- **psrpcore**: [host_method.py](https://github.com/jborean93/psrpcore) (Python reference implementation)
