import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

/// 主题皮肤（移动端口径与 Web 端 web/src/theme/themeSkins.ts 一致）：
/// 同名、同 key、同色值，四套皮肤 × 明暗两档。移动端只调中性色板，
/// 强调色沿用 [cockpitAccent]（= Web 默认主题色「蔷薇红」）；
/// Web 端强调色可独立切换，移动端暂不支持自定义，两端口径差异见
/// docs/ui-theme-design.md。
const Color cockpitAccent = Color(0xFFE11D8F);

/// 单个皮肤在中/深一档下的色板。
class SkinPalette {
  const SkinPalette({
    required this.bg,
    required this.surface,
    required this.surfaceAlt,
    required this.border,
    required this.borderSoft,
    required this.text,
    required this.textMuted,
  });

  /// 页面底色
  final Color bg;

  /// 卡片/面板面
  final Color surface;

  /// 次级面
  final Color surfaceAlt;

  /// 主边框
  final Color border;

  /// 更弱的分割线
  final Color borderSoft;

  /// 主文字
  final Color text;

  /// 次级文字
  final Color textMuted;
}

class AppSkin {
  const AppSkin({
    required this.key,
    required this.name,
    required this.description,
    required this.light,
    required this.dark,
  });

  final String key;
  final String name;
  final String description;
  final SkinPalette light;
  final SkinPalette dark;

  SkinPalette paletteFor(Brightness brightness) =>
      brightness == Brightness.dark ? dark : light;
}

const List<AppSkin> appSkins = [
  AppSkin(
    key: 'default',
    name: '默认 · 雾白',
    description: '冷中性灰，历史默认观感',
    light: SkinPalette(
      bg: Color(0xFFF8FAFC),
      surface: Color(0xFFFFFFFF),
      surfaceAlt: Color(0xFFFFFFFF),
      border: Color(0xFFE2E8F0),
      borderSoft: Color(0xFFE2E8F0),
      text: Color(0xFF0F172A),
      textMuted: Color(0xFF64748B),
    ),
    dark: SkinPalette(
      bg: Color(0xFF0C0E14),
      surface: Color(0xFF141620),
      surfaceAlt: Color(0xFF141620),
      border: Color(0x0FFFFFFF),
      borderSoft: Color(0x0AFFFFFF),
      text: Color(0xFFE2E8F0),
      textMuted: Color(0xFF94A3B8),
    ),
  ),
  AppSkin(
    key: 'midnight',
    name: '深邃 · 夜航',
    description: '靛蓝夜幕，运维台气质',
    light: SkinPalette(
      bg: Color(0xFFF2F5FA),
      surface: Color(0xFFFFFFFF),
      surfaceAlt: Color(0xFFF7F9FC),
      border: Color(0xFFDBE3EE),
      borderSoft: Color(0xFFE8EDF5),
      text: Color(0xFF16233B),
      textMuted: Color(0xFF5F7192),
    ),
    dark: SkinPalette(
      bg: Color(0xFF070D1A),
      surface: Color(0xFF0E1729),
      surfaceAlt: Color(0xFF101B30),
      border: Color(0x2494B4FF),
      borderSoft: Color(0x1794B4FF),
      text: Color(0xFFDBE7FF),
      textMuted: Color(0xFF8296BD),
    ),
  ),
  AppSkin(
    key: 'eyecare',
    name: '护眼 · 米纸',
    description: '暖米纸色，久看不刺眼',
    light: SkinPalette(
      bg: Color(0xFFF4EFE4),
      surface: Color(0xFFFBF7EE),
      surfaceAlt: Color(0xFFF7F2E7),
      border: Color(0xFFDED2BA),
      borderSoft: Color(0xFFE7DDC9),
      text: Color(0xFF3B342A),
      textMuted: Color(0xFF7B6F5B),
    ),
    dark: SkinPalette(
      bg: Color(0xFF1A1712),
      surface: Color(0xFF241F19),
      surfaceAlt: Color(0xFF272119),
      border: Color(0x2EE2CDA8),
      borderSoft: Color(0x1CE2CDA8),
      text: Color(0xFFE8DFCD),
      textMuted: Color(0xFFA3937A),
    ),
  ),
  AppSkin(
    key: 'graphite',
    name: '石墨 · 曜岩',
    description: '零色相纯灰，最保守的底色',
    light: SkinPalette(
      bg: Color(0xFFF4F4F5),
      surface: Color(0xFFFFFFFF),
      surfaceAlt: Color(0xFFFAFAFA),
      border: Color(0xFFE4E4E7),
      borderSoft: Color(0xFFEDEDF0),
      text: Color(0xFF18181B),
      textMuted: Color(0xFF71717A),
    ),
    dark: SkinPalette(
      bg: Color(0xFF131313),
      surface: Color(0xFF1C1C1E),
      surfaceAlt: Color(0xFF1F1F21),
      border: Color(0x1AFFFFFF),
      borderSoft: Color(0x0FFFFFFF),
      text: Color(0xFFEDEDF0),
      textMuted: Color(0xFF9B9BA3),
    ),
  ),
];

const String defaultAppSkin = 'default';

/// 非法 skin key 回退默认皮肤（与 Web 端 resolveThemeSkin 同策略）。
AppSkin resolveAppSkin(String? key) => appSkins.firstWhere(
      (s) => s.key == key,
      orElse: () => appSkins.first,
    );

const Map<String, String> themeModeLabels = {
  'system': '跟随系统',
  'light': '浅色',
  'dark': '深色',
};

/// 非法/未知模式回退跟随系统。
ThemeMode themeModeOf(String? mode) => switch (mode) {
      'light' => ThemeMode.light,
      'dark' => ThemeMode.dark,
      _ => ThemeMode.system,
    };

/// 按皮肤 + 亮度生成 ThemeData。色板取自 [AppSkin]，
/// 强调色固定 [cockpitAccent]（与 Web 默认主题色一致）。
ThemeData buildAppTheme(AppSkin skin, Brightness brightness) {
  final p = skin.paletteFor(brightness);
  final dark = brightness == Brightness.dark;
  final scheme = ColorScheme.fromSeed(
    seedColor: cockpitAccent,
    brightness: brightness,
  ).copyWith(
    // fromSeed 会把主色映射到 tonal 变体，这里显式钉回品牌色相，
    // 保证两端强调色一致（Web 端 colorPrimary 即该值）
    primary: cockpitAccent,
    onPrimary: Colors.white,
    secondary: cockpitAccent,
    onSecondary: Colors.white,
    surface: p.surface,
    onSurface: p.text,
    onSurfaceVariant: p.textMuted,
    surfaceContainerHighest: p.surfaceAlt,
    outline: p.border,
    outlineVariant: p.borderSoft,
  );
  return ThemeData(
    colorScheme: scheme,
    scaffoldBackgroundColor: p.bg,
    appBarTheme: AppBarTheme(
      backgroundColor: p.surface,
      foregroundColor: p.text,
      // 浅色：深色状态栏图标；深色：浅色状态栏图标
      systemOverlayStyle: SystemUiOverlayStyle(
        statusBarColor: Colors.transparent,
        statusBarIconBrightness: dark ? Brightness.light : Brightness.dark,
        statusBarBrightness: dark ? Brightness.dark : Brightness.light,
        systemNavigationBarColor: p.surface,
        systemNavigationBarIconBrightness:
            dark ? Brightness.light : Brightness.dark,
      ),
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: p.surface,
      indicatorColor: cockpitAccent.withValues(alpha: 0.16),
    ),
    dividerTheme: DividerThemeData(color: p.borderSoft, thickness: 1),
    listTileTheme: ListTileThemeData(
      iconColor: p.textMuted,
      textColor: p.text,
    ),
  );
}