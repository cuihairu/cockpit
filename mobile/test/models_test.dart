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

    test('Agent：cron/file 能力判定', () {
      final a = Agent.fromJson({
        'id': 'ag-3',
        'capabilities': [
          {'type': 'cron'},
          {'type': 'file'},
        ],
        'hostname': 'node-03',
        'ip': '10.0.0.3',
      });
      expect(a.hasCron, isTrue);
      expect(a.hasFile, isTrue);
      expect(a.hasDocker, isFalse);
      // stacks 与容器同门：docker/docker-api
      expect(a.hasStacks, isFalse);
    });

    test('StatusSummary：子对象全缺省时落 0', () {
      final s = StatusSummary.fromJson({});
      expect(s.agentsTotal, 0);
      expect(s.agentsOnline, 0);
      expect(s.domainsValid, 0);
      expect(s.domainsExpiring, 0);
      expect(s.certsValid, 0);
      expect(s.certsExpiring, 0);
      expect(s.servicesDown, 0);
      // 部分子对象缺省
      final s2 = StatusSummary.fromJson({
        'infrastructure': {'total': 2, 'online': 1},
      });
      expect(s2.agentsTotal, 2);
      expect(s2.certsExpiring, 0);
    });

    test('RemoteTicket：缺省字段落空串', () {
      final t = RemoteTicket.fromJson({});
      expect(t.ticket, '');
      expect(t.expiresAt, '');
    });

    test('Stack 系列：camelCase 字段 + info 缺省 + task.done 判定', () {
      final s = StackSummary.fromJson({
        'agentId': 'ag-1',
        'agentName': 'docker-1',
        'name': 'blog',
        'running': 2,
        'total': 3,
        'lastAction': 'up',
        'lastStatus': 'success',
        'lastDeployedAt': 1758586800,
        'online': true,
      });
      expect(s.name, 'blog');
      expect(s.running, 2);
      expect(s.total, 3);
      expect(s.online, isTrue);
      // 全缺省落零值
      final empty = StackSummary.fromJson({});
      expect(empty.name, '');
      expect(empty.lastDeployedAt, 0);
      expect(empty.online, isFalse);

      final info = StackInfo.fromJson(
          {'dir': '/opt/stacks', 'dirWritable': false, 'dirError': 'denied'});
      expect(info.dirWritable, isFalse);
      expect(info.dirError, 'denied');
      expect(info.composeVersion, isNull);
      // dirWritable 缺省按 true 兜底（server 不回 false 即视为可写）
      expect(StackInfo.fromJson({}).dirWritable, isTrue);

      final running = StackTask.fromJson(
          {'id': 't1', 'stack': 'blog', 'action': 'up', 'status': 'running'});
      expect(running.done, isFalse);
      expect(running.finishedAt, 0);
      final failed = StackTask.fromJson(
          {'status': 'failed', 'log': 'boom', 'finishedAt': 9});
      expect(failed.done, isTrue);
      expect(failed.log, 'boom');
      expect(StackTask.fromJson({'status': 'success'}).done, isTrue);

      final c = StackCompose.fromJson({
        'name': 'blog',
        'compose': 'services: {}',
        'env': '',
        'composeFile': '/opt/stacks/blog/compose.yml',
        'modifiedAt': 1758586800,
      });
      expect(c.env, '');
      expect(c.composeFile, '/opt/stacks/blog/compose.yml');

      // info 可为 null（agent 自检拉取失败）或缺键；空条目字面量需显式
      // <String, dynamic>（否则推断为 Map<dynamic, dynamic>，非真实 jsonDecode 形态）
      final r1 = StacksResult.fromJson(
          {'stacks': [<String, dynamic>{}], 'info': null});
      expect(r1.stacks, hasLength(1));
      expect(r1.info, isNull);
      final r2 = StacksResult.fromJson({});
      expect(r2.stacks, isEmpty);
      expect(r2.info, isNull);
      final r3 = StacksResult.fromJson(
          {'stacks': [], 'info': {'dir': '/opt/stacks', 'dirWritable': true}});
      expect(r3.info!.dir, '/opt/stacks');
    });

    test('Agent：smart/nas/overlay 能力判定（观测三入口的门）', () {
      // hardware-monitor 但无 smart 标志（仅温度/UPS）不算 SMART 主机
      final a = Agent.fromJson({
        'id': 'ag-1',
        'capabilities': [
          {'type': 'hardware-monitor', 'metadata': {'temperature': true}},
          {'type': 'nas'},
          {'type': 'overlay'},
        ],
        'hostname': 'n1',
        'ip': '10.0.0.1',
        'status': 'online',
        'lastSeen': '',
      });
      expect(a.hasSmart, isFalse);
      expect(a.hasNas, isTrue);
      expect(a.hasOverlay, isTrue);

      final b = Agent.fromJson({
        'id': 'ag-2',
        'capabilities': [
          {'type': 'hardware-monitor', 'metadata': {'smart': true}},
        ],
        'hostname': 'n2',
        'ip': '10.0.0.2',
        'status': 'online',
        'lastSeen': '',
      });
      expect(b.hasSmart, isTrue);
      expect(b.hasNas, isFalse);
      expect(b.hasOverlay, isFalse);
    });

    test('Smart 系列：available/devices + 指针字段缺省', () {
      final s = SmartStatus.fromJson({
        'available': true,
        'devices': [
          {
            'name': 'sda',
            'model': 'WD Red',
            'health': 'passed',
            'sizeBytes': 4000787030016,
            'temperatureC': 38,
            'powerOnHours': 12345,
          },
          {
            'name': 'sdb',
            'health': 'failed',
            'reallocatedSectors': 12,
            'error': 'smartctl exit 4',
          },
        ],
      });
      expect(s.available, isTrue);
      expect(s.devices, hasLength(2));
      final d0 = s.devices.first;
      expect(d0.powerOnHours, 12345);
      expect(d0.reallocatedSectors, isNull); // Go omitempty 指针字段
      final d1 = s.devices.last;
      expect(d1.health, 'failed');
      expect(d1.error, 'smartctl exit 4');

      // 全缺省：unknown 健康态 + 空列表
      final empty = SmartStatus.fromJson({});
      expect(empty.available, isFalse);
      expect(empty.devices, isEmpty);
      expect(empty.devices, isA<List<SmartDevice>>());
    });

    test('Nas 系列：快照三段 + host 来源设备', () {
      final n = NasStatus.fromJson({
        'available': true,
        'source': 'linux',
        'pools': [
          {
            'name': 'md0',
            'kind': 'mdadm',
            'state': 'degraded',
            'totalGB': 2000,
            'usedGB': 1000.5,
            'devices': ['sda1', 'sdb1'],
            'detail': 'U_',
            'host': '',
          },
        ],
        'mounts': [
          {
            'device': '/dev/md0',
            'mountPath': '/mnt/pool',
            'fsType': 'ext4',
            'totalGB': 2000,
            'usedGB': 1848,
            'host': '',
          },
        ],
        'shares': [
          {
            'protocol': 'smb',
            'name': 'media',
            'path': '/mnt/pool/media',
            'comment': '媒体库',
            'hosts': '192.168.0.0/16',
            'host': 'dsm1',
          },
        ],
      });
      expect(n.pools.single.state, 'degraded');
      expect(n.pools.single.usedGB, 1000.5);
      expect(n.mounts.single.mountPath, '/mnt/pool');
      expect(n.shares.single.host, 'dsm1');

      final empty = NasStatus.fromJson({});
      expect(empty.available, isFalse);
      expect(empty.source, '');
      expect(empty.pools, isEmpty);
      expect(empty.mounts, isEmpty);
      expect(empty.shares, isEmpty);
    });

    test('NasConfig：usage_warn_percent 阈值', () {
      final c = NasConfig.fromJson(
          {'scan_interval_seconds': 1800, 'usage_warn_percent': 75});
      expect(c.scanIntervalSeconds, 1800);
      expect(c.usageWarnPercent, 75);
      // 缺省兜底对齐 server 默认（1800 / 80）
      expect(NasConfig.fromJson({}).usageWarnPercent, 80);
    });

    test('Overlay 系列：tools/peers/interfaces 白名单字段', () {
      final o = OverlayStatus.fromJson({
        'tools': [
          {
            'tool': 'zerotier',
            'status': 'ok',
            'version': '1.14.0',
            'networks': [
              {
                'id': '8056c2e21c',
                'name': 'home',
                'status': 'OK',
                'type': 'PRIVATE',
                'dev': 'zt0',
                'ips': ['10.147.20.10'],
              },
            ],
            'peers': [
              {
                'id': 'a1b2c3',
                'latencyMs': 12,
                'online': true,
                'endpoint': '1.2.3.4/9993',
                'role': 'LEAF',
              },
            ],
          },
          {
            'tool': 'wireguard',
            'status': 'ok',
            'interfaces': [
              {
                'name': 'wg0',
                'listenPort': '51820',
                'peerCount': 2,
                'peers': [
                  {'id': 'pubkey==', 'online': true, 'virtualIps': ['10.9.0.2']},
                ],
              },
            ],
          },
          {'tool': 'tailscale', 'status': 'unavailable'},
        ],
      });
      expect(o.tools, hasLength(3));
      final zt = o.tools.first;
      expect(zt.networks.single.id, '8056c2e21c');
      expect(zt.networks.single.online, isNull); // ZT 网络无该键
      expect(zt.peers.single.online, isTrue);
      expect(zt.peers.single.latencyMs, 12);
      final wg = o.tools[1];
      expect(wg.interfaces.single.listenPort, '51820'); // wg dump 字符串列
      expect(wg.interfaces.single.peers.single.virtualIps, ['10.9.0.2']);
      expect(o.tools.last.status, 'unavailable');
      expect(o.tools.last.networks, isEmpty);

      // 空/缺省形态
      final empty = OverlayStatus.fromJson({});
      expect(empty.tools, isEmpty);
    });

  });
}
