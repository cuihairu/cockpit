import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';

import 'package:cockpit_mobile/theme/app_theme.dart';

void main() {
  test('皮肤预设：四套齐备、key 唯一、明暗两档色板非空', () {
    expect(appSkins.length >= 3, isTrue);
    expect(appSkins.map((s) => s.key).toSet().length, appSkins.length);
    for (final skin in appSkins) {
      expect(skin.name, isNotEmpty);
      expect(skin.description, isNotEmpty);
      for (final palette in [skin.light, skin.dark]) {
        expect(palette.bg, isNot(palette.surface),
            reason: '${skin.key} 的底色与面板色不应同值');
        expect(palette.text, isNot(palette.bg));
      }
    }
  });

  test('resolveAppSkin：命中 / 非法 / 空值回退默认', () {
    expect(resolveAppSkin('eyecare').key, 'eyecare');
    expect(resolveAppSkin('neon').key, defaultAppSkin);
    expect(resolveAppSkin(null).key, defaultAppSkin);
  });

  test('themeModeOf：system / light / dark 映射，未知回退跟随系统', () {
    expect(themeModeOf('light'), ThemeMode.light);
    expect(themeModeOf('dark'), ThemeMode.dark);
    expect(themeModeOf('system'), ThemeMode.system);
    expect(themeModeOf('sepia'), ThemeMode.system);
    expect(themeModeOf(null), ThemeMode.system);
  });

  test('buildAppTheme：浅色档取亮色板，深色档取暗色板', () {
    final skin = resolveAppSkin('midnight');
    final light = buildAppTheme(skin, Brightness.light);
    final dark = buildAppTheme(skin, Brightness.dark);

    expect(light.brightness, Brightness.light);
    expect(light.colorScheme.surface, skin.light.surface);
    expect(light.scaffoldBackgroundColor, skin.light.bg);
    expect(light.appBarTheme.backgroundColor, skin.light.surface);
    expect(light.appBarTheme.systemOverlayStyle?.statusBarIconBrightness,
        Brightness.dark);

    expect(dark.brightness, Brightness.dark);
    expect(dark.colorScheme.surface, skin.dark.surface);
    expect(dark.scaffoldBackgroundColor, skin.dark.bg);
    expect(dark.appBarTheme.backgroundColor, skin.dark.surface);
    expect(dark.appBarTheme.systemOverlayStyle?.statusBarIconBrightness,
        Brightness.light);
  });

  test('buildAppTheme：强调色恒为 Web 默认蔷薇红，且随皮肤切换即时改底色', () {
    for (final skin in appSkins) {
      final theme = buildAppTheme(skin, Brightness.light);
      expect(theme.colorScheme.primary, cockpitAccent);
      expect(theme.navigationBarTheme.backgroundColor, skin.light.surface);
      expect(theme.dividerTheme.color, skin.light.borderSoft);
    }
    // 换皮肤 → 页面底色跟着换（整套切换而非只改强调色）
    final a = buildAppTheme(resolveAppSkin('default'), Brightness.light);
    final b = buildAppTheme(resolveAppSkin('graphite'), Brightness.light);
    expect(a.scaffoldBackgroundColor, isNot(b.scaffoldBackgroundColor));
    expect(a.colorScheme.primary, b.colorScheme.primary);
  });
}