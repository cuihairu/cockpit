# R8/ProGuard 规则（release 开启 minify+shrink 时生效）。
# 目标：dio / xterm / local_auth / flutter_secure_storage 及 Flutter embedding
# 不被裁剪或错误混淆。逐项注明原因，升级依赖后复核。

# ---- Flutter embedding（反射入口）----
-keep class io.flutter.app.** { *; }
-keep class io.flutter.plugin.** { *; }
-keep class io.flutter.util.** { *; }
-keep class io.flutter.view.** { *; }
-keep class io.flutter.** { *; }
-keep class io.flutter.plugins.** { *; }
-dontwarn io.flutter.embedding.**

# GeneratedPluginRegistrant 经反射调用插件注册
-keep class io.flutter.plugins.GeneratedPluginRegistrant { *; }

# ---- dio（拦截器/适配器经泛型反射）----
-keep class io.flutter.plugins.** { *; }
-keepclassmembers class * {
    @retrofit2.http.* <methods>;
}
# dio 自带 consumer 规则；此处兜底其反射用到的 OkHttp/HttpClient 适配器
-keep class okhttp3.** { *; }
-keep interface okhttp3.** { *; }
-dontwarn okhttp3.**
-dontwarn retrofit2.**
-keep class org.apache.http.** { *; }
-dontwarn org.apache.http.**

# ---- flutter_secure_storage（MethodChannel + Android Keystore）----
# Keystore/JCA 经反射按算法名实例化，实现类不可裁剪
-keep class android.security.** { *; }
-keep class java.security.** { *; }
-keep class javax.crypto.** { *; }
-dontwarn javax.crypto.**

# ---- local_auth（FragmentActivity + BiometricPrompt 反射回调）----
-keep class androidx.fragment.app.** { *; }
-keep class androidx.biometric.** { *; }
-dontwarn androidx.biometric.**
-keep class io.flutter.plugins.localauth.** { *; }

# ---- xterm（平台视图嵌入）----
-keep class xterm.** { *; }
-dontwarn xterm.**

# ---- web_socket_channel / dart:io WebSocket（无特殊反射，仅防误删接口）----
-keep class java.net.** { *; }
-keep class javax.net.** { *; }
-dontwarn javax.net.**

# ---- 通用：保留注解与泛型签名（Gson/dio 反射依赖）----
-keepattributes *Annotation*, Signature, InnerClasses, EnclosingMethod
-keepattributes SourceFile, LineNumberTable
-renamesourcefileattribute SourceFile

# 原生方法与 JNI
-keepclasseswithmembernames class * {
    native <methods>;
}
-keepclassmembers class * {
    @android.webkit.JavascriptInterface <methods>;
}
