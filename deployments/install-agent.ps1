# Cockpit Agent Windows 安装脚本
# 管理员权限运行: .\install-agent.ps1 -ServerUrl "ws://server:8080"

#requires -RunAsAdministrator

param(
    [Parameter(Mandatory=$false)]
    [string]$ServerUrl = "",

    [Parameter(Mandatory=$false)]
    [string]$AgentId = "",

    [Parameter(Mandatory=$false)]
    [string]$Region = "",

    [Parameter(Mandatory=$false)]
    [string]$Zone = "",

    [Parameter(Mandatory=$false)]
    [string]$InstallDir = "C:\Program Files\CockpitAgent",

    [Parameter(Mandatory=$false)]
    [string]$ReleaseVersion = "latest",

    [Parameter(Mandatory=$false)]
    [string]$BinaryUrl = ""
)

Write-Host "Cockpit Agent 安装脚本" -ForegroundColor Cyan
Write-Host "=====================" -ForegroundColor Cyan
Write-Host ""

# 检测架构
$arch = if ([Environment]::Is64BitOperatingSystem) { "amd64" } else { "386" }
Write-Host "系统架构: $arch" -ForegroundColor Green

# 创建安装目录
Write-Host "创建安装目录: $InstallDir" -ForegroundColor Yellow
if (!(Test-Path $InstallDir)) {
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
}

# 下载二进制文件
$binaryName = "cockpit-agent-windows-$arch.exe"
if ([string]::IsNullOrEmpty($BinaryUrl)) {
    $BinaryUrl = "https://github.com/cuihairu/cockpit/releases/download/$ReleaseVersion/$binaryName"
}

$binaryPath = "$InstallDir\cockpit-agent.exe"

Write-Host "下载: $BinaryUrl" -ForegroundColor Yellow
try {
    Invoke-WebRequest -Uri $BinaryUrl -OutFile $binaryPath -UseBasicParsing
} catch {
    Write-Host "下载失败: $_" -ForegroundColor Red
    exit 1
}

# 创建配置文件
$configDir = "C:\ProgramData\CockpitAgent"
if (!(Test-Path $configDir)) {
    New-Item -ItemType Directory -Path $configDir -Force | Out-Null
}

$configPath = "$configDir\config.env"

# 如果没有通过参数提供 ServerUrl，则提示输入
if ([string]::IsNullOrEmpty($ServerUrl)) {
    $ServerUrl = Read-Host "请输入 Server WebSocket 地址 (例如: ws://192.168.1.10:8080)"
}

# 创建配置文件
@"
# Cockpit Agent Configuration
SERVER_URL=$ServerUrl
REGION=$Region
ZONE=$Zone
AGENT_ID=$AgentId
"@ | Out-File -FilePath $configPath -Encoding ASCII

Write-Host "配置文件: $configPath" -ForegroundColor Green

# 创建服务
Write-Host "注册 Windows 服务..." -ForegroundColor Yellow

# 先删除旧服务（如果存在）
$serviceName = "CockpitAgent"
if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
    Write-Host "删除旧服务..." -ForegroundColor Yellow
    Stop-Service -Name $serviceName -Force
    Remove-Service -Name $serviceName
    Start-Sleep -Seconds 2
}

# 构建启动参数
$startArgs = @()
$startArgs += "start"
$startArgs += "-server", "`"$ServerUrl`""

if (![string]::IsNullOrEmpty($AgentId)) {
    $startArgs += "-id", "`"$AgentId`""
}
if (![string]::IsNullOrEmpty($Region)) {
    $startArgs += "-region", "`"$Region`""
}
if (![string]::IsNullOrEmpty($Zone)) {
    $startArgs += "-zone", "`"$Zone`""
}

$arguments = $startArgs -join " "

# 创建新服务
try {
    New-Service -Name $serviceName `
        -BinaryPathName "`"$binaryPath`" $arguments" `
        -DisplayName "Cockpit Infrastructure Monitoring Agent" `
        -Description "Cockpit Agent - Connects to Cockpit Server for infrastructure monitoring" `
        -StartupType Automatic
} catch {
    Write-Host "创建服务失败: $_" -ForegroundColor Red
    exit 1
}

# 配置服务恢复选项
Write-Host "配置服务恢复策略..." -ForegroundColor Yellow
& sc.exe failure $serviceName reset= 86400 actions= restart/5000/restart/10000/restart/20000

# 启动服务
Write-Host "启动服务..." -ForegroundColor Yellow
try {
    Start-Service -Name $serviceName
    Start-Sleep -Seconds 2

    $service = Get-Service -Name $serviceName
    if ($service.Status -eq "Running") {
        Write-Host "服务已启动!" -ForegroundColor Green
    } else {
        Write-Host "服务状态: $($service.Status)" -ForegroundColor Yellow
        Write-Host "请检查事件查看器获取详细错误信息" -ForegroundColor Yellow
    }
} catch {
    Write-Host "启动服务失败: $_" -ForegroundColor Red
    Write-Host "手动启动: Start-Service -Name $serviceName" -ForegroundColor Yellow
}

Write-Host ""
Write-Host "安装完成!" -ForegroundColor Green
Write-Host ""
Write-Host "管理命令:" -ForegroundColor Cyan
Write-Host "  查看状态: Get-Service -Name $serviceName" -ForegroundColor White
Write-Host "  启动服务: Start-Service -Name $serviceName" -ForegroundColor White
Write-Host "  停止服务: Stop-Service -Name $serviceName" -ForegroundColor White
Write-Host "  重启服务: Restart-Service -Name $serviceName" -ForegroundColor White
Write-Host "  查看日志: Get-EventLog -LogName Application -Source $serviceName -Newest 50" -ForegroundColor White
Write-Host "  卸载服务: & sc.exe delete $serviceName" -ForegroundColor White
Write-Host ""
Write-Host "修改配置后需要重新安装服务:" -ForegroundColor Yellow
Write-Host "  .\install-agent.ps1 -ServerUrl `"ws://your-server:8080`" -Region `"jiangsu-huaian`" -Zone `"datacenter-a`"" -ForegroundColor White
