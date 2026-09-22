import 'package:flutter_test/flutter_test.dart';
import 'package:cockpit_mobile/models/models.dart';

void main() {
  group('models fromJson 对齐后端序列化', () {
    test('Agent：region/zone 顶层字段（storage.Agent）', () {
      final a = Agent.fromJson({
        'id': 'ag-1',
        'region': 'cn',
        'zone': 'home',
        'capabilities': [
          {'type': 'docker', 'version': '27.0'},
          {'type': 'logs'},
        ],
        'hostname': 'node-01',
        'ip': '192.168.1.10',
        'status': 'online',
        'lastSeen': '2026-09-22T02:00:00Z',
      });
      expect(a.online, isTrue);
      expect(a.region, 'cn');
      expect(a.zone, 'home');
      expect(a.hasDocker, isTrue);
      expect(a.capabilities.length, 2);
    });

    test('Agent：docker 前缀能力判定与 offline 缺省', () {
      final a = Agent.fromJson({
        'id': 'ag-2',
        'capabilities': [
          {'type': 'docker.compose'},
        ],
        'hostname': 'node-02',
        'ip': '10.0.0.2',
      });
      expect(a.online, isFalse);
      expect(a.hasDocker, isTrue);
      expect(a.status, 'offline');
    });

    test('Alert：/alerts 包 data 且 snake_case 字段', () {
      final a = Alert.fromJson({
        'id': 'al-1',
        'type': 'error',
        'title': 'agent 离线',
        'message': 'node-01 心跳超时',
        'resource_id': 'ag-1',
        'resource_type': 'agent',
        'created_at': '2026-09-22T02:00:00Z',
        'read': false,
      });
      expect(a.type, 'error');
      expect(a.read, isFalse);
      expect(a.resourceId, 'ag-1');
    });

    test('ContainerInfo：Go 大写缩写词命名', () {
      final c = ContainerInfo.fromJson({
        'ID': 'abc123',
        'Name': 'nginx',
        'Image': 'nginx:latest',
        'ImageID': 'sha256:x',
        'State': 'running',
        'Status': 'Up 2 hours',
        'Labels': <String, String>{},
        'Created': 1758500000,
      });
      expect(c.id, 'abc123');
      expect(c.name, 'nginx');
      expect(c.state, 'running');
      expect(c.created, 1758500000);
    });

    test('LoginResponse：requires_totp 时携带 tmp_token', () {
      final r = LoginResponse.fromJson({
        'user_id': 'u-1',
        'username': 'admin',
        'requires_totp': true,
        'tmp_token': 'tmp-x',
      });
      expect(r.requiresTotp, isTrue);
      expect(r.token, isNull);
      expect(r.tmpToken, 'tmp-x');
    });

    test('AuditLog：admin/audit/logs 行 snake_case 字段', () {
      final l = AuditLog.fromJson({
        'id': 42,
        'user_id': 'u-1',
        'username': 'admin',
        'action': 'login',
        'resource': 'session',
        'resource_id': '',
        'details': '{}',
        'ip': '192.168.1.5',
        'user_agent': 'Dart/3.13',
        'status': 'success',
        'created_at': '2026-09-22T03:00:00Z',
      });
      expect(l.id, 42);
      expect(l.username, 'admin');
      expect(l.action, 'login');
      expect(l.status, 'success');
    });

    test('TotpVerifyResponse：换取正式 JWT', () {
      final r = TotpVerifyResponse.fromJson({
        'token': 'jwt-x',
        'expires_at': 1759000000,
        'user_id': 'u-1',
        'username': 'admin',
        'role': 'admin',
      });
      expect(r.token, 'jwt-x');
      expect(r.role, 'admin');
    });
  });
}
