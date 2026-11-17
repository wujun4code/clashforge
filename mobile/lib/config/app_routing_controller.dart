/// Transaction outcome shared by the settings sheet and VPN restart caller.
class AppRoutingResult {
  const AppRoutingResult({required this.success, required this.message});
  final bool success;
  final String message;
}

class AppRoutingSetting {
  AppRoutingSetting(this.mode, Iterable<String> packages)
      : packages = Set.unmodifiable(packages);
  final String mode;
  final Set<String> packages;
}

/// Keep the displayed setting unchanged until native, persistence and restart
/// have all accepted it. A failed apply restores the next-connect setting.
class AppRoutingController {
  AppRoutingController({
    required this.setNative,
    required this.persist,
    required this.restart,
  });
  final Future<String> Function(AppRoutingSetting) setNative;
  final Future<void> Function(AppRoutingSetting) persist;
  final Future<AppRoutingResult> Function() restart;
  bool _busy = false;

  Future<AppRoutingResult> apply({
    required AppRoutingSetting previous,
    required AppRoutingSetting desired,
    required bool connected,
    String? preflightError,
  }) async {
    if (_busy) {
      return const AppRoutingResult(success: false, message: '正在应用设置');
    }
    if (desired.mode == 'allowlist' && desired.packages.isEmpty) {
      return const AppRoutingResult(success: false, message: '请选择至少一个应用');
    }
    if (connected && preflightError != null) {
      return AppRoutingResult(success: false, message: preflightError);
    }
    _busy = true;
    try {
      final native = await setNative(desired);
      if (native.toLowerCase().startsWith('error')) throw StateError(native);
      await persist(desired);
      if (connected) {
        final result = await restart();
        if (!result.success) throw StateError(result.message);
      }
      return AppRoutingResult(
        success: true,
        message: connected ? '分应用设置已生效' : '已保存，连接后生效',
      );
    } catch (error) {
      final rollbackErrors = <String>[];
      try {
        final result = await setNative(previous);
        if (result.toLowerCase().startsWith('error')) {
          rollbackErrors.add(result);
        }
      } catch (error) {
        rollbackErrors.add(error.toString());
      }
      try {
        await persist(previous);
      } catch (error) {
        rollbackErrors.add(error.toString());
      }
      return AppRoutingResult(
        success: false,
        message: '应用失败：$error'
            '${rollbackErrors.isEmpty ? '' : '；恢复旧设置失败：${rollbackErrors.join('; ')}'}',
      );
    } finally {
      _busy = false;
    }
  }
}
