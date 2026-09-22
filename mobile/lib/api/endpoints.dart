import 'package:dio/dio.dart';

import 'client.dart';

import '../models/models.dart';

/// 分页响应壳：data + pagination.total。

/// M1 端点封装。响应形状逐一对齐 web `services/api.ts`：
/// `/agents` 裸数组、`/alerts` 包 `{data}`、Docker 走 `/docker/agents/{id}` 代理。
class CockpitApi {
  CockpitApi(this.client);

  final ApiClient client;

  Future<LoginResponse> login(String username, String password) async {
    final r = await client.dio.post<Map<String, dynamic>>('/api/auth/login',
        data: {'username': username, 'password': password},
        options: Options(extra: {'skipAuth': true}));
    return LoginResponse.fromJson(r.data!);
  }

  Future<TotpVerifyResponse> verifyTotp(String code, String tmpToken) async {
    final r = await client.dio.post<Map<String, dynamic>>(
        '/api/auth/totp/verify',
        data: {'code': code, 'tmp_token': tmpToken},
        options: Options(extra: {'skipAuth': true}));
    return TotpVerifyResponse.fromJson(r.data!);
  }

  Future<List<Agent>> agents() async {
    final r = await client.dio.get<List<dynamic>>('/api/agents');
    return r.data!.map((e) => Agent.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<Alert>> alerts() async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/alerts');
    final data = r.data!['data'] as List<dynamic>? ?? [];
    return data.map((e) => Alert.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<void> readAllAlerts() =>
      client.dio.put('/api/alerts/read-all');

  Future<List<ContainerInfo>> containers(String agentId) async {
    final r = await client.dio.get<List<dynamic>>(
        '/api/docker/agents/$agentId/containers',
        queryParameters: {'all': true});
    return r.data!
        .map((e) => ContainerInfo.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  Future<void> containerAction(
      String agentId, String containerId, String action) async {
    final allowed = ['start', 'stop', 'restart', 'pause', 'unpause'];
    if (!allowed.contains(action)) {
      throw ArgumentError('unsupported container action: $action');
    }
    await client.dio
        .post('/api/docker/agents/$agentId/containers/$containerId/$action');
  }
}

class AuditLogsPage {
  final List<AuditLog> data;
  final int total;

  AuditLogsPage({required this.data, required this.total});
}

extension AuditApi on CockpitApi {
  /// GET /api/admin/audit/logs?page=&page_size=
  Future<AuditLogsPage> auditLogs({int page = 1, int pageSize = 20}) async {
    final r = await client.dio.get<Map<String, dynamic>>('/api/admin/audit/logs',
        queryParameters: {'page': page, 'page_size': pageSize});
    final d = r.data!;
    final data = (d['data'] as List<dynamic>? ?? [])
        .map((e) => AuditLog.fromJson(e as Map<String, dynamic>))
        .toList();
    final total =
        ((d['pagination'] as Map<String, dynamic>?)?['total'] as num?)?.toInt() ?? 0;
    return AuditLogsPage(data: data, total: total);
  }
}

extension ResourceApi on CockpitApi {
  /// GET /api/status（仪表盘聚合）。
  Future<StatusSummary> status() async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/status');
    return StatusSummary.fromJson(r.data!);
  }

  Future<List<Domain>> domains() async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/resources/domains');
    final data = r.data!['data'] as List<dynamic>? ?? [];
    return data.map((e) => Domain.fromJson(e as Map<String, dynamic>)).toList();
  }

  Future<List<Certificate>> certificates() async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/resources/certificates');
    final data = r.data!['data'] as List<dynamic>? ?? [];
    return data
        .map((e) => Certificate.fromJson(e as Map<String, dynamic>))
        .toList();
  }
}

extension BackupApi on CockpitApi {
  /// GET /api/backups/configs → {configs}
  Future<List<BackupConfig>> backupConfigs() async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/backups/configs');
    final data = r.data!['configs'] as List<dynamic>? ?? [];
    return data
        .map((e) => BackupConfig.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// GET /api/backups/runs?limit=N → {runs}
  Future<List<BackupRun>> backupRuns({int limit = 5}) async {
    final r = await client.dio.get<Map<String, dynamic>>('/api/backups/runs',
        queryParameters: {'limit': limit});
    final data = r.data!['runs'] as List<dynamic>? ?? [];
    return data
        .map((e) => BackupRun.fromJson(e as Map<String, dynamic>))
        .toList();
  }

  /// POST /api/backups/configs/{id}/run → {status}（手动触发一次）。
  Future<String> runBackup(int configId) async {
    final r = await client.dio
        .post<Map<String, dynamic>>('/api/backups/configs/$configId/run');
    return r.data?['status'] as String? ?? '';
  }
}

extension CronApi on CockpitApi {
  /// GET /api/agents/{id}/cron/jobs（cockpit 任务 + 外部条目原文）。
  Future<CronJobsResult> cronJobs(String agentId) async {
    final r = await client.dio
        .get<Map<String, dynamic>>('/api/agents/$agentId/cron/jobs');
    return CronJobsResult.fromJson(r.data!);
  }
}

extension FilesApi on CockpitApi {
  /// POST /api/agents/{id}/files/list——列目录直属条目（移动端只读浏览）。
  Future<FileListResult> listFiles(String agentId, String dir) async {
    final r = await client.dio.post<Map<String, dynamic>>(
        '/api/agents/$agentId/files/list',
        data: {'dir': dir});
    return FileListResult.fromJson(r.data!);
  }
}


extension RemoteApi on CockpitApi {
  /// POST /api/remote/tickets——换取终端 WS 子协议票据（body snake_case 对齐 web）。
  Future<RemoteTicket> createRemoteTicket({
    required String agentId,
    required String host,
    required int port,
    String? username,
    String? password,
  }) async {
    final r = await client.dio.post<Map<String, dynamic>>('/api/remote/tickets',
        data: {
          'agent_id': agentId,
          'host': host,
          'port': port,
          'protocol': 'ssh',
          'username': ?username,
          'password': ?password,
        });
    return RemoteTicket.fromJson(r.data!);
  }
}
