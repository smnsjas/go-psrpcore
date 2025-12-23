# Enable PSRP tracing to see what PowerShell sends
$ErrorActionPreference = "Stop"

# Create a PowerShell object
$ps = [PowerShell]::Create()
$ps.AddCommand("Get-Date")

# Serialize to CLIXML
$serializer = [System.Management.Automation.PSSerializer]::Serialize($ps)
Write-Host "PowerShell CLIXML:"
Write-Host $serializer
