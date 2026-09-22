import 'package:dio/dio.dart';

import 'client.dart';

import '../models/models.dart';

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
