import 'dart:io';

import 'package:dio/dio.dart';
import 'package:dio/io.dart';
import 'package:meta/meta.dart';

/// server API 客户端：base URL 可切换、JWT 自动附带、401 刷新重放一次、
/// 自签证书显式开关（默认严格校验，见 mobile-design D4/D5）。
class ApiClient {
  ApiClient._(this.dio);

  /// 测试专用：注入预配置的 Dio（mock adapter）。
  @visibleForTesting
  ApiClient.forTest(this.dio) {
    _installInterceptors();
  }

  /// 测试专用：注入 Dio 并带完整回调（401 刷新链路验证）。
  @visibleForTesting
  ApiClient.createWith({
    required this.dio,
    String? initialToken,
    void Function(String token)? onTokenRefreshed,
    Future<void> Function()? onUnauthorized,
  }) {
    _token = initialToken;
    _onTokenRefreshed = onTokenRefreshed;
    _onUnauthorized = onUnauthorized;
    _installInterceptors();
  }

  final Dio dio;
  String? _token;
  void Function(String token)? _onTokenRefreshed;
  Future<void> Function()? _onUnauthorized;

  static Future<ApiClient> create({
    required String baseUrl,
    required bool allowSelfSigned,
    String? initialToken,
    void Function(String token)? onTokenRefreshed,
    Future<void> Function()? onUnauthorized,
  }) async {
    final dio = Dio(BaseOptions(
      baseUrl: _normalize(baseUrl),
      connectTimeout: const Duration(seconds: 8),
      receiveTimeout: const Duration(seconds: 20),
    ));
    final client = ApiClient._(dio)
      .._token = initialToken
      .._onTokenRefreshed = onTokenRefreshed
      .._onUnauthorized = onUnauthorized;
    await client._configureAdapter(allowSelfSigned);
    client._installInterceptors();
    return client;
  }

  static String _normalize(String url) =>
      url.endsWith('/') ? url.substring(0, url.length - 1) : url;

  Future<void> _configureAdapter(bool allowSelfSigned) async {
    final adapter = dio.httpClientAdapter as IOHttpClientAdapter;
    adapter.createHttpClient = () {
      final http = HttpClient();
      if (allowSelfSigned) {
        http.badCertificateCallback = (_, _, _) => true;
      }
      return http;
    };
  }

  void _installInterceptors() {
    dio.interceptors.add(InterceptorsWrapper(
      onRequest: (options, handler) {
        final t = _token;
        if (t != null && t.isNotEmpty) {
          options.headers['Authorization'] = 'Bearer $t';
        }
        handler.next(options);
      },
      onError: (e, handler) async {
        if (e.response?.statusCode != 401) return handler.next(e);
        // 刷新请求自身的 401 不再触发刷新（否则无限递归），直接交给 onUnauthorized。
        if (e.requestOptions.extra['skipRefresh'] == true) {
          await _onUnauthorized?.call();
          return handler.next(e);
        }
        // 401 → 尝试刷新一次并重放；刷新失败视为登出。
        try {
          final resp = await dio.post('/api/auth/refresh',
              options: Options(extra: {'skipAuth': true, 'skipRefresh': true}));
          final newToken = resp.data['token'] as String?;
          if (newToken == null || newToken.isEmpty) {
            throw DioException(requestOptions: e.requestOptions);
          }
          _token = newToken;
          _onTokenRefreshed?.call(newToken);
          final opts = Options(
            method: e.requestOptions.method,
            headers: <String, dynamic>{}
              ..addAll(e.requestOptions.headers)
              ..['Authorization'] = 'Bearer $newToken',
          );
          final replay = await dio.request<void>(e.requestOptions.path,
              data: e.requestOptions.data,
              queryParameters: e.requestOptions.queryParameters,
              options: opts);
          return handler.resolve(replay);
        } on DioException catch (_) {
          await _onUnauthorized?.call();
          return handler.next(e);
        } catch (_) {
          await _onUnauthorized?.call();
          return handler.next(e);
        }
      },
    ));
  }

  void updateToken(String? token) => _token = token;

  /// GET /health（引导页连接测试，无需认证）。
  Future<void> health() async {
    await dio.get<String>('/health',
        options: Options(extra: {'skipAuth': true}));
  }
}
