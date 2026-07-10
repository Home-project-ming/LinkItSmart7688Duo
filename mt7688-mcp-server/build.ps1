# Cross-compiles the MCP server for the board (mipsel, soft-float,
# statically linked). $env: assignments write straight to the process
# environment block - unlike regular PowerShell variables they are NOT
# scoped by dot-sourcing vs. script invocation (no child process is
# spawned either way), so setting them unconditionally would leak
# GOOS/GOARCH/GOMIPS/CGO_ENABLED into the calling shell for the rest of
# the session. The try/finally below is what actually provides the
# isolation: it saves the prior values up front and restores them (or
# removes the var if it was previously unset) once the build finishes,
# regardless of success or failure.
$prevGOOS = $env:GOOS
$prevGOARCH = $env:GOARCH
$prevGOMIPS = $env:GOMIPS
$prevCGO_ENABLED = $env:CGO_ENABLED

function Restore-EnvVar {
    param([string]$Name, [string]$Value)
    if ($null -eq $Value) {
        Remove-Item -Path "Env:\$Name" -ErrorAction SilentlyContinue
    } else {
        Set-Item -Path "Env:\$Name" -Value $Value
    }
}

try {
    $env:GOOS = 'linux'
    $env:GOARCH = 'mipsle'
    $env:GOMIPS = 'softfloat'
    $env:CGO_ENABLED = '0'

    go build -ldflags="-s -w" -o mcp-server .
    if ($LASTEXITCODE -ne 0) { throw "build failed" }

    $size = (Get-Item .\mcp-server).Length
    Write-Output "built .\mcp-server ($size bytes)"
}
finally {
    Restore-EnvVar -Name 'GOOS' -Value $prevGOOS
    Restore-EnvVar -Name 'GOARCH' -Value $prevGOARCH
    Restore-EnvVar -Name 'GOMIPS' -Value $prevGOMIPS
    Restore-EnvVar -Name 'CGO_ENABLED' -Value $prevCGO_ENABLED
}
