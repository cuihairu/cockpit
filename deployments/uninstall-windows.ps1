# Cockpit Windows 卸载脚本
# 管理员权限运行: .\uninstall-windows.ps1

#requires -RunAsAdministrator

param(
    [Parameter(Mandatory=$false)]
    [ValidateSet("Server", "Agent", "All")]
    [string]$Component = "All"
)

Write-Host "Cockpit 卸载脚本" -ForegroundColor Cyan
Write-Host "===============" -ForegroundColor Cyan
Write-Host ""

function Remove-CockpitService {
    param([string]$ServiceName, [string]$DisplayName)

    Write-Host "检查 $DisplayName 服务..." -ForegroundColor Yellow

    $service = Get-Service -Name $ServiceName -ErrorAction SilentlyContinue
    if ($service) {
        Write-Host "停止服务..." -ForegroundColor Yellow
        try {
            Stop-Service -Name $ServiceName -Force -ErrorAction Stop
        } catch {
            Write-Host "停止失败: $_" -ForegroundColor Red
        }

        Write-Host "删除服务..." -ForegroundColor Yellow
        try {
            Remove-Service -Name $ServiceName
            Write-Host "$DisplayName 服务已删除" -ForegroundColor Green
        } catch {
            Write-Host "删除服务失败: $_" -ForegroundColor Red
            Write-Host "尝试使用 sc.exe..." -ForegroundColor Yellow
            & sc.exe delete $ServiceName
        }
    } else {
        Write-Host "$DisplayName 服务未安装" -ForegroundColor Gray
    }
}

function Remove-CockpitFiles {
    param([string]$Path, [string]$Description)

    if (Test-Path $Path) {
        Write-Host "删除 $Description ($Path)..." -ForegroundColor Yellow
        Remove-Item -Path $Path -Recurse -Force
        Write-Host "$Description 已删除" -ForegroundColor Green
    }
}

# 确认卸载
$confirmation = Read-Host "确定要卸载 Cockpit $Component 组件吗? (y/N)"
if ($confirmation -ne 'y' -and $confirmation -ne 'Y') {
    Write-Host "取消卸载" -ForegroundColor Yellow
    exit 0
}

# 根据 Component 参数选择要卸载的组件
switch ($Component) {
    "Server" {
        Remove-CockpitService -ServiceName "CockpitServer" -DisplayName "Cockpit Server"
        Remove-CockpitFiles -Path "C:\Program Files\Cockpit" -Description "Server 程序文件"
        Remove-CockpitFiles -Path "C:\ProgramData\Cockpit" -Description "Server 数据文件"
    }
    "Agent" {
        Remove-CockpitService -ServiceName "CockpitAgent" -DisplayName "Cockpit Agent"
        Remove-CockpitFiles -Path "C:\Program Files\CockpitAgent" -Description "Agent 程序文件"
        Remove-CockpitFiles -Path "C:\ProgramData\CockpitAgent" -Description "Agent 数据文件"
    }
    "All" {
        Remove-CockpitService -ServiceName "CockpitServer" -DisplayName "Cockpit Server"
        Remove-CockpitService -ServiceName "CockpitAgent" -DisplayName "Cockpit Agent"
        Remove-CockpitFiles -Path "C:\Program Files\Cockpit" -Description "Server 程序文件"
        Remove-CockpitFiles -Path "C:\ProgramData\Cockpit" -Description "Server 数据文件"
        Remove-CockpitFiles -Path "C:\Program Files\CockpitAgent" -Description "Agent 程序文件"
        Remove-CockpitFiles -Path "C:\ProgramData\CockpitAgent" -Description "Agent 数据文件"
    }
}

# 删除防火墙规则
Write-Host "删除防火墙规则..." -ForegroundColor Yellow
Remove-NetFirewallRule -DisplayName "Cockpit Server" -ErrorAction SilentlyContinue

Write-Host ""
Write-Host "卸载完成!" -ForegroundColor Green
Write-Host ""
Write-Host "注意: 配置文件和数据已删除。如需备份，请提前复制相关目录。" -ForegroundColor Yellow
