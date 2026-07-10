# Ships .\mcp-server to the board's normal SD-backed /usr/local/bin/.
# Run .\build.ps1 first.
#
# No host-key-pinning parameter here: native ssh/scp verify host keys via
# known_hosts (there is no plink-style `-hostkey` flag for OpenSSH), and
# this board's key was already accepted into known_hosts earlier via
# `-o StrictHostKeyChecking=accept-new`. A `-HostKey` param that isn't
# wired into any ssh/scp flag would just be dead code that looks load-bearing.
#
# The board's old Dropbear also only offers hmac-sha1/hmac-md5 for the
# MAC, neither of which is in modern OpenSSH's default-enabled MAC list
# (confirmed via `ssh -G ... | Select-String macs`) - hence MACs=+hmac-sha1
# alongside the Kex/HostKey/Pubkey algorithm overrides below.
param(
    [string] $BoardHost = '192.168.50.216',
    [string] $KeyFile   = (Join-Path $PSScriptRoot '..\provisioning\lib\keys\board_rsa')
)

if (-not (Test-Path .\mcp-server)) { throw "mcp-server not found - run .\build.ps1 first" }
if (-not (Test-Path $KeyFile)) { throw "key file not found: $KeyFile" }

$sshOpts = @(
    '-i', $KeyFile,
    '-o', 'KexAlgorithms=+diffie-hellman-group14-sha1',
    '-o', 'HostKeyAlgorithms=+ssh-rsa',
    '-o', 'PubkeyAcceptedAlgorithms=+ssh-rsa',
    '-o', 'MACs=+hmac-sha1'
)

scp -O @sshOpts .\mcp-server "root@${BoardHost}:/tmp/mcp-server"
if ($LASTEXITCODE -ne 0) { throw "scp failed" }

& ssh @sshOpts "root@${BoardHost}" 'mkdir -p /usr/local/bin && mv /tmp/mcp-server /usr/local/bin/mcp-server && chmod 755 /usr/local/bin/mcp-server && echo deployed'
if ($LASTEXITCODE -ne 0) { throw "remote install failed" }
