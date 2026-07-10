# Copyright (c) 2026 Ming-Chih Lung(Taiwan Tainan)

<#
.SYNOPSIS
    Run a shell script on the LinkIt Smart 7688 Duo, non-interactively.

.DESCRIPTION
    Encodes the three things that repeatedly broke during provisioning:

      1. PowerShell strips embedded double quotes when passing arguments to a
         native .exe, so a remote command line containing  grep -E "a|b"
         reaches ash as a bare pipeline. Never inline a remote script; upload
         a .sh file and run it by path.

      2. Dropbear (OpenWrt's SSH server) has no sftp-server, so pscp must be
         forced onto the legacy SCP protocol with -scp.

      3. plink hangs on an uncached host key even when fed "y" on stdin.
         Pin the fingerprint with -hostkey instead.

    Shell scripts are normalised to LF before upload; CRLF is a syntax error
    in ash.

.EXAMPLE
    .\run-on-board.ps1 -Script .\01-sd-extroot\preflight.sh

.EXAMPLE
    .\run-on-board.ps1 -Command 'df -h /overlay'
#>
[CmdletBinding(DefaultParameterSetName = 'Script')]
param(
    [Parameter(ParameterSetName = 'Script', Mandatory)]
    [string] $Script,

    [Parameter(ParameterSetName = 'Command', Mandatory)]
    [string] $Command,

    [string] $BoardHost = '192.168.50.216',
    [string] $User      = 'root',

    # Pin the host key. Obtain a new one by running plink with -batch once:
    # it fails and prints the fingerprint.
    [string] $HostKey   = 'SHA256:pmSxVegIX1+q4Mkux0AB1hPZYylPT12RuMC0hAWLNa4',

    # Not defaulted on purpose - pass it, or set BOARD_PASSWORD in the env.
    [string] $Password  = $env:BOARD_PASSWORD,

    [int] $TimeoutSec = 600
)

$ErrorActionPreference = 'Stop'

if (-not $Password) {
    throw "No password. Pass -Password, or set `$env:BOARD_PASSWORD first."
}

$target = "$User@$BoardHost"

if ($PSCmdlet.ParameterSetName -eq 'Command') {
    # Safe only because we pass a single argument with no embedded quotes.
    # Anything non-trivial belongs in a .sh file - see note 1 above.
    & plink -ssh -batch -hostkey $HostKey -pw $Password $target $Command
    return
}

if (-not (Test-Path $Script)) { throw "Script not found: $Script" }

# ash chokes on CRLF. Normalise a temp copy; never mutate the source file.
$src  = (Resolve-Path $Script).Path
$name = Split-Path $src -Leaf
$tmp  = Join-Path ([IO.Path]::GetTempPath()) "board_$name"
$text = [IO.File]::ReadAllText($src) -replace "`r`n", "`n"
[IO.File]::WriteAllText($tmp, $text, (New-Object Text.UTF8Encoding $false))

Write-Verbose "uploading $name -> /tmp/$name"
& pscp -scp -batch -hostkey $HostKey -pw $Password $tmp "${target}:/tmp/$name" | Out-Null
if ($LASTEXITCODE -ne 0) { throw "pscp failed with exit code $LASTEXITCODE" }

Remove-Item $tmp -Force

& plink -ssh -batch -hostkey $HostKey -pw $Password $target "sh /tmp/$name"
