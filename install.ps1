# Cockpit Agent 一键安装（Windows PowerShell 5.1+ / pwsh）
#
# 从每日构建（nightly release，匿名可直链下载）拉取与本机架构匹配的
# cockpit-agent 并安装，可选注册 Windows 服务（开机自启）。
#
# 用法:
#   # 一键（自动检测架构，装到当前用户目录，无需管理员）:
#   irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1 | iex
#
#   # 带参数（注册 Windows 服务，需管理员 PowerShell）:
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/cuihairu/cockpit/main/install.ps1))) `
#       -WithService -ServerUrl "wss://cockpit.example.com/ws"
#
#   # 管道形态传参不便时用环境变量:
#   $env:COCKPIT_WITH_SERVICE='1'; $env:COCKPIT_SERVER='wss://...'; irm <url> | iex
#
# 幂等：重跑即升级（覆盖二进制；已注册服务则自动重启加载新版本）。
#requires -Version 5.1

param(
    [string]$ServerUrl = $env:COCKPIT_SERVER,
    [string]$AgentId = $env:COCKPIT_AGENT_ID,
    [string]$Secret = $env:COCKPIT_SECRET,
    [string]$Region = $env:COCKPIT_REGION,
    [string]$Zone = $env:COCKPIT_ZONE,
    [string]$Labels = $env:COCKPIT_LABELS,
    [string]$InstallDir = $env:COCKPIT_INSTALL_DIR,
    [switch]$WithService = ($env:COCKPIT_WITH_SERVICE -eq '1')
)

$ErrorActionPreference = 'Stop'
$Repo = if ($env:COCKPIT_REPO) { $env:COCKPIT_REPO } else { 'cuihairu/cockpit' }
$ReleaseTag = if ($env:COCKPIT_RELEASE) { $env:COCKPIT_RELEASE } else { 'nightly' }
$ServiceName = 'CockpitAgent'

function Test-IsAdmin {
    $id = [Security.Principal.WindowsIdentity]::GetCurrent()
    return (New-Object Security.Principal.WindowsPrincipal($id)).IsInRole(
        [Security.Principal.WindowsBuiltInRole]::Administrator)
}

# 纯逻辑函数（架构/资产名/URL/启动参数），便于脱离 Windows 环境校验
function Get-AgentArch {
    # 32 位 PowerShell 跑在 64 位系统上时 PROCESSOR_ARCHITECTURE 是 x86，真实架构在 W6432
    $pa = $env:PROCESSOR_ARCHITECTURE
    if ($env:PROCESSOR_ARCHITEW6432) { $pa = $env:PROCESSOR_ARCHITEW6432 }
    switch ($pa) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        'x86'   { throw "不支持的架构: 32 位 x86 —— nightly 未提供 windows-386 产物" }
        default { throw "不支持的架构: $pa —— 已支持: AMD64(x86_64)、ARM64" }
    }
}

function Get-AssetUrl {
    param([string]$Arch)
    return "https://github.com/$Repo/releases/download/$ReleaseTag/cockpit-agent-windows-$Arch-$ReleaseTag.zip"
}

function Quote-CmdArg {
    param([string]$v)
    # Windows 服务命令行引号规则：内嵌 " 转义为 \"（其前的 \ 按 MSVCRT 规则成对加倍）
    return '"' + ($v -replace '(\\*)"', '$1$1\"') + '"'
}

function Build-StartArgs {
    # 服务启动参数（与 cockpit-agent start 的 flag 面一一对应）
    if ([string]::IsNullOrEmpty($ServerUrl)) { throw "注册服务需要 -ServerUrl（或既有 $ServiceName 服务）" }
    $startArgs = @('start', '-server', (Quote-CmdArg $ServerUrl))
    if (-not [string]::IsNullOrEmpty($AgentId)) { $startArgs += @('-id', (Quote-CmdArg $AgentId)) }
    if (-not [string]::IsNullOrEmpty($Secret)) { $startArgs += @('-secret', (Quote-CmdArg $Secret)) }
    if (-not [string]::IsNullOrEmpty($Region)) { $startArgs += @('-region', (Quote-CmdArg $Region)) }
    if (-not [string]::IsNullOrEmpty($Zone)) { $startArgs += @('-zone', (Quote-CmdArg $Zone)) }
    if (-not [string]::IsNullOrEmpty($Labels)) { $startArgs += @('-labels', (Quote-CmdArg $Labels)) }
    return $startArgs
}

function Write-Step($msg) { Write-Host $msg -ForegroundColor Yellow }

try {
    Write-Host "Cockpit Agent 一键安装" -ForegroundColor Cyan
    Write-Host "======================" -ForegroundColor Cyan

    # WinPS 5.1 默认可能不启用 TLS1.2（GitHub 下载需要）
    if ([Net.ServicePointManager]::SecurityProtocol -band [Net.SecurityProtocolType]::Tls12 -eq 0) {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    }

    $isAdmin = Test-IsAdmin
    $arch = Get-AgentArch
    $assetUrl = Get-AssetUrl -Arch $arch

    # 安装目录：注册服务且具备管理员权限 → 系统目录；否则当前用户目录（免管理员）
    if ([string]::IsNullOrEmpty($InstallDir)) {
        if ($WithService -and $isAdmin) {
            $InstallDir = 'C:\Program Files\CockpitAgent'
        } else {
            $InstallDir = Join-Path $env:LOCALAPPDATA 'Programs\Cockpit\bin'
        }
    }

    Write-Host ""
    Write-Host "  架构: $arch"
    Write-Host "  安装目录: $InstallDir"
    Write-Host "  来源: $assetUrl"
    Write-Host ""

    # 下载前先停既有服务（Windows 下无法覆盖正在运行的 exe）
    $svc = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($svc -and $svc.Status -ne 'Stopped') {
        if (-not $isAdmin) {
            throw "检测到正在运行的 $ServiceName 服务，升级需以管理员身份重跑本脚本"
        }
        Write-Step "停止既有 $ServiceName 服务..."
        Stop-Service -Name $ServiceName -Force
    }

    Write-Step "下载 nightly 产物..."
    $tmpZip = Join-Path ([IO.Path]::GetTempPath()) "cockpit-agent-$([guid]::NewGuid().ToString()).zip"
    $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("cockpit-agent-" + [guid]::NewGuid().ToString())
    try {
        Invoke-WebRequest -Uri $assetUrl -OutFile $tmpZip -UseBasicParsing
    } catch {
        throw "下载失败: $assetUrl`n  - 404：该平台（windows-$arch）的 nightly 产物可能尚未生成——每日构建在近 24h 有提交时于 00:00 UTC 重建；`n  - 网络问题：$($_.Exception.Message)"
    }
    Expand-Archive -Path $tmpZip -DestinationPath $tmpDir -Force
    $srcExe = Join-Path $tmpDir 'cockpit-agent.exe'
    if (-not (Test-Path $srcExe)) { throw "解包异常：包内未找到 cockpit-agent.exe" }

    Write-Step "安装到 $InstallDir ..."
    if (-not (Test-Path $InstallDir)) { New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null }
    $binPath = Join-Path $InstallDir 'cockpit-agent.exe'
    Copy-Item -Path $srcExe -Destination $binPath -Force
    Remove-Item $tmpZip, $tmpDir -Recurse -Force -ErrorAction SilentlyContinue

    # 用户 PATH（幂等：已存在则跳过）
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if ([string]::IsNullOrEmpty($userPath)) {
        [Environment]::SetEnvironmentVariable('Path', $InstallDir, 'User')
        Write-Host "已写入用户 PATH: $InstallDir" -ForegroundColor Green
    } elseif ("$userPath;" -notlike "*$InstallDir;*") {
        [Environment]::SetEnvironmentVariable('Path', "$userPath;$InstallDir", 'User')
        Write-Host "已加入用户 PATH: $InstallDir（新开终端生效）" -ForegroundColor Green
    }
    if ($env:Path -notlike "*$InstallDir*") { $env:Path += ";$InstallDir" }

    # 安装后验证
    $ver = & $binPath --version 2>$null
    if ($LASTEXITCODE -ne 0 -or -not ($ver -match '^Cockpit Agent v')) {
        throw "安装后验证失败：$binPath --version 输出异常"
    }
    Write-Host "已安装: $ver -> $binPath" -ForegroundColor Green

    # 服务注册（可选，需管理员）
    if ($WithService) {
        if (-not $isAdmin) {
            throw "注册 Windows 服务需要管理员权限——请以管理员身份运行 PowerShell 后重试"
        }
        if (-not $ServerUrl -and -not $svc) {
            throw "注册服务需要 -ServerUrl（或已注册过 $ServiceName 服务）"
        }

        if ($svc) {
            Write-Step "删除旧服务（重新注册以刷新启动参数）..."
            Remove-Service -Name $ServiceName
            Start-Sleep -Seconds 1
        }

        $startArgs = Build-StartArgs
        $arguments = $startArgs -join ' '
        Write-Step "注册 Windows 服务 $ServiceName ..."
        New-Service -Name $ServiceName `
            -BinaryPathName "`"$binPath`" $arguments" `
            -DisplayName "Cockpit Infrastructure Monitoring Agent" `
            -Description "Cockpit Agent - Connects to Cockpit Server for infrastructure monitoring" `
            -StartupType Automatic | Out-Null

        # 失败自动重启：5s / 10s / 20s，24h 内计数重置
        & sc.exe failure $ServiceName reset= 86400 actions= restart/5000/restart/10000/restart/20000 | Out-Null

        # 连接信息留档（服务参数改配置后需重跑本脚本重新注册）
        $configDir = 'C:\ProgramData\CockpitAgent'
        if (-not (Test-Path $configDir)) { New-Item -ItemType Directory -Path $configDir -Force | Out-Null }
        @"
SERVER_URL=$ServerUrl
AGENT_ID=$AgentId
SECRET=$Secret
REGION=$Region
ZONE=$Zone
LABELS=$Labels
"@ | Out-File -FilePath (Join-Path $configDir 'config.env') -Encoding ASCII

        Write-Step "启动服务..."
        Start-Service -Name $ServiceName
        Start-Sleep -Seconds 2
        $svc = Get-Service -Name $ServiceName
        if ($svc.Status -eq 'Running') {
            Write-Host "服务已启动: $ServiceName（Automatic + 失败自动重启）" -ForegroundColor Green
        } else {
            Write-Host "服务状态: $($svc.Status)——请检查事件查看器" -ForegroundColor Yellow
        }
    } elseif ($svc) {
        # 未要求注册但服务已存在：重启加载新版本（重跑=升级）
        if (-not $isAdmin) {
            Write-Host "检测到既有 $ServiceName 服务但当前非管理员——新版本将在下次服务重启时生效" -ForegroundColor Yellow
        } else {
            Write-Step "重启既有 $ServiceName 服务以加载新版本..."
            Start-Service -Name $ServiceName
            Write-Host "服务已重启: $ServiceName" -ForegroundColor Green
        }
    }

    Write-Host ""
    Write-Host "完成。下一步:" -ForegroundColor Cyan
    if (-not $WithService) {
        Write-Host "  注册 Windows 服务（管理员 PowerShell）:" -ForegroundColor White
        Write-Host "    & ([scriptblock]::Create((irm <本脚本URL>))) -WithService -ServerUrl `"wss://<server>/ws`"" -ForegroundColor White
        Write-Host "  或手动启动: cockpit-agent start -server ws://<server>/ws" -ForegroundColor White
    }
    Write-Host "  验证版本: cockpit-agent --version" -ForegroundColor White
    Write-Host "  查看服务: Get-Service -Name $ServiceName" -ForegroundColor White
} catch {
    Write-Host "错误: $_" -ForegroundColor Red
    exit 1
}
