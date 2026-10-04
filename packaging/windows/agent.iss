; Cockpit Agent Windows 安装器（Inno Setup 6）
;
; 产物：cockpit-agent-setup-<version>.exe（nightly 分发名 cockpit-agent-setup-nightly.exe；
;       与既有 zip 资产的 `cockpit-agent-<goos>-<goarch>-nightly.*` 命名刻意错开，
;       install.ps1 按架构拉 zip 不会误取安装器）
;
; 构建（CI Build Agent Installer 步骤已把载荷暂存到 stage\ 目录）：
;   ISCC.exe /DAppVersion=nightly /DStageDir=<abs>\stage /O<abs> packaging\windows\agent.iss
;
; 交互安装：勾选「注册 Windows 服务」后向导询问 Cockpit Server 的 WebSocket
;           地址（服务任务取消勾选则跳过该页，装完可手动
;           `cockpit-agent.exe service install -server ...`）。
; 静默安装：服务任务默认勾选，必须带 /SERVER="wss://..."（缺省时 service
;           install 参数校验失败 → 安装器非零退出）。
; 验收：安装到 Program Files、服务 Automatic + Running（开机自启）、开始菜单/
;       桌面快捷方式、控制面板卸载条目；静默 卸载 → 服务/文件/快捷方式全清。
;       round trip 由 nightly 的 Walkthrough 步骤在真 Windows runner 上走查。

#ifndef AppVersion
  #define AppVersion "nightly"
#endif
#ifndef StageDir
  ; 本地默认：仓库根相对路径（源文件路径相对本 iss 所在目录解析）
  #define StageDir "..\stage"
#endif

[Setup]
; AppId 固定 GUID：重复安装/升级合并为同一卸载条目（升级语义）
AppId={{5D9C2A47-8B3E-4F6C-9A51-E7D04C8B21F3}
AppName=Cockpit Agent
AppVersion={#AppVersion}
AppPublisher=cuihairu
AppPublisherURL=https://github.com/cuihairu/cockpit
; 服务注册需管理员（scm 写入）；装 Program Files（x64 位模式）
DefaultDirName={autopf}\Cockpit Agent
DefaultGroupName=Cockpit Agent
DisableProgramGroupPage=yes
; /SERVER=... 静默装机开关：{param:server} 为 Inno 原生常量，无需额外指令
SetupIconFile=agent.ico
UninstallDisplayName=Cockpit Agent
UninstallDisplayIcon={app}\cockpit-agent.exe
OutputBaseFilename=cockpit-agent-setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
PrivilegesRequired=admin
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
; 升级时服务/进程在跑：Restart Manager 先停后启，避免文件占用
CloseApplications=yes
RestartApplications=yes

[Tasks]
Name: "service"; Description: "注册 Windows 服务并开机自启（推荐）"; GroupDescription: "服务化:"
Name: "desktopicon"; Description: "创建桌面快捷方式(&D)"; GroupDescription: "附加图标:"

[Files]
; 载荷 = CI 暂存的 agent 运行时（cockpit-agent.exe）
Source: "{#StageDir}\*"; DestDir: "{app}"; Flags: ignoreversion recursesubdirs createallsubdirs

[Icons]
Name: "{group}\Cockpit Agent"; Filename: "{app}\cockpit-agent.exe"; WorkingDir: "{app}"
; 卸载入口（开始菜单；控制面板/设置卸载条目由 Inno 注册表键自动生成）
Name: "{group}\Uninstall Cockpit Agent"; Filename: "{uninstallexe}"
Name: "{autodesktop}\Cockpit Agent"; Filename: "{app}\cockpit-agent.exe"; WorkingDir: "{app}"; Tasks: desktopicon

[Run]
; 服务注册/升级（agent 侧子命令：Automatic + 失败自动重启 + 立即启动）。
; 静默安装的地址来自 /SERVER；交互安装来自向导连接页（ServerUrl 函数分流）
Filename: "{app}\cockpit-agent.exe"; \
  Parameters: "service install -server ""{code:ServerUrl}"""; \
  Tasks: service; Flags: runhidden

[UninstallRun]
; 先于文件删除执行：停服务并注销（agent 侧已含等待停止逻辑）
Filename: "{app}\cockpit-agent.exe"; Parameters: "service uninstall"; Flags: runhidden; RunOnceId: "CockpitAgentSvc"

[UninstallDelete]
; 服务运行日志与连接信息留档（ProgramData 不在 {app} 内，卸载统一清理）
Type: filesandordirs; Name: "{commonappdata}\CockpitAgent"

[Code]
var
  ServerPage: TInputQueryWizardPage;

procedure InitializeWizard();
begin
  ServerPage := CreateInputQueryPage(wpSelectTasks,
    'Cockpit Server 连接', 'Agent 要连接到哪台 Cockpit Server？',
    '服务注册需要服务器地址（安装后可重跑安装包或手动执行 service install 变更）。');
  ServerPage.Add('WebSocket 地址（如 wss://cockpit.example.com/ws）:', False);
  ServerPage.Values[0] := ExpandConstant('{param:server}');
end;

function ShouldSkipPage(PageID: Integer): Boolean;
begin
  { 未勾选服务任务时跳过连接页；其余页面一律不跳过 }
  Result := (PageID = ServerPage.ID) and (not IsTaskSelected('service'));
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = ServerPage.ID then begin
    if Trim(ServerPage.Values[0]) = '' then begin
      MsgBox('服务注册需要填写 Cockpit Server 的 WebSocket 地址。', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function ServerUrl(Param: String): String;
var
  Url: String;
begin
  { 静默安装不走向向导页：从 /SERVER 命令行开关取；交互安装取向导输入 }
  if WizardSilent() then
    Url := Trim(ExpandConstant('{param:server}'))
  else
    Url := Trim(ServerPage.Values[0]);
  { 仅在 service 任务被选中时才会被调用：此处兜底把「没给地址」变成
    显式失败（静默模式写日志、交互模式弹错），而不是让 service install
    拿空 -server 去校验后失败 }
  if Url = '' then
    RaiseException(
      '服务注册需要 Cockpit Server 的 WebSocket 地址。' + #13#10 +
      '静默安装请加：/SERVER="wss://cockpit.example.com/ws"');
  Result := Url;
end;
