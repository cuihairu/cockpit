# Cockpit Agent 一键安装（Windows PowerShell 5.1+ / pwsh）
#
# 图形化安装首选 nightly release 的 cockpit-agent-setup-nightly.exe（Inno
# Setup 安装器：装目录/开始菜单/桌面快捷方式/服务化开机自启/卸载器一站式）。
# 本脚本是无 GUI/脚本化批量装机路径，能力对齐（服务注册经 agent 自身
# `service install` 子命令，SCM 协议完整应答）。
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
#       -WithService -ServerUrl "wss://cockpit.cuihairu.site/ws"
#
#   # 静默安装（零交互；不带 -ServerUrl 则装完打印手动配置指引）:
#   & ([scriptblock]::Create((irm <本脚本URL>))) -Silent -ServerUrl "cockpit.cuihairu.site"
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
    [switch]$WithService = ($env:COCKPIT_WITH_SERVICE -eq '1'),
    [switch]$Silent = ($env:COCKPIT_SILENT -eq '1')
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

function ConvertTo-ServerUrl {
    # 服务端地址规范化：域名/IP[:端口] → wss://<addr>/ws（纯地址默认 wss——
    # 服务端前置 nginx 80 端口 301 到 https，ws:// 握手不跟随重定向会失败）；
    # 完整 ws(s):// 地址原样保留并补 /ws 路径。无效输入返回 $null
    param([string]$Value)
    $s = "$Value".Trim()
    if (-not $s) { return $null }
    if ($s -match '^(?i)wss?://') {
        $s = $s.TrimEnd('/')
        if ($s -notmatch '/ws$') {
            $s = $s + '/ws'
        }
        return $s
    }
    # 域名 / IPv4[:端口]
    if ($s -notmatch '^[A-Za-z0-9._-]+(:\d{1,5})?$' -and $s -notmatch '^\[[0-9A-Fa-f:]+\](:\d{1,5})?$') {
        return $null
    }
    if ($s -match ':') {
        $port = ($s -split ':')[-1]
        if ($port -match '^\d+$' -and ([int]$port -lt 1 -or [int]$port -gt 65535)) { return $null }
    }
    return "wss://$s/ws"
}

function Test-ServerInConfig {
    # 校验 config.env 中 SERVER_URL 整行精确等于期望值
    param([string]$ConfigPath, [string]$Expected)
    if (-not (Test-Path $ConfigPath)) { return $false }
    return [bool](Select-String -Path $ConfigPath -Pattern ("^SERVER_URL=" + [regex]::Escape($Expected) + "$") -Quiet)
}

function Build-StartArgs {
    # 服务启动参数（与 cockpit-agent start 的 flag 面一一对应）。
    # 返回原始值不预加引号：调用点是 PowerShell 原生调用 `& $exe @args`，
    # 由 PowerShell 按 argv 语义自行转义；预加引号会把字面 " 传进 Go 的
    # flag 值（旧版拼 BinaryPathName 字符串才需要引号，该路径已收口）
    if ([string]::IsNullOrEmpty($ServerUrl)) { throw "注册服务需要 -ServerUrl（或既有 $ServiceName 服务）" }
    $startArgs = @('start', '-server', $ServerUrl)
    if (-not [string]::IsNullOrEmpty($AgentId)) { $startArgs += @('-id', $AgentId) }
    if (-not [string]::IsNullOrEmpty($Secret)) { $startArgs += @('-secret', $Secret) }
    if (-not [string]::IsNullOrEmpty($Region)) { $startArgs += @('-region', $Region) }
    if (-not [string]::IsNullOrEmpty($Zone)) { $startArgs += @('-zone', $Zone) }
    if (-not [string]::IsNullOrEmpty($Labels)) { $startArgs += @('-labels', $Labels) }
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

    # 服务端地址解析：显式参数 > 交互提示；注册服务拿不到地址则失败即停
    if (-not [string]::IsNullOrEmpty($ServerUrl)) {
        $normalized = ConvertTo-ServerUrl $ServerUrl
        if (-not $normalized) {
            throw "无效的服务端地址: $ServerUrl（应为 域名[:端口] 或完整 ws(s):// URL）"
        }
        $ServerUrl = $normalized
        Write-Host "服务端地址: $ServerUrl" -ForegroundColor White
    } else {
        $interactive = (-not $Silent) -and [Environment]::UserInteractive
        if ($interactive) {
            $tries = 3
            while ($tries -gt 0) {
                $addrInput = Read-Host "请输入 Cockpit 服务端地址（域名或 IP，例: cockpit.cuihairu.site 或 10.0.0.5:9000；完整 wss:// 地址亦可；直接回车跳过）"
                $addrInput = "$addrInput".Trim()
                if (-not $addrInput) { break }
                $normalized = ConvertTo-ServerUrl $addrInput
                if ($normalized) {
                    $ServerUrl = $normalized
                    Write-Host "服务端地址: $ServerUrl" -ForegroundColor White
                    break
                }
                Write-Host "[警告] 地址格式无效: $addrInput（应为 域名[:端口] 或完整 ws(s):// URL）" -ForegroundColor Yellow
                $tries--
            }
        }
        if ([string]::IsNullOrEmpty($ServerUrl) -and $WithService) {
            throw "注册服务需要 -ServerUrl=<域名或IP>，或以交互方式输入"
        }
    }

    # 显式传入服务端地址且既有服务 → 重注册刷新启动参数（Windows 服务参数固化在
    # BinaryPathName，仅改配置文件不生效，须重注册）
    $needRegister = $WithService -or ($svc -and -not [string]::IsNullOrEmpty($ServerUrl))

    # 服务注册（可选，需管理员）
    if ($needRegister) {
        if (-not $isAdmin) {
            throw "注册/更新 Windows 服务需要管理员权限——请以管理员身份运行 PowerShell 后重试"
        }
        if (-not $ServerUrl -and -not $svc) {
            throw "注册服务需要 -ServerUrl（或已注册过 $ServiceName 服务）"
        }

        $startArgs = Build-StartArgs
        # 服务注册交给 agent 自身子命令（二进制内置 SCM 协议应答：60 秒内
        # StartServiceCtrlDispatcher + 状态机，失败自动重启 5s/10s/20s）。
        # 存量 New-Service 直挂 `start` 会被 SCM 判定无响应杀死——此路径收口。
        # 已注册时 agent 内部走升级：停服务 → 刷新启动参数（BinaryPathName）→ 重启
        Write-Step "注册/升级 Windows 服务 $ServiceName（Automatic + 失败自动重启）..."
        $svcArgs = @('service', 'install') + ($startArgs | Select-Object -Skip 1)
        & $binPath @svcArgs
        if ($LASTEXITCODE -ne 0) { throw "service install 失败（exit $LASTEXITCODE）" }

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

        Write-Step "确认服务状态..."
        & $binPath service status
        $svc = Get-Service -Name $ServiceName
        if ($svc.Status -eq 'Running') {
            Write-Host "服务已启动: $ServiceName（Automatic + 失败自动重启）" -ForegroundColor Green
        } else {
            Write-Host "服务状态: $($svc.Status)——请检查事件查看器" -ForegroundColor Yellow
        }
    } elseif ($svc) {
        # 未要求注册但服务已存在（且未显式传地址）：重启加载新版本（重跑=升级）
        if (-not $isAdmin) {
            Write-Host "检测到既有 $ServiceName 服务但当前非管理员——新版本将在下次服务重启时生效" -ForegroundColor Yellow
        } else {
            Write-Step "重启既有 $ServiceName 服务以加载新版本..."
            Restart-Service -Name $ServiceName
            Write-Host "服务已重启: $ServiceName" -ForegroundColor Green
        }
    } elseif (-not [string]::IsNullOrEmpty($ServerUrl)) {
        # 未注册服务且服务端地址已知：写入用户级连接配置（装机时落盘）
        $configDir = Join-Path $env:APPDATA 'CockpitAgent'
        if (-not (Test-Path $configDir)) { New-Item -ItemType Directory -Path $configDir -Force | Out-Null }
        $userConfig = Join-Path $configDir 'config.env'
        @"
SERVER_URL=$ServerUrl
AGENT_ID=$AgentId
SECRET=$Secret
REGION=$Region
ZONE=$Zone
LABELS=$Labels
"@ | Out-File -FilePath $userConfig -Encoding ASCII
        if (-not (Test-ServerInConfig $userConfig $ServerUrl)) {
            throw "配置写入校验失败: $userConfig 中未找到 SERVER_URL=$ServerUrl"
        }
        Write-Host "连接配置已写入: $userConfig（SERVER_URL=$ServerUrl）" -ForegroundColor Green
    }

    Write-Host ""
    Write-Host "完成。下一步:" -ForegroundColor Cyan
    if (-not $needRegister) {
        Write-Host "  注册 Windows 服务（管理员 PowerShell）:" -ForegroundColor White
        Write-Host "    & ([scriptblock]::Create((irm <本脚本URL>))) -WithService -ServerUrl `"wss://cockpit.cuihairu.site/ws`"" -ForegroundColor White
        if (-not [string]::IsNullOrEmpty($ServerUrl)) {
            Write-Host "  或手动启动: cockpit-agent start -server $ServerUrl" -ForegroundColor White
        } else {
            Write-Host "  未配置服务端地址——之后自己手动执行配置:" -ForegroundColor White
            Write-Host "    cockpit-agent start -server wss://<你的服务端>/ws（例: wss://cockpit.cuihairu.site/ws）" -ForegroundColor White
            Write-Host "    或重跑本脚本并带 -ServerUrl <域名或IP>（自动写入连接配置）" -ForegroundColor White
        }
    }
    Write-Host "  验证版本: cockpit-agent --version" -ForegroundColor White
    Write-Host "  查看服务: Get-Service -Name $ServiceName" -ForegroundColor White
} catch {
    Write-Host "错误: $_" -ForegroundColor Red
    exit 1
}
