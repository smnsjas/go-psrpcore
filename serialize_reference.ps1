#!/usr/bin/env pwsh

# Use reflection to access internal ToPSObjectForRemoting method
$ps = [System.Management.Automation.PowerShell]::Create()
$ps.AddCommand("Get-Date")

# Get the internal method using reflection
$method = [System.Management.Automation.PowerShell].GetMethod(
    "ToPSObjectForRemoting",
    [System.Reflection.BindingFlags]::NonPublic -bor [System.Reflection.BindingFlags]::Instance
)

if ($method) {
    $psObj = $method.Invoke($ps, $null)
    $clixml = [System.Management.Automation.PSSerializer]::Serialize($psObj, 10)
    Write-Host "=== PowerShell Object CLIXML ===" 
    Write-Host $clixml
}
else {
    Write-Host "Could not find ToPSObjectForRemoting method"
}

# Try to get Command serialization
if ($ps.Commands.Commands.Count -gt 0) {
    $cmd = $ps.Commands.Commands[0]
    $cmdMethod = [System.Management.Automation.Runspaces.Command].GetMethod(
        "ToPSObjectForRemoting",
        [System.Reflection.BindingFlags]::NonPublic -bor [System.Reflection.BindingFlags]::Instance
    )
    
    if ($cmdMethod) {
        $cmdObj = $cmdMethod.Invoke($cmd, @([version]"2.3"))
        $cmdXml = [System.Management.Automation.PSSerializer]::Serialize($cmdObj, 10)
        Write-Host "`n=== Command CLIXML ==="
        Write-Host $cmdXml
    }
}
