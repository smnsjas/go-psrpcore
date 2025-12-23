#!/usr/bin/env python3
"""Test if pypsrp can connect to PowerShell 7.5 on macOS via SSH"""

import subprocess
import time

# Start PowerShell in SSHServerMode
ps_process = subprocess.Popen(
    ['pwsh', '-SSHServerMode'],
    stdin=subprocess.PIPE,
    stdout=subprocess.PIPE,
    stderr=subprocess.PIPE
)

time.sleep(1)  # Let it initialize

try:
    from pypsrp.client import Client
    from pypsrp.powershell import PowerShell, RunspacePool
    from pypsrp.wsman import WSMan
    
    # Create client using the subprocess pipes
    # Note: pypsrp typically uses WinRM/WSMAN, not stdio
    # For SSHServerMode, we need a different approach
    
    print("pypsrp imported successfully")
    print(f"PowerShell process started: PID {ps_process.pid}")
    
    # Try to send a basic message
    # This is just to verify pypsrp can serialize messages
    ps = PowerShell(RunspacePool())
    ps.add_command("Get-Date")
    
    print("PowerShell command created successfully")
    print("Note: pypsrp is designed for WinRM, not SSHServerMode stdio")
    print("We would need a custom transport layer to use pypsrp with SSHServerMode")
    
finally:
    ps_process.terminate()
    ps_process.wait()
    print("PowerShell process terminated")
