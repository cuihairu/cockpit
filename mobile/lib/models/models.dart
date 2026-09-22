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
