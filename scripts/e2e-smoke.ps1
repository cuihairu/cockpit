param(
    [int]$Port = 19990,
    [switch]$KeepLogs
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RootDir = Split-Path -Parent $PSScriptRoot
$WorkDir = Join-Path ([System.IO.Path]::GetTempPath()) ("cockpit-e2e-{0}" -f $PID)
$ServerHost = "127.0.0.1"
$ServerUrl = "http://{0}:{1}" -f $ServerHost, $Port
$WSUrl = "ws://{0}:{1}/ws" -f $ServerHost, $Port
$AdminUser = "admin"
$AdminPass = "e2e-strong-pass-1"
$ServerProc = $null
$AgentProc = $null
$ExitCode = 0

function Write-Log {
    param([string]$Message)
    Write-Host "[e2e] $Message" -ForegroundColor Cyan
}

function Write-Err {
    param([string]$Message)
    Write-Host "[e2e:err] $Message" -ForegroundColor Red
}

function Require-Command {
    param([string]$Name)
    if (-not (Get-Command $Name -ErrorAction SilentlyContinue)) {
        throw "Missing required command: $Name"
    }
}

function Get-HttpStatus {
    param(
        [string]$Uri,
        [hashtable]$Headers = @{}
    )

    try {
        $response = Invoke-WebRequest -Uri $Uri -Headers $Headers -TimeoutSec 5
        return [int]$response.StatusCode
    } catch {
        if ($_.Exception.PSObject.Properties.Name -contains "Response" -and $null -ne $_.Exception.Response) {
            return [int]$_.Exception.Response.StatusCode
        }
        return -1
    }
}

function Wait-Until {
    param(
        [string]$Name,
        [scriptblock]$Condition,
        [int]$Attempts = 30,
        [int]$SleepSeconds = 1
    )

    for ($i = 1; $i -le $Attempts; $i++) {
        if (& $Condition) {
            return
        }
        Start-Sleep -Seconds $SleepSeconds
    }

    throw "$Name did not complete within $Attempts attempts"
}

function Show-LogTail {
    param([string]$Path)
    if (Test-Path -LiteralPath $Path) {
        Get-Content -LiteralPath $Path -Tail 60 | ForEach-Object { Write-Host $_ }
    }
}

function Write-Utf8NoBomFile {
    param(
        [string]$Path,
        [string]$Content
    )

    $normalized = $Content -replace "`r`n", "`n" -replace "`r", "`n"
    [System.IO.File]::WriteAllText($Path, $normalized, [System.Text.UTF8Encoding]::new($false))
}

function Cleanup {
    if ($null -ne $ServerProc -and -not $ServerProc.HasExited) {
        Stop-Process -Id $ServerProc.Id -Force -ErrorAction SilentlyContinue
    }
    if ($null -ne $AgentProc -and -not $AgentProc.HasExited) {
        Stop-Process -Id $AgentProc.Id -Force -ErrorAction SilentlyContinue
    }

    if ($KeepLogs) {
        Write-Log "Keeping logs at $WorkDir"
        return
    }

    if (Test-Path -LiteralPath $WorkDir) {
        Remove-Item -LiteralPath $WorkDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}

try {
    Require-Command go

    New-Item -ItemType Directory -Path (Join-Path $WorkDir "data") -Force | Out-Null
    New-Item -ItemType Directory -Path (Join-Path $WorkDir "bin") -Force | Out-Null

    $configPath = Join-Path $WorkDir "cockpit.yaml"
    $inventoryPath = Join-Path $WorkDir "inventory.yaml"
    $serverLog = Join-Path $WorkDir "server.log"
    $serverErrLog = Join-Path $WorkDir "server.err.log"
    $agentLog = Join-Path $WorkDir "agent.log"
    $agentErrLog = Join-Path $WorkDir "agent.err.log"
    $binDir = Join-Path $WorkDir "bin"
    $cockpitExe = Join-Path $binDir "cockpit.exe"
    $agentExe = Join-Path $binDir "cockpit-agent.exe"

    $configContent = @"
server:
  host: $ServerHost
  port: $Port
database:
  path: $WorkDir/data/cockpit.db
jwt:
  secret: e2e-jwt-secret
  expiration: 1h
"@
    Write-Utf8NoBomFile -Path $configPath -Content $configContent

    $inventoryContent = @"
version: "v1"
metadata:
  name: e2e-smoke
  description: End-to-end smoke inventory
regions:
  local:
    name: Local
    zones:
      smoke:
        name: Smoke Zone
        agents:
          inventory-agent:
            hostname: inventory-agent.local
            ip: 127.0.0.1
            capabilities:
              - docker
domains:
  example.com:
    id: example-com
    domain: example.com
    provider: manual
    agent: inventory-agent
    certificates:
      - id: example-com-cert
        domain: example.com
        provider: manual
        agent: inventory-agent
computeInstances:
  smoke-vm:
    name: Smoke VM
    type: vm
    agent: inventory-agent
    region: local
    zone: smoke
    cpu: 1
    memory: 512
    disk: 10
services:
  smoke-http:
    name: Smoke HTTP
    type: http
    agent: inventory-agent
    region: local
    zone: smoke
    url: https://example.com
gateways:
  smoke-router:
    name: Smoke Router
    type: openwrt
    agent: inventory-agent
    region: local
    zone: smoke
    ipv4: 127.0.0.1
storages:
  smoke-storage:
    name: Smoke Storage
    type: local
    agent: inventory-agent
    region: local
    zone: smoke
    path: /tmp
"@
    Write-Utf8NoBomFile -Path $inventoryPath -Content $inventoryContent

    Write-Log "Building binaries..."
    Push-Location $RootDir
    try {
        & go build -o "$binDir/" ./cmd/cockpit ./cmd/cockpit-agent
        if ($LASTEXITCODE -ne 0) {
            throw "go build failed"
        }
    } finally {
        Pop-Location
    }

    $env:ADMIN_USERNAME = $AdminUser
    $env:ADMIN_PASSWORD = $AdminPass

    Write-Log "Starting server on $ServerUrl..."
    $ServerProc = Start-Process -FilePath $cockpitExe `
        -ArgumentList @("server", "-config", $configPath) `
        -WorkingDirectory $RootDir `
        -WindowStyle Hidden `
        -RedirectStandardOutput $serverLog `
        -RedirectStandardError $serverErrLog `
        -PassThru

    Write-Log "Waiting for server health..."
    Wait-Until -Name "Server health" -Condition {
        try {
            $health = Invoke-RestMethod -Uri "$ServerUrl/health" -TimeoutSec 5
            if ($health.status -eq "ok") {
                Write-Log ("Server healthy: " + ($health | ConvertTo-Json -Compress))
                return $true
            }
        } catch {
        }
        return $false
    }

    Write-Log "Starting agent..."
    $AgentProc = Start-Process -FilePath $agentExe `
        -ArgumentList @("start", "-server", $WSUrl) `
        -WorkingDirectory $RootDir `
        -WindowStyle Hidden `
        -RedirectStandardOutput $agentLog `
        -RedirectStandardError $agentErrLog `
        -PassThru

    Write-Log "Waiting for agent API route..."
    Wait-Until -Name "Agent API route" -Attempts 15 -Condition {
        $status = Get-HttpStatus -Uri "$ServerUrl/api/agents"
        if ($status -eq 401) {
            Write-Log "API reachable (401 = auth required, route exists)"
            return $true
        }
        return $false
    }

    Write-Log "Logging in as $AdminUser..."
    $loginResponse = Invoke-RestMethod -Method Post `
        -Uri "$ServerUrl/api/auth/login" `
        -ContentType "application/json" `
        -Body (@{ username = $AdminUser; password = $AdminPass } | ConvertTo-Json -Compress) `
        -TimeoutSec 5

    if ([string]::IsNullOrWhiteSpace($loginResponse.token)) {
        throw "Login failed or no token in response: $($loginResponse | ConvertTo-Json -Compress)"
    }

    $headers = @{ Authorization = "Bearer $($loginResponse.token)" }
    Write-Log "Got JWT token"

    Write-Log "Verifying agent registration via /api/agents..."
    Wait-Until -Name "Agent registration" -Condition {
        try {
            $agents = Invoke-RestMethod -Uri "$ServerUrl/api/agents" -Headers $headers -TimeoutSec 5
            $count = ($agents | Measure-Object).Count
            if ($count -gt 0) {
                Write-Log "Agent online. Agents count: $count"
                return $true
            }
        } catch {
        }
        return $false
    }

    Write-Log "Syncing inventory..."
    Push-Location $RootDir
    try {
        & $cockpitExe sync -config $configPath -inventory $inventoryPath
        if ($LASTEXITCODE -ne 0) {
            throw "Inventory sync failed"
        }
    } finally {
        Pop-Location
    }
    Write-Log "Inventory sync done"

    foreach ($kind in @("compute-instances", "domains", "certificates", "services", "gateways", "storages")) {
        $response = Invoke-RestMethod -Uri "$ServerUrl/api/resources/$kind" -Headers $headers -TimeoutSec 5
        $total = [int]$response.total
        if ($total -lt 1) {
            throw "GET /api/resources/$kind returned no synced data: $($response | ConvertTo-Json -Compress)"
        }
        Write-Log "GET /api/resources/$kind -> 200 ($total item(s))"
    }

    Write-Log "All smoke checks passed"
} catch {
    $ExitCode = 1
    Write-Err $_.Exception.Message
    if (Test-Path -LiteralPath $serverLog) {
        Write-Err "--- server.log ---"
        Show-LogTail -Path $serverLog
    }
    if (Test-Path -LiteralPath $serverErrLog) {
        Write-Err "--- server.err.log ---"
        Show-LogTail -Path $serverErrLog
    }
    if (Test-Path -LiteralPath $agentLog) {
        Write-Err "--- agent.log ---"
        Show-LogTail -Path $agentLog
    }
    if (Test-Path -LiteralPath $agentErrLog) {
        Write-Err "--- agent.err.log ---"
        Show-LogTail -Path $agentErrLog
    }
} finally {
    Cleanup
}

exit $ExitCode
