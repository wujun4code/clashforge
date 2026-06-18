/// Compile-time configuration for subscriptions bundled into this build.
///
/// Inject at build time:
///   flutter build apk \
///     --dart-define="CLASHFORGE_BUNDLED_SUBS=url1,url2" \
///     --dart-define="CLASHFORGE_BUILD_TIME=$(date +%s000)"
///
/// For local dev, create mobile/.bundled_subs.json (gitignored):
///   {
///     "CLASHFORGE_BUNDLED_SUBS": "https://example.com/sub1.yaml",
///     "CLASHFORGE_BUILD_TIME": "1750000000000"
///   }
/// Then build with:
///   flutter build apk --dart-define-from-file=.bundled_subs.json
class BundledSubscriptionsConfig {
  static const _rawUrls = String.fromEnvironment(
    'CLASHFORGE_BUNDLED_SUBS',
    defaultValue: '',
  );

  static const _buildTimeMs = int.fromEnvironment(
    'CLASHFORGE_BUILD_TIME',
    defaultValue: 0,
  );

  /// Returns the list of subscription URLs to auto-import on first launch.
  /// Empty when no URLs were injected at build time.
  static List<String> get urls =>
      _rawUrls.isEmpty
          ? []
          : _rawUrls
              .split(',')
              .map((u) => u.trim())
              .where((u) => u.isNotEmpty)
              .toList();

  /// The build timestamp injected at compile time.
  /// Falls back to [DateTime.now] when CLASHFORGE_BUILD_TIME was not set.
  static DateTime get buildTime => _buildTimeMs > 0
      ? DateTime.fromMillisecondsSinceEpoch(_buildTimeMs)
      : DateTime.now();
}
