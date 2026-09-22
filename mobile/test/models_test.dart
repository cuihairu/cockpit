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

    test('BackupConfig/BackupRun：snake_case 配置 + camelCase 运行', () {
      final c = BackupConfig.fromJson({
        'id': 7,
        'agent_id': 'ag-1',
        'name': 'etc-backup',
        'sources': ['/etc', '/opt/data'],
        'dest_dir': '/var/backups',
        'schedule': 'daily@03:00',
        'retention': 7,
        'enabled': true,
        'last_status': 'failed',
        'last_run_at': 1758500000,
        'next_run_at': 1758586400,
        'created_at': 1750000000,
      });
      expect(c.id, 7);
      expect(c.sources.length, 2);
      expect(c.schedule, 'daily@03:00');
      expect(c.lastStatus, 'failed');

      final r = BackupRun.fromJson({
        'id': 99,
        'configId': 7,
        'taskId': 't-1',
        'status': 'timeout',
        'file': 'etc-backup-20260922.tar.gz',
        'size': 52428800,
        'error': 'context deadline exceeded',
        'remoteStatus': 'ok',
        'startedAt': 1758500000,
        'finishedAt': 1758500900,
      });
      expect(r.configId, 7);
      expect(r.status, 'timeout');
      expect(r.size, 52428800);
      expect(r.remoteStatus, 'ok');
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
