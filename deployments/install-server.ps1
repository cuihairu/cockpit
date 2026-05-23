# Cockpit Server Windows 安装脚本
# 管理员权限运行: .\install-server.ps1

#requires -RunAsAdministrator

param(
    [string]$InstallDir = "C:\Program Files\Cockpit",
    [string]$ReleaseVersion = "latest",
    [string]$BinaryUrl = ""
)

Write-Host "Cockpit Server 安装脚本" -ForegroundColor Cyan
Write-Host "======================" -ForegroundColor Cyan
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
$binaryName = "cockpit-windows-$arch.exe"
if ([string]::IsNullOrEmpty($BinaryUrl)) {
    $BinaryUrl = "https://github.com/cuihairu/cockpit/releases/download/$ReleaseVersion/$binaryName"
}

$binaryPath = "$InstallDir\cockpit.exe"

Write-Host "下载: $BinaryUrl" -ForegroundColor Yellow
try {
    Invoke-WebRequest -Uri $BinaryUrl -OutFile $binaryPath -UseBasicParsing
} catch {
    Write-Host "下载失败: $_" -ForegroundColor Red
    exit 1
}

# 创建数据目录
$dataDir = "C:\ProgramData\Cockpit"
if (!(Test-Path $dataDir)) {
    New-Item -ItemType Directory -Path $dataDir -Force | Out-Null
}

# 创建配置文件
$configPath = "$dataDir\config.yaml"
if (!(Test-Path $configPath)) {
    @"
# Cockpit Server Configuration
server:
  addr: "0.0.0.0:8080"

web:
  enabled: true
"@ | Out-File -FilePath $configPath -Encoding UTF8
    Write-Host "配置文件: $configPath" -ForegroundColor Green
}

# 创建服务
Write-Host "注册 Windows 服务..." -ForegroundColor Yellow

# 先删除旧服务（如果存在）
$serviceName = "CockpitServer"
if (Get-Service -Name $serviceName -ErrorAction SilentlyContinue) {
    Write-Host "删除旧服务..." -ForegroundColor Yellow
    Stop-Service -Name $serviceName -Force
    Remove-Service -Name $serviceName
    Start-Sleep -Seconds 2
}

# 创建新服务
$executablePath = $binaryPath
$arguments = "server start --config `"$configPath`""

try {
    New-Service -Name $serviceName `
        -BinaryPathName "`"$executablePath`" $arguments" `
        -DisplayName "Cockpit Infrastructure Management Server" `
        -Description "Personal hybrid infrastructure management platform" `
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

# 添加防火墙规则
Write-Host "配置防火墙..." -ForegroundColor Yellow
try {
    New-NetFirewallRule -DisplayName "Cockpit Server" `
        -Direction Inbound `
        -LocalPort 8080 `
        -Protocol TCP `
        -Action Allow `
        -Profile Any `
        -ErrorAction SilentlyContinue
    Write-Host "防火墙规则已添加" -ForegroundColor Green
} catch {
    Write-Host "防火墙配置失败（可能需要手动配置）: $_" -ForegroundColor Yellow
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
Write-Host "Web UI: http://localhost:8080" -ForegroundColor Cyan
