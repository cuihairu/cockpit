// 与后端 Go model json tag 对齐的数据模型。
// 字段以 `internal/storage` 序列化为准，禁止凭记忆互抄（见 web 侧 8f928baf 教训）。

class Capability {
  final String type;
  final String? endpoint;
  final String? version;
  final Map<String, dynamic>? metadata;

  Capability({required this.type, this.endpoint, this.version, this.metadata});

  factory Capability.fromJson(Map<String, dynamic> j) => Capability(
        type: j['type'] as String,
        endpoint: j['endpoint'] as String?,
        version: j['version'] as String?,
        metadata: j['metadata'] as Map<String, dynamic>?,
      );
}

/// 对齐 storage.Agent：region/zone 为顶层可选字段。
class Agent {
  final String id;
  final String? region;
  final String? zone;
  final List<Capability> capabilities;
  final String hostname;
  final String ip;
  final String status; // online / offline
  final String lastSeen; // RFC3339

  Agent({
    required this.id,
    this.region,
    this.zone,
    required this.capabilities,
    required this.hostname,
    required this.ip,
    required this.status,
    required this.lastSeen,
  });

  bool get online => status == 'online';
  bool get hasDocker => capabilities.any((c) => c.type.startsWith('docker'));
  bool get hasCron => capabilities.any((c) => c.type == 'cron');

  factory Agent.fromJson(Map<String, dynamic> j) => Agent(
        id: j['id'] as String,
        region: j['region'] as String?,
        zone: j['zone'] as String?,
        capabilities: (j['capabilities'] as List<dynamic>? ?? [])
            .map((e) => Capability.fromJson(e as Map<String, dynamic>))
            .toList(),
        hostname: j['hostname'] as String? ?? '',
        ip: j['ip'] as String? ?? '',
        status: j['status'] as String? ?? 'offline',
        lastSeen: j['lastSeen'] as String? ?? '',
      );
}

/// 对齐 web services/api.ts Alert（后端 alert 表）。
class Alert {
  final String id;
  final String type; // info / warning / error / success
  final String title;
  final String message;
  final String? resourceId;
  final String? resourceType;
  final String createdAt;
  final bool read;

  Alert({
    required this.id,
    required this.type,
    required this.title,
    required this.message,
    this.resourceId,
    this.resourceType,
    required this.createdAt,
    required this.read,
  });

  factory Alert.fromJson(Map<String, dynamic> j) => Alert(
        id: j['id'] as String,
        type: j['type'] as String? ?? 'info',
        title: j['title'] as String? ?? '',
        message: j['message'] as String? ?? '',
        resourceId: j['resource_id'] as String?,
        resourceType: j['resource_type'] as String?,
        createdAt: j['created_at'] as String? ?? '',
        read: j['read'] as bool? ?? false,
      );
}

/// 对齐 internal/docker ContainerInfo：Go 大写缩写词命名，Created 为 Unix 秒。
class ContainerInfo {
  final String id;
  final String name;
  final String image;
  final String state; // running / paused / exited / created
  final String status; // 可读状态串，如 "Up 2 hours"
  final int created;

  ContainerInfo({
    required this.id,
    required this.name,
    required this.image,
    required this.state,
    required this.status,
    required this.created,
  });

  factory ContainerInfo.fromJson(Map<String, dynamic> j) => ContainerInfo(
        id: j['ID'] as String? ?? '',
        name: j['Name'] as String? ?? '',
        image: j['Image'] as String? ?? '',
        state: j['State'] as String? ?? '',
        status: j['Status'] as String? ?? '',
        created: (j['Created'] as num?)?.toInt() ?? 0,
      );
}

/// 对齐 POST /api/auth/login 响应：requires_totp 时携带 tmp_token 走二次验证。
class LoginResponse {
  final String? token;
  final int? expiresAt;
  final String userId;
  final String username;
  final String? role;
  final bool requiresTotp;
  final String? tmpToken;

  LoginResponse({
    this.token,
    this.expiresAt,
    required this.userId,
    required this.username,
    this.role,
    required this.requiresTotp,
    this.tmpToken,
  });

  factory LoginResponse.fromJson(Map<String, dynamic> j) => LoginResponse(
        token: j['token'] as String?,
        expiresAt: (j['expires_at'] as num?)?.toInt(),
        userId: j['user_id'] as String? ?? '',
        username: j['username'] as String? ?? '',
        role: j['role'] as String?,
        requiresTotp: j['requires_totp'] as bool? ?? false,
        tmpToken: j['tmp_token'] as String?,
      );
}

/// 对齐 POST /api/auth/totp/verify 响应。
class TotpVerifyResponse {
  final String token;
  final int expiresAt;
  final String userId;
  final String username;
  final String role;

  TotpVerifyResponse({
    required this.token,
    required this.expiresAt,
    required this.userId,
    required this.username,
    required this.role,
  });

  factory TotpVerifyResponse.fromJson(Map<String, dynamic> j) =>
      TotpVerifyResponse(
        token: j['token'] as String,
        expiresAt: (j['expires_at'] as num?)?.toInt() ?? 0,
        userId: j['user_id'] as String? ?? '',
        username: j['username'] as String? ?? '',
        role: j['role'] as String? ?? '',
      );
}

/// 对齐 GET /api/admin/audit/logs 行（web AuditLog 同款 snake_case）。
class AuditLog {
  final int id;
  final String userId;
  final String username;
  final String action;
  final String resource;
  final String resourceId;
  final String details;
  final String ip;
  final String status;
  final String createdAt;

  AuditLog({
    required this.id,
    required this.userId,
    required this.username,
    required this.action,
    required this.resource,
    required this.resourceId,
    required this.details,
    required this.ip,
    required this.status,
    required this.createdAt,
  });

  factory AuditLog.fromJson(Map<String, dynamic> j) => AuditLog(
        id: (j['id'] as num).toInt(),
        userId: j['user_id'] as String? ?? '',
        username: j['username'] as String? ?? '',
        action: j['action'] as String? ?? '',
        resource: j['resource'] as String? ?? '',
        resourceId: j['resource_id'] as String? ?? '',
        details: j['details'] as String? ?? '',
        ip: j['ip'] as String? ?? '',
        status: j['status'] as String? ?? '',
        createdAt: j['created_at'] as String? ?? '',
      );
}

/// 对齐 web Domain：status/daysRemaining 由后端算好，前端不重算。
class Domain {
  final String id;
  final String name;
  final String registrar;
  final String expiryDate;
  final bool autoRenew;
  final String dnsProvider;
  final String status; // valid / expiring / expired / 空=未知

  Domain({
    required this.id,
    required this.name,
    required this.registrar,
    required this.expiryDate,
    required this.autoRenew,
    required this.dnsProvider,
    required this.status,
  });

  factory Domain.fromJson(Map<String, dynamic> j) => Domain(
        id: j['id'] as String,
        name: j['name'] as String? ?? '',
        registrar: j['registrar'] as String? ?? '',
        expiryDate: j['expiryDate'] as String? ?? '',
        autoRenew: j['autoRenew'] as bool? ?? false,
        dnsProvider: j['dnsProvider'] as String? ?? '',
        status: j['status'] as String? ?? '',
      );
}

/// 对齐 web Certificate（移动端展示子集）。
class Certificate {
  final String id;
  final String commonName;
  final String type;
  final String expiryDate;
  final bool autoRenew;
  final String? acmeProvider;
  final int? daysRemaining;
  final String status; // valid / expiring / expired / 空=未知

  Certificate({
    required this.id,
    required this.commonName,
    required this.type,
    required this.expiryDate,
    required this.autoRenew,
    this.acmeProvider,
    this.daysRemaining,
    required this.status,
  });

  factory Certificate.fromJson(Map<String, dynamic> j) => Certificate(
        id: j['id'] as String,
        commonName: j['commonName'] as String? ?? '',
        type: j['type'] as String? ?? '',
        expiryDate: j['expiryDate'] as String? ?? '',
        autoRenew: j['autoRenew'] as bool? ?? false,
        acmeProvider: j['acmeProvider'] as String?,
        daysRemaining: (j['daysRemaining'] as num?)?.toInt(),
        status: j['status'] as String? ?? '',
      );
}

/// 对齐 GET /api/status 聚合（仪表盘四组计数）。
class StatusSummary {
  final int agentsTotal;
  final int agentsOnline;
  final int domainsValid;
  final int domainsExpiring;
  final int certsValid;
  final int certsExpiring;
  final int servicesDown;

  StatusSummary({
    required this.agentsTotal,
    required this.agentsOnline,
    required this.domainsValid,
    required this.domainsExpiring,
    required this.certsValid,
    required this.certsExpiring,
    required this.servicesDown,
  });

  factory StatusSummary.fromJson(Map<String, dynamic> j) {
    final infra = j['infrastructure'] as Map<String, dynamic>? ?? {};
    final domains = j['domains'] as Map<String, dynamic>? ?? {};
    final certs = j['certificates'] as Map<String, dynamic>? ?? {};
    final services = j['services'] as Map<String, dynamic>? ?? {};
    return StatusSummary(
      agentsTotal: (infra['total'] as num?)?.toInt() ?? 0,
      agentsOnline: (infra['online'] as num?)?.toInt() ?? 0,
      domainsValid: (domains['valid'] as num?)?.toInt() ?? 0,
      domainsExpiring: (domains['expiring'] as num?)?.toInt() ?? 0,
      certsValid: (certs['valid'] as num?)?.toInt() ?? 0,
      certsExpiring: (certs['expiring'] as num?)?.toInt() ?? 0,
      servicesDown: (services['down'] as num?)?.toInt() ?? 0,
    );
  }
}

/// 对齐 web BackupConfig（agent 侧备份任务）。
class BackupConfig {
  final int id;
  final String agentId;
  final String name;
  final List<String> sources;
  final String schedule; // manual / daily@HH:mm / every:Nh
  final bool enabled;
  final String lastStatus; // "" / running / success / failed
  final int lastRunAt; // unix 秒
  final int nextRunAt; // manual 恒为 0

  BackupConfig({
    required this.id,
    required this.agentId,
    required this.name,
    required this.sources,
    required this.schedule,
    required this.enabled,
    required this.lastStatus,
    required this.lastRunAt,
    required this.nextRunAt,
  });

  factory BackupConfig.fromJson(Map<String, dynamic> j) => BackupConfig(
        id: (j['id'] as num).toInt(),
        agentId: j['agent_id'] as String? ?? '',
        name: j['name'] as String? ?? '',
        sources: (j['sources'] as List<dynamic>? ?? [])
            .map((e) => e as String)
            .toList(),
        schedule: j['schedule'] as String? ?? 'manual',
        enabled: j['enabled'] as bool? ?? false,
        lastStatus: j['last_status'] as String? ?? '',
        lastRunAt: (j['last_run_at'] as num?)?.toInt() ?? 0,
        nextRunAt: (j['next_run_at'] as num?)?.toInt() ?? 0,
      );
}

/// 对齐 web BackupRun（camelCase，remoteStatus 空=未启用异地）。
class BackupRun {
  final int id;
  final int configId;
  final String status; // running / success / failed / timeout
  final int size;
  final String error;
  final String remoteStatus; // "" / ok / failed
  final int startedAt; // unix 秒
  final int finishedAt; // unix 秒

  BackupRun({
    required this.id,
    required this.configId,
    required this.status,
    required this.size,
    required this.error,
    required this.remoteStatus,
    required this.startedAt,
    required this.finishedAt,
  });

  factory BackupRun.fromJson(Map<String, dynamic> j) => BackupRun(
        id: (j['id'] as num).toInt(),
        configId: (j['configId'] as num?)?.toInt() ?? 0,
        status: j['status'] as String? ?? '',
        size: (j['size'] as num?)?.toInt() ?? 0,
        error: j['error'] as String? ?? '',
        remoteStatus: j['remoteStatus'] as String? ?? '',
        startedAt: (j['startedAt'] as num?)?.toInt() ?? 0,
        finishedAt: (j['finishedAt'] as num?)?.toInt() ?? 0,
      );
}

/// 对齐 web CronJob（cockpit 名下任务，next_run unix 秒可选）。
class CronJob {
  final String name;
  final String schedule;
  final String command;
  final bool enabled;
  final int nextRun; // 0 或缺省 = 未知

  CronJob({
    required this.name,
    required this.schedule,
    required this.command,
    required this.enabled,
    required this.nextRun,
  });

  factory CronJob.fromJson(Map<String, dynamic> j) => CronJob(
        name: j['name'] as String? ?? '',
        schedule: j['schedule'] as String? ?? '',
        command: j['command'] as String? ?? '',
        enabled: j['enabled'] as bool? ?? false,
        nextRun: (j['next_run'] as num?)?.toInt() ?? 0,
      );
}

/// 对齐 web CronJobsResult：jobs + 外部条目原文（只读）。
class CronJobsResult {
  final List<CronJob> jobs;
  final String external;

  CronJobsResult({required this.jobs, required this.external});

  factory CronJobsResult.fromJson(Map<String, dynamic> j) =>
      CronJobsResult(
        jobs: (j['jobs'] as List<dynamic>? ?? [])
            .map((e) => CronJob.fromJson(e as Map<String, dynamic>))
            .toList(),
        external: j['external'] as String? ?? '',
      );
}
