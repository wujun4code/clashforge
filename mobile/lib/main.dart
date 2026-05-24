import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:pointycastle/export.dart' as pc;
import 'package:shared_preferences/shared_preferences.dart';
import 'package:url_launcher/url_launcher.dart';
import 'subscription/parsed_subscription.dart';
import 'subscription/subscription_parser.dart';
import 'subscription/subscription_store.dart';
import 'subscription/proxy_node.dart';
import 'config/vpn_manager.dart';
import 'config/config_generator.dart';
import 'config/free_node_config.dart';
import 'logger/app_logger.dart';
import 'logger/log_entry.dart';
import 'update_checker.dart';
import 'l10n/app_localizations.dart';

Future<void> _launchUrl(String url) async {
  final uri = Uri.parse(url);
  if (await canLaunchUrl(uri)) {
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  // Cap Flutter's decoded-image cache at 20 MB (default: 100 MB).
  PaintingBinding.instance.imageCache.maximumSizeBytes = 20 << 20;
  runApp(const ClashForgeApp());
}

// ─── Brand tokens ─────────────────────────────────────────────
// Pulled from ClashForge Nebula theme (web UI / favicon.svg)
const _kBg = Color(0xFF0B0A1A); // deep indigo-black
const _kSurface = Color(0xFF13112B); // nav bar / surface
const _kCard = Color(0xFF1A1830); // card bg
const _kCardGrad = Color(0xFF201D3A); // card gradient end
const _kBorder = Color(0xFF2A2750); // card border
const _kBrand = Color(0xFF863BFF); // violet — primary brand
const _kConnected = Color(0xFF22D3EE); // neon cyan — VPN on
const _kError = Color(0xFFEF5350);
const _kTextHi = Color(0xFFEEEBFF); // lavender-white
const _kTextMuted = Color(0xFF9B8CC4); // muted lavender
const _kTextFaint = Color(0xFF4A4570); // very faint

const _kClashControllerHost = '127.0.0.1';
const _kClashControllerPort = 9090;
const _kHttpProxyPort = 7890;
const _kMainProxyGroup = '🚀 Proxy';

class _IpInfo {
  const _IpInfo({required this.ip, required this.location, required this.source});
  final String ip;
  final String location;
  final String source;
}

class _SiteCheckResult {
  const _SiteCheckResult({
    required this.name,
    required this.description,
    required this.ok,
    this.latencyMs,
    this.dnsLabel,
    this.error,
  });
  final String name;
  final String description;
  final bool ok;
  final int? latencyMs;
  final String? dnsLabel;
  final String? error;
}

class _ConnectivitySnapshot {
  const _ConnectivitySnapshot({
    required this.checkedAt,
    required this.directIpResults,
    required this.proxyIpResults,
    required this.domesticResults,
    required this.foreignResults,
    required this.aiResults,
  });
  final DateTime checkedAt;
  final List<_IpInfo> directIpResults;
  final List<_IpInfo> proxyIpResults;
  final List<_SiteCheckResult> domesticResults;
  final List<_SiteCheckResult> foreignResults;
  final List<_SiteCheckResult> aiResults;
}

class _BrowserDnsCheckResult {
  const _BrowserDnsCheckResult({
    required this.name,
    required this.ok,
    required this.detail,
  });

  final String name;
  final bool ok;
  final String detail;
}

class _BrowserDnsSnapshot {
  const _BrowserDnsSnapshot({
    required this.checkedAt,
    required this.checks,
    required this.summary,
  });

  final DateTime checkedAt;
  final List<_BrowserDnsCheckResult> checks;
  final String summary;

  bool get healthy => checks.isNotEmpty && checks.every((item) => item.ok);
}

// ─── Minimal YAML serialiser ──────────────────────────────────
String _mapToYaml(Map<String, dynamic> m) {
  final buf = StringBuffer();
  _writeYamlMap(buf, m, 0);
  return buf.toString();
}

void _writeYamlMap(StringBuffer buf, Map<String, dynamic> m, int depth) {
  final pad = '  ' * depth;
  for (final e in m.entries) {
    final v = e.value;
    if (v is Map<String, dynamic>) {
      buf.writeln('$pad${e.key}:');
      _writeYamlMap(buf, v, depth + 1);
    } else if (v is List) {
      buf.writeln('$pad${e.key}:');
      _writeYamlList(buf, v, depth + 1);
    } else {
      buf.writeln('$pad${e.key}: ${_yamlScalar(v)}');
    }
  }
}

void _writeYamlList(StringBuffer buf, List<dynamic> list, int depth) {
  final pad = '  ' * depth;
  for (final item in list) {
    if (item is Map<String, dynamic>) {
      var first = true;
      for (final e in item.entries) {
        final prefix = first ? '$pad- ' : '$pad  ';
        final v = e.value;
        if (v is List) {
          buf.writeln('$prefix${e.key}:');
          _writeYamlList(buf, v, depth + 1);
        } else if (v is Map<String, dynamic>) {
          buf.writeln('$prefix${e.key}:');
          _writeYamlMap(buf, v, depth + 2);
        } else {
          buf.writeln('$prefix${e.key}: ${_yamlScalar(v)}');
        }
        first = false;
      }
    } else {
      buf.writeln('$pad- ${_yamlScalar(item)}');
    }
  }
}

String _yamlScalar(dynamic v) {
  if (v is bool || v is int || v is double) return v.toString();
  final s = v.toString();
  if (s.isEmpty ||
      s.contains(':') ||
      s.contains('#') ||
      s.contains("'") ||
      s.startsWith('*') ||
      s.startsWith('!') ||
      s == 'true' ||
      s == 'false' ||
      s == 'null') {
    return '"${s.replaceAll('\\', '\\\\').replaceAll('"', '\\"')}"';
  }
  return s;
}

// ─── App root ─────────────────────────────────────────────────
class ClashForgeApp extends StatefulWidget {
  const ClashForgeApp({super.key});

  @override
  State<ClashForgeApp> createState() => _ClashForgeAppState();
}

class _ClashForgeAppState extends State<ClashForgeApp> {
  Locale? _locale;

  static const _kLocaleKey = 'app_locale';

  @override
  void initState() {
    super.initState();
    _loadLocale();
  }

  Future<void> _loadLocale() async {
    final prefs = await SharedPreferences.getInstance();
    final code = prefs.getString(_kLocaleKey);
    if (code != null && mounted) {
      setState(() => _locale = Locale(code));
    }
  }

  void _setLocale(Locale locale) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_kLocaleKey, locale.languageCode);
    if (mounted) setState(() => _locale = locale);
  }

  @override
  Widget build(BuildContext context) {
    const seed = _kBrand;
    final scheme =
        ColorScheme.fromSeed(seedColor: seed, brightness: Brightness.dark)
            .copyWith(
      primary: seed,
      onPrimary: Colors.white,
      secondary: _kConnected,
      surface: _kSurface,
      onSurface: _kTextHi,
      surfaceContainerHighest: _kCard,
      secondaryContainer: seed.withAlpha(40),
      onSecondaryContainer: seed,
    );

    return MaterialApp(
      title: 'ClashForge',
      debugShowCheckedModeBanner: false,
      locale: _locale,
      localizationsDelegates: AppLocalizations.localizationsDelegates,
      supportedLocales: AppLocalizations.supportedLocales,
      theme: ThemeData(
        colorScheme: scheme,
        useMaterial3: true,
        scaffoldBackgroundColor: _kBg,
        cardTheme: CardThemeData(
          color: _kCard,
          elevation: 0,
          shape:
              RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        ),
        navigationBarTheme: NavigationBarThemeData(
          backgroundColor: _kSurface,
          surfaceTintColor: Colors.transparent,
          shadowColor: Colors.transparent,
          indicatorColor: seed.withAlpha(35),
        ),
        dividerTheme: const DividerThemeData(color: _kBorder, space: 1),
        textTheme: const TextTheme(
          bodyMedium: TextStyle(color: _kTextMuted),
        ),
        dialogTheme: const DialogThemeData(
          backgroundColor: _kCard,
          surfaceTintColor: Colors.transparent,
          titleTextStyle: TextStyle(
              color: _kTextHi, fontSize: 17, fontWeight: FontWeight.w600),
        ),
        snackBarTheme: SnackBarThemeData(
          backgroundColor: _kCardGrad,
          contentTextStyle: const TextStyle(color: _kTextHi),
          shape:
              RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
          behavior: SnackBarBehavior.floating,
        ),
      ),
      home: HomeScreen(onLocaleChanged: _setLocale),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Root screen — shared state
// ─────────────────────────────────────────────────────────────
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key, required this.onLocaleChanged});
  final void Function(Locale) onLocaleChanged;
  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  static const _logChannel = EventChannel('com.clashforge.mobile/logs');
  // (url, source-label, parse-type)  parse-type: 'upaiyun' | 'ipsb' | 'ipinfo'
  static const _directIpCandidates = [
    ('https://pubstatic.b0.upaiyun.com/?_upnode', 'UpaiYun', 'upaiyun'),
    ('https://api.ip.sb/geoip',                   'IP.SB',   'ipsb'),
    ('https://ipinfo.io/json',                     'IPInfo',  'ipinfo'),
  ];
  static const _proxyIpCandidates = [
    ('https://api.ip.sb/geoip',  'IP.SB',  'ipsb'),
    ('https://ipinfo.io/json',   'IPInfo', 'ipinfo'),
  ];

  // (url, domain-for-DNS-query) — names and descriptions come from l10n
  static const _domesticSiteUrls = [
    ('https://www.taobao.com', 'www.taobao.com'),
    ('https://music.163.com', 'music.163.com'),
  ];
  static const _foreignSiteUrls = [
    ('https://github.com', 'github.com'),
    ('https://www.google.com', 'www.google.com'),
  ];
  static const _aiSiteUrls = [
    ('https://chat.openai.com', 'chat.openai.com'),
    ('https://claude.ai', 'claude.ai'),
  ];

  List<(String, String, String, String)> _domesticSites(AppLocalizations l10n) => [
    (l10n.siteTaobaoName, l10n.siteTaobaoDesc, _domesticSiteUrls[0].$1, _domesticSiteUrls[0].$2),
    (l10n.siteNeteaseName, l10n.siteNeteaseDesc, _domesticSiteUrls[1].$1, _domesticSiteUrls[1].$2),
  ];
  List<(String, String, String, String)> _foreignSites(AppLocalizations l10n) => [
    ('GitHub', l10n.siteGitHubDesc, _foreignSiteUrls[0].$1, _foreignSiteUrls[0].$2),
    ('Google', l10n.siteGoogleDesc, _foreignSiteUrls[1].$1, _foreignSiteUrls[1].$2),
  ];
  List<(String, String, String, String)> _aiSites(AppLocalizations l10n) => [
    ('OpenAI', l10n.siteOpenAIDesc, _aiSiteUrls[0].$1, _aiSiteUrls[0].$2),
    ('Claude', l10n.siteClaudeDesc, _aiSiteUrls[1].$1, _aiSiteUrls[1].$2),
  ];

  int _tabIndex = 0;
  bool _isConnected = false;
  bool _isConnecting = false;
  bool _probeLoading = false;
  bool _browserDnsLoading = false;
  bool _switchingNode = false;
  String _connectionStatus = '';
  String? _probeMessage;
  String? _browserDnsMessage;
  String? _privateDnsWarning;
  _ConnectivitySnapshot? _connectivitySnapshot;
  _BrowserDnsSnapshot? _browserDnsSnapshot;
  final List<ProxyNode> _nodes = [];
  ProxyNode? _selectedNode;
  Map<String, String> _proxyNowMap = {};
  final List<Subscription> _subscriptions = [];
  String? _activeSubscriptionId;

  @override
  void initState() {
    super.initState();
    _logChannel.receiveBroadcastStream().listen(_onNativeLog, onError: (e) {
      AppLogger.instance.error('native', 'EventChannel error: $e');
    });
    AppLogger.instance.info('app', 'ClashForge started');
    _loadPersistedData();
  }

  Future<void> _loadPersistedData() async {
    final subs = await SubscriptionStore.loadSubscriptions();
    final activeId = await SubscriptionStore.loadActiveId();
    if (subs.isNotEmpty) {
      final active = subs.firstWhere(
        (s) => s.id == activeId,
        orElse: () => subs.first,
      );
      setState(() {
        _subscriptions.addAll(subs);
        _activeSubscriptionId = active.id;
        _nodes.addAll(active.nodes);
        _selectedNode = active.nodes.isEmpty ? null : active.nodes.first;
      });
      AppLogger.instance.info('app', 'Loaded subscriptions',
          fields: {'count': subs.length, 'active': active.nickname});
    }
    // Always ensure the builtin subscription is present, regardless of
    // whether other subscriptions exist. Skips silently if already imported.
    await _ensureBuiltinSub();
  }

  Future<void> _ensureBuiltinSub() async {
    final url = FreeNodeConfig.subscriptionUrl;
    if (url == null) return;

    // Migration: users upgrading from pre-rc75 have the Free subscription saved
    // without isBuiltIn=true, so the UI still shows server/port/protocol.
    // Retroactively set the flag and persist without re-fetching.
    final existingIdx = _subscriptions.indexWhere((s) => s.url == url);
    if (existingIdx >= 0) {
      final existing = _subscriptions[existingIdx];
      if (!existing.isBuiltIn) {
        final fixed = Subscription(
          id: existing.id,
          nickname: existing.nickname,
          url: existing.url,
          nodes: existing.nodes,
          customRules: existing.customRules,
          customProxyGroups: existing.customProxyGroups,
          customRuleProviders: existing.customRuleProviders,
          isBuiltIn: true,
        );
        setState(() => _subscriptions[existingIdx] = fixed);
        await SubscriptionStore.saveSubscriptions(List.of(_subscriptions));
        AppLogger.instance.info('app', 'Retroactively marked built-in subscription');
      }
      return;
    }

    AppLogger.instance.info('app', 'Importing built-in subscription');
    try {
      final response =
          await http.get(Uri.parse(url)).timeout(const Duration(seconds: 20));
      if (response.statusCode != 200) return;
      final content = _decryptResponseOrFallback(response.bodyBytes, response.body);
      final parsed = SubscriptionParser.parse(content);
      if (parsed.proxies.isEmpty) return;
      _onNodesImported(parsed, url: url, nickname: 'Free', isBuiltIn: true);
      AppLogger.instance.info('app', 'Built-in subscription imported',
          fields: {'nodes': parsed.proxies.length});
    } catch (e) {
      AppLogger.instance.warn('app', 'Built-in subscription import failed',
          fields: {'error': e.toString()});
    }
  }

  // Try AES-256-GCM decryption (nonce=12, tag=16) with the hex-decoded key.
  // Falls back to the raw body string if the key is absent or decryption fails.
  String _decryptResponseOrFallback(Uint8List bytes, String fallback) {
    final key = FreeNodeConfig.rawKeyBytes;
    if (key == null || bytes.length < 29) return fallback;
    try {
      final nonce = bytes.sublist(0, 12);
      final cipherAndTag = bytes.sublist(12);
      final cipher = pc.GCMBlockCipher(pc.AESEngine());
      cipher.init(false,
          pc.AEADParameters(pc.KeyParameter(key), 128, nonce, Uint8List(0)));
      final plain = cipher.process(cipherAndTag);
      return String.fromCharCodes(plain);
    } catch (_) {
      return fallback;
    }
  }

  void _onNativeLog(dynamic event) {
    try {
      final map = json.decode(event as String) as Map<String, dynamic>;
      final level = map['level'] as String? ?? 'info';
      final component = map['component'] as String? ?? 'native';
      final message = map['message'] as String? ?? '';
      final fields = (map['fields'] as Map<String, dynamic>?) ?? {};
      AppLogger.instance.log(level, component, message, fields: fields);
    } catch (_) {
      AppLogger.instance.debug('native', event.toString());
    }
  }

  void _onNodesImported(ParsedSubscription parsed,
      {String url = '', String nickname = '', bool isBuiltIn = false}) {
    final id = '${DateTime.now().millisecondsSinceEpoch}';
    final sub = Subscription(
      id: id,
      nickname: nickname,
      url: url,
      nodes: parsed.proxies,
      customRules: parsed.rules,
      customProxyGroups: parsed.proxyGroups,
      customRuleProviders: parsed.ruleProviders,
      isBuiltIn: isBuiltIn,
    );

    final previousSelected = _selectedNode?.name;
    ProxyNode? nextSelected;
    if (previousSelected != null) {
      for (final item in parsed.proxies) {
        if (item.name == previousSelected) {
          nextSelected = item;
          break;
        }
      }
    }
    nextSelected ??= parsed.proxies.isEmpty ? null : parsed.proxies.first;

    setState(() {
      _subscriptions.add(sub);
      _activeSubscriptionId = id;
      _nodes
        ..clear()
        ..addAll(parsed.proxies);
      _selectedNode = nextSelected;
    });
    SubscriptionStore.saveSubscriptions(List.of(_subscriptions));
    SubscriptionStore.saveActiveId(id);
  }

  void _activateSubscription(Subscription sub) {
    final previousSelected = _selectedNode?.name;
    ProxyNode? nextSelected;
    if (previousSelected != null) {
      for (final item in sub.nodes) {
        if (item.name == previousSelected) {
          nextSelected = item;
          break;
        }
      }
    }
    nextSelected ??= sub.nodes.isEmpty ? null : sub.nodes.first;

    setState(() {
      _activeSubscriptionId = sub.id;
      _nodes
        ..clear()
        ..addAll(sub.nodes);
      _selectedNode = nextSelected;
    });
    SubscriptionStore.saveActiveId(sub.id);
    unawaited(_applyNodeSelectionIfRunning(triggerProbe: false));
  }

  void _deleteSubscription(Subscription sub) {
    final wasActive = _activeSubscriptionId == sub.id;
    setState(() {
      _subscriptions.removeWhere((s) => s.id == sub.id);
    });
    SubscriptionStore.saveSubscriptions(List.of(_subscriptions));
    if (wasActive) {
      if (_subscriptions.isNotEmpty) {
        _activateSubscription(_subscriptions.first);
      } else {
        setState(() {
          _activeSubscriptionId = null;
          _nodes.clear();
          _selectedNode = null;
        });
        SubscriptionStore.saveActiveId('');
      }
    }
  }

  Future<void> _onNodeSelected(ProxyNode node) async {
    setState(() {
      _selectedNode = node;
      _tabIndex = 0;
    });
    await _applyNodeSelectionIfRunning(triggerProbe: true);
  }

  Future<void> _toggleVpn() async {
    if (_isConnecting) return;
    setState(() => _isConnecting = true);
    final logger = AppLogger.instance;
    try {
      if (_isConnected) {
        logger.info('vpn', 'Stopping VPN');
        await VpnManager.stopVpn();
        logger.info('vpn', 'VPN stopped');
        setState(() {
          _isConnected = false;
          _connectionStatus = '';
          _connectivitySnapshot = null;
          _browserDnsSnapshot = null;
          _probeMessage = null;
          _browserDnsMessage = null;
          _privateDnsWarning = null;
        });
      } else {
        if (_selectedNode == null) {
          logger.warn('vpn', 'No node selected');
          setState(() => _connectionStatus = 'Select a node first');
          return;
        }
        Subscription? activeSub;
        for (final s in _subscriptions) {
          if (s.id == _activeSubscriptionId) { activeSub = s; break; }
        }
        // Never log server/port/type for built-in nodes — they must stay hidden.
        logger.info('vpn', 'Starting VPN', fields: {
          'node': _selectedNode!.name,
          if (!(activeSub?.isBuiltIn ?? false)) ...{
            'type': _selectedNode!.type,
            'server': _selectedNode!.server,
            'port': _selectedNode!.port,
          },
        });
        final filesDir = await VpnManager.getFilesDir();
        final configMap = ConfigGenerator.generate(
          nodes: _nodes,
          geodataPath: filesDir,
          selectedNodeName: _selectedNode!.name,
          customRules: activeSub?.customRules ?? const [],
          customProxyGroups: activeSub?.customProxyGroups ?? const [],
          customRuleProviders: activeSub?.customRuleProviders ?? const {},
        );
        final writeResult = await VpnManager.writeConfig(_mapToYaml(configMap));
        logger.debug('vpn', 'Config write result: $writeResult');

        final res = await VpnManager.startVpn();
        logger.info('vpn', 'VPN start result: $res');
        if (res == 'permission_needed') {
          if (!mounted) return;
          final l10n = AppLocalizations.of(context);
          setState(() => _connectionStatus = l10n.connGrantPermission);
        } else {
          if (!mounted) return;
          final l10n = AppLocalizations.of(context);
          setState(() {
            _isConnected = true;
            _connectionStatus = l10n.connConnected;
            _probeMessage = null;
          });
          unawaited(_bootstrapAfterConnect());
        }
      }
    } catch (e, st) {
      logger.error('vpn', 'Toggle failed: $e',
          fields: {'stack': st.toString().split('\n').first});
      if (!mounted) return;
      final l10n = AppLocalizations.of(context);
      setState(() {
        _isConnected = false;
        _connectionStatus = l10n.connError(e.toString());
      });
    } finally {
      setState(() => _isConnecting = false);
    }
  }

  // Discover which Selector group contains [nodeName].
  // Tries well-known group names first, then falls back to scanning all Selectors.
  Future<String?> _resolveProxyGroup(String nodeName) async {
    try {
      final res = await http
          .get(Uri.parse(
              'http://$_kClashControllerHost:$_kClashControllerPort/proxies'))
          .timeout(const Duration(seconds: 3));
      if (res.statusCode != 200) return null;
      final proxies =
          ((jsonDecode(res.body) as Map<String, dynamic>)['proxies']
              as Map<String, dynamic>?) ??
              {};
      const preferred = ['🚀 Proxy', '🚀 节点选择', 'Proxy', 'GLOBAL'];
      for (final name in preferred) {
        final g = proxies[name] as Map<String, dynamic>?;
        if (g == null || g['type'] != 'Selector') continue;
        final all = (g['all'] as List?)?.cast<String>() ?? [];
        if (all.contains(nodeName)) return name;
      }
      for (final entry in proxies.entries) {
        final g = entry.value as Map<String, dynamic>? ?? {};
        if (g['type'] != 'Selector') continue;
        final all = (g['all'] as List?)?.cast<String>() ?? [];
        if (all.contains(nodeName)) return entry.key;
      }
    } catch (_) {}
    return null;
  }

  Future<void> _refreshProxyNow() async {
    try {
      final res = await http
          .get(Uri.parse(
              'http://$_kClashControllerHost:$_kClashControllerPort/proxies'))
          .timeout(const Duration(seconds: 3));
      if (res.statusCode != 200 || !mounted) return;
      final data =
          ((jsonDecode(res.body) as Map<String, dynamic>)['proxies']
              as Map<String, dynamic>?) ??
              {};
      final map = <String, String>{};
      for (final e in data.entries) {
        final now =
            (e.value as Map<String, dynamic>?)?['now'] as String?;
        if (now != null && now.isNotEmpty) map[e.key] = now;
      }
      if (mounted) setState(() => _proxyNowMap = map);
    } catch (_) {}
  }

  Future<void> _switchGroupMember(String group, String member) async {
    setState(() => _proxyNowMap = {..._proxyNowMap, group: member});
    if (!_isConnected) return;
    final uri = Uri.parse(
      'http://$_kClashControllerHost:$_kClashControllerPort/proxies/'
      '${Uri.encodeComponent(group)}',
    );
    try {
      await http
          .put(uri,
              headers: {'Content-Type': 'application/json'},
              body: jsonEncode({'name': member}))
          .timeout(const Duration(seconds: 2));
    } catch (_) {}
    await _refreshProxyNow();
  }


  Future<void> _bootstrapAfterConnect() async {
    await _refreshPrivateDnsWarning();

    // Retry applying node selection until mihomo's controller is ready.
    // The controller typically starts within 1–2 s; 8 × 500 ms = up to 4 s.
    for (var attempt = 0; attempt < 8; attempt++) {
      await Future.delayed(const Duration(milliseconds: 500));
      if (!_isConnected || !mounted) return;
      try {
        final nodeName = _selectedNode?.name ?? '';
        final group = nodeName.isNotEmpty
            ? await _resolveProxyGroup(nodeName) ?? _kMainProxyGroup
            : _kMainProxyGroup;
        final uri = Uri.parse(
          'http://$_kClashControllerHost:$_kClashControllerPort/proxies/'
          '${Uri.encodeComponent(group)}',
        );
        await http
            .put(
              uri,
              headers: {'Content-Type': 'application/json'},
              body: jsonEncode({'name': nodeName}),
            )
            .timeout(const Duration(seconds: 2));
        if (mounted) {
          setState(() => _connectionStatus = AppLocalizations.of(context).connConnected);
        }
        break; // controller responded — stop retrying
      } catch (_) {
        // controller not ready yet — keep retrying
      }
    }
    await _runConnectivityChecks();
    await _runBrowserDnsDiagnostics();
    await _refreshProxyNow();
  }

  Future<void> _refreshPrivateDnsWarning() async {
    final info = await VpnManager.getSystemInfo();
    final mode =
        (info['private_dns_mode'] ?? '').toString().trim().toLowerCase();
    final specifier = (info['private_dns_specifier'] ?? '').toString().trim();

    String? nextWarning;
    if (mode == 'hostname' || mode == 'opportunistic') {
      if (!mounted) return;
      final l10n = AppLocalizations.of(context);
      nextWarning = mode == 'hostname' && specifier.isNotEmpty
          ? l10n.privateDnsWarningHostname(specifier)
          : l10n.privateDnsWarningAuto;
    }

    if (!mounted) return;
    if (_privateDnsWarning == nextWarning) return;
    setState(() => _privateDnsWarning = nextWarning);
  }

  Future<void> _applyNodeSelectionIfRunning(
      {required bool triggerProbe}) async {
    if (!_isConnected || _selectedNode == null) return;

    final logger = AppLogger.instance;
    final nodeName = _selectedNode!.name;
    final group = await _resolveProxyGroup(nodeName) ?? _kMainProxyGroup;
    try {
      final uri = Uri.parse(
        'http://$_kClashControllerHost:$_kClashControllerPort/proxies/'
        '${Uri.encodeComponent(group)}',
      );
      final res = await http
          .put(
            uri,
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': nodeName}),
          )
          .timeout(const Duration(seconds: 5));

      if (res.statusCode != 200 && res.statusCode != 204) {
        throw Exception('HTTP ${res.statusCode}');
      }
      logger.info('proxy', 'Applied node selection',
          fields: {'group': group, 'node': nodeName});
      if (triggerProbe) {
        await _runConnectivityChecks();
      }
    } catch (e) {
      logger.warn('proxy', 'Apply node selection failed: $e',
          fields: {'group': group, 'node': nodeName});
      if (mounted) {
        setState(() => _connectionStatus =
            AppLocalizations.of(context).connNodeSwitchPending);
      }
    }
  }

  Future<void> _switchNodeFromHome(ProxyNode node) async {
    if (_switchingNode) return;
    setState(() {
      _selectedNode = node;
      _switchingNode = _isConnected;
    });

    if (!_isConnected) {
      setState(() => _connectionStatus = AppLocalizations.of(context).connNodeSelectedTapConnect);
      return;
    }

    try {
      await _applyNodeSelectionIfRunning(triggerProbe: false);
      if (mounted) {
        setState(() => _connectionStatus = AppLocalizations.of(context).connSwitchedTo(node.name));
      }
      await _runConnectivityChecks();
      await _runBrowserDnsDiagnostics();
    } finally {
      if (mounted) {
        setState(() => _switchingNode = false);
      }
    }
  }

  Future<void> _runConnectivityChecks() async {
    if (_probeLoading) return;
    setState(() {
      _probeLoading = true;
      _probeMessage = null;
    });

    if (!mounted) return;
    final l10n = AppLocalizations.of(context);

    final directClient = HttpClient()
      ..connectionTimeout = const Duration(seconds: 8);
    final proxyClient = HttpClient()
      ..connectionTimeout = const Duration(seconds: 8)
      ..findProxy = (_) => 'PROXY $_kClashControllerHost:$_kHttpProxyPort;';

    try {
      // Start all probes concurrently
      final directIpFut = _fetchAllIpInfos(directClient, _directIpCandidates);
      final proxyIpFut = _fetchAllIpInfos(proxyClient, _proxyIpCandidates);
      final domesticFut = _checkSites(directClient, _domesticSites(l10n));
      final foreignFut = _checkSites(proxyClient, _foreignSites(l10n));
      final aiFut = _checkSites(proxyClient, _aiSites(l10n));

      final directIps = await directIpFut;
      final proxyIps = await proxyIpFut;
      final domestic = await domesticFut;
      final foreign = await foreignFut;
      final ai = await aiFut;

      if (!mounted) return;
      setState(() {
        _connectivitySnapshot = _ConnectivitySnapshot(
          checkedAt: DateTime.now(),
          directIpResults: directIps,
          proxyIpResults: proxyIps,
          domesticResults: domestic,
          foreignResults: foreign,
          aiResults: ai,
        );
      });
    } catch (e) {
      AppLogger.instance.error('probe', 'Connectivity probe failed: $e');
      if (!mounted) return;
      setState(() => _probeMessage = AppLocalizations.of(context).browserDnsFailed(e.toString()));
    } finally {
      directClient.close(force: true);
      proxyClient.close(force: true);
      if (mounted) setState(() => _probeLoading = false);
    }
  }

  // Fetch all candidates concurrently; returns only the ones that succeed.
  Future<List<_IpInfo>> _fetchAllIpInfos(
    HttpClient client,
    List<(String, String, String)> candidates,
  ) async {
    final futures = candidates.map((c) => _fetchSingleIpInfo(client, c));
    final results = await Future.wait(futures);
    return results.whereType<_IpInfo>().toList();
  }

  Future<_IpInfo?> _fetchSingleIpInfo(
    HttpClient client,
    (String, String, String) candidate,
  ) async {
    final (url, source, parse) = candidate;
    try {
      final req = await client
          .getUrl(Uri.parse(url))
          .timeout(const Duration(seconds: 8));
      req.headers.set(HttpHeaders.userAgentHeader, 'ClashForgeMobile/1.0');
      final res = await req.close().timeout(const Duration(seconds: 8));
      final body =
          await utf8.decodeStream(res).timeout(const Duration(seconds: 6));
      if (res.statusCode != 200) return null;
      final data = json.decode(body) as Map<String, dynamic>;

      String ip;
      String location;

      if (parse == 'upaiyun') {
        // {"remote_addr":"114.x.x.x","remote_addr_location":{"country":"中国","province":"江苏省","city":"苏州","isp":"电信"}}
        ip = (data['remote_addr'] ?? '').toString().trim();
        final loc = data['remote_addr_location'] as Map<String, dynamic>?;
        location = loc == null
            ? ''
            : [loc['country'], loc['province'], loc['city'], loc['isp']]
                .whereType<String>()
                .where((s) => s.isNotEmpty)
                .join(' · ');
      } else if (parse == 'ipsb') {
        // {"ip":"...","country":"China","city":"Suzhou","organization":"AS4134 CHINANET-BACKBONE"}
        ip = (data['ip'] ?? '').toString().trim();
        final city = (data['city'] ?? '').toString();
        final country = (data['country'] ?? '').toString();
        final org = (data['organization'] ?? '').toString();
        final orgClean = org.contains(' ') ? org.substring(org.indexOf(' ') + 1) : org;
        location = [city, country, orgClean].where((s) => s.isNotEmpty).join(' · ');
      } else {
        // ipinfo.io: {"ip":"...","city":"Tokyo","country":"JP","org":"AS8075 Microsoft Corporation"}
        ip = (data['ip'] ?? '').toString().trim();
        final city = (data['city'] ?? '').toString();
        final country = (data['country'] ?? '').toString();
        final org = (data['org'] ?? '').toString();
        location = [city, country, org].where((s) => s.isNotEmpty).join(' · ');
      }

      if (ip.isEmpty) return null;
      return _IpInfo(ip: ip, location: location, source: source);
    } catch (_) {
      return null;
    }
  }

  Future<List<_SiteCheckResult>> _checkSites(
    HttpClient client,
    List<(String, String, String, String)> sites,
  ) async {
    final results = <_SiteCheckResult>[];
    for (final (name, desc, url, domain) in sites) {
      final watch = Stopwatch()..start();
      try {
        final req = await client
            .getUrl(Uri.parse(url))
            .timeout(const Duration(seconds: 8));
        req.headers.set(HttpHeaders.userAgentHeader, 'ClashForgeMobile/1.0');
        final res = await req.close().timeout(const Duration(seconds: 10));
        await res.drain<void>().timeout(const Duration(seconds: 5));
        watch.stop();
        final ok = res.statusCode >= 200 && res.statusCode < 400;

        final dnsIps = await _queryMihomoDnsA(domain);
        String? dnsLabel;
        if (dnsIps.isNotEmpty) {
          final ip = dnsIps.first;
          final isFake = ip.startsWith('198.18.') || ip.startsWith('198.19.');
          dnsLabel = isFake ? '$ip (fake-ip)' : ip;
        }

        results.add(_SiteCheckResult(
          name: name,
          description: desc,
          ok: ok,
          latencyMs: watch.elapsedMilliseconds,
          dnsLabel: dnsLabel,
          error: ok ? null : 'HTTP ${res.statusCode}',
        ));
      } on TimeoutException {
        watch.stop();
        results.add(_SiteCheckResult(name: name, description: desc, ok: false, error: 'timeout'));
      } catch (e) {
        watch.stop();
        results.add(_SiteCheckResult(
          name: name,
          description: desc,
          ok: false,
          error: e.toString().split('\n').first,
        ));
      }
    }
    return results;
  }

  Future<void> _runBrowserDnsDiagnostics() async {
    if (_browserDnsLoading) return;
    final logger = AppLogger.instance;

    setState(() {
      _browserDnsLoading = true;
      _browserDnsMessage = null;
    });

    if (!mounted) return;
    final l10n = AppLocalizations.of(context);

    try {
      final checks = <_BrowserDnsCheckResult>[];
      final info = await VpnManager.getSystemInfo();

      final modeRaw =
          (info['private_dns_mode'] ?? '').toString().trim().toLowerCase();
      final specifier = (info['private_dns_specifier'] ?? '').toString().trim();
      final privateDnsOn = modeRaw == 'hostname' || modeRaw == 'opportunistic';
      checks.add(
        _BrowserDnsCheckResult(
          name: l10n.checkPrivateDns,
          ok: !privateDnsOn,
          detail: privateDnsOn
              ? (modeRaw == 'hostname' && specifier.isNotEmpty
                  ? l10n.privateDnsOnHostname(specifier)
                  : l10n.privateDnsOn(modeRaw))
              : l10n.privateDnsOff,
        ),
      );

      final mihomoAnswers = await _queryMihomoDnsA('www.google.com');
      checks.add(
        _BrowserDnsCheckResult(
          name: l10n.checkMihomoDns,
          ok: mihomoAnswers.isNotEmpty,
          detail: mihomoAnswers.isNotEmpty
              ? 'www.google.com -> ${mihomoAnswers.take(3).join(', ')}'
              : l10n.mihomoDnsNoRecord,
        ),
      );

      bool systemDnsOk = false;
      String systemDnsDetail = '';
      try {
        final addrs = await InternetAddress.lookup('www.google.com')
            .timeout(const Duration(seconds: 6));
        final ips = addrs
            .map((item) => item.address.trim())
            .where((item) => item.isNotEmpty)
            .toSet()
            .toList();
        systemDnsOk = ips.isNotEmpty;
        systemDnsDetail = systemDnsOk ? ips.take(3).join(', ') : l10n.systemDnsNoAddr;
      } catch (e) {
        systemDnsDetail = e.toString();
      }
      checks.add(
        _BrowserDnsCheckResult(
          name: l10n.checkSystemDns,
          ok: systemDnsOk,
          detail: systemDnsDetail,
        ),
      );

      bool proxySmokeOk = false;
      String proxySmokeDetail = '';
      final proxyClient = HttpClient()
        ..connectionTimeout = const Duration(seconds: 6)
        ..findProxy = (_) => 'PROXY $_kClashControllerHost:$_kHttpProxyPort;';
      try {
        final watch = Stopwatch()..start();
        final req = await proxyClient
            .getUrl(Uri.parse('https://www.gstatic.com/generate_204'))
            .timeout(const Duration(seconds: 6));
        req.headers.set(HttpHeaders.userAgentHeader, 'ClashForgeMobile/1.0');
        final res = await req.close().timeout(const Duration(seconds: 8));
        await res.drain<void>();
        watch.stop();
        proxySmokeOk = res.statusCode >= 200 && res.statusCode < 400;
        proxySmokeDetail = proxySmokeOk
            ? l10n.proxyChainSuccess(watch.elapsedMilliseconds)
            : 'HTTP ${res.statusCode}';
      } on TimeoutException {
        proxySmokeDetail = 'timeout';
      } catch (e) {
        proxySmokeDetail = e.toString().split('\n').first;
      } finally {
        proxyClient.close(force: true);
      }
      checks.add(
        _BrowserDnsCheckResult(
          name: l10n.checkProxyChain,
          ok: proxySmokeOk,
          detail: proxySmokeDetail,
        ),
      );

      final summary = _buildBrowserDnsSummary(checks, l10n);
      if (!mounted) return;
      setState(() {
        _browserDnsSnapshot = _BrowserDnsSnapshot(
          checkedAt: DateTime.now(),
          checks: checks,
          summary: summary,
        );
      });
    } catch (e) {
      logger.error('browser-dns', 'Browser DNS diagnostics failed: $e');
      if (!mounted) return;
      setState(() => _browserDnsMessage = AppLocalizations.of(context).browserDnsFailed(e.toString()));
    } finally {
      if (mounted) {
        setState(() => _browserDnsLoading = false);
      }
    }
  }

  String _buildBrowserDnsSummary(List<_BrowserDnsCheckResult> checks, AppLocalizations l10n) {
    bool failed(String name) =>
        checks.any((item) => item.name == name && !item.ok);

    if (failed(l10n.checkPrivateDns)) return l10n.summaryPrivateDnsOn;
    if (failed(l10n.checkMihomoDns)) return l10n.summaryMihomoDnsFailed;
    if (failed(l10n.checkSystemDns)) return l10n.summarySystemDnsFailed;
    if (failed(l10n.checkProxyChain)) return l10n.summaryProxyChainFailed;
    return l10n.summaryAllOk;
  }

  Future<List<String>> _queryMihomoDnsA(String host) async {
    try {
      final uri = Uri.parse(
        'http://$_kClashControllerHost:$_kClashControllerPort/dns/query?name='
        '${Uri.encodeQueryComponent(host)}&type=A',
      );
      final res = await http.get(uri).timeout(const Duration(seconds: 6));
      if (res.statusCode < 200 || res.statusCode >= 300) {
        return const [];
      }

      final body = json.decode(res.body);
      if (body is! Map<String, dynamic>) return const [];
      final answersRaw = body['Answer'];
      if (answersRaw is! List) return const [];

      final out = <String>[];
      for (final item in answersRaw) {
        if (item is! Map) continue;
        final data = (item['data'] ?? '').toString().trim();
        if (data.isEmpty) continue;
        out.add(data);
      }
      return out.toSet().toList();
    } catch (_) {
      return const [];
    }
  }


  @override
  Widget build(BuildContext context) {
    Subscription? activeSub;
    for (final s in _subscriptions) {
      if (s.id == _activeSubscriptionId) { activeSub = s; break; }
    }
    final activeIsBuiltIn = activeSub?.isBuiltIn ?? false;

    final tabs = <Widget>[
      _HomeTab(
        isConnected: _isConnected,
        isConnecting: _isConnecting,
        connectionStatus: _connectionStatus,
        selectedNode: _selectedNode,
        nodes: _nodes,
        isSwitchingNode: _switchingNode,
        probeLoading: _probeLoading,
        probeMessage: _probeMessage,
        browserDnsLoading: _browserDnsLoading,
        browserDnsMessage: _browserDnsMessage,
        privateDnsWarning: _privateDnsWarning,
        snapshot: _connectivitySnapshot,
        browserDnsSnapshot: _browserDnsSnapshot,
        onToggle: _toggleVpn,
        onRecheckProbe: _runConnectivityChecks,
        onRecheckBrowserDns: _runBrowserDnsDiagnostics,
        onSwitchNode: _switchNodeFromHome,
        onTapNode: () => setState(() => _tabIndex = 1),
      ),
      _ProxiesTab(
        customProxyGroups: activeSub?.customProxyGroups ?? const [],
        nodes: _nodes,
        selectedNode: _selectedNode,
        proxyNowMap: _proxyNowMap,
        vpnRunning: _isConnected,
        hideNodeDetails: activeIsBuiltIn,
        onGroupSwitch: _switchGroupMember,
        onRefresh: _refreshProxyNow,
        onSelect: (node) {
          unawaited(_onNodeSelected(node));
        },
      ),
      _SubscriptionsTab(
        onImported: _onNodesImported,
        subscriptions: _subscriptions,
        activeSubId: _activeSubscriptionId,
        onActivate: _activateSubscription,
        onDelete: _deleteSubscription,
      ),
      _SettingsTab(
        nodeCount: _nodes.length,
        onLocaleChanged: widget.onLocaleChanged,
      ),
    ];

    final l10n = AppLocalizations.of(context);
    return Scaffold(
      body: tabs[_tabIndex],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _tabIndex,
        onDestinationSelected: (i) => setState(() => _tabIndex = i),
        destinations: [
          NavigationDestination(
            icon: const Icon(Icons.shield_outlined),
            selectedIcon: const Icon(Icons.shield),
            label: l10n.navHome,
          ),
          NavigationDestination(
            icon: const Icon(Icons.language_outlined),
            selectedIcon: const Icon(Icons.language),
            label: l10n.navProxies,
          ),
          NavigationDestination(
            icon: const Icon(Icons.cloud_download_outlined),
            selectedIcon: const Icon(Icons.cloud_download),
            label: l10n.navSubscriptions,
          ),
          NavigationDestination(
            icon: const Icon(Icons.settings_outlined),
            selectedIcon: const Icon(Icons.settings),
            label: l10n.navSettings,
          ),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 1 — Home
// ─────────────────────────────────────────────────────────────
class _HomeTab extends StatelessWidget {
  const _HomeTab({
    required this.isConnected,
    required this.isConnecting,
    required this.connectionStatus,
    required this.selectedNode,
    required this.nodes,
    required this.isSwitchingNode,
    required this.probeLoading,
    required this.probeMessage,
    required this.browserDnsLoading,
    required this.browserDnsMessage,
    required this.privateDnsWarning,
    required this.snapshot,
    required this.browserDnsSnapshot,
    required this.onToggle,
    required this.onRecheckProbe,
    required this.onRecheckBrowserDns,
    required this.onSwitchNode,
    required this.onTapNode,
  });

  final bool isConnected;
  final bool isConnecting;
  final String connectionStatus;
  final ProxyNode? selectedNode;
  final List<ProxyNode> nodes;
  final bool isSwitchingNode;
  final bool probeLoading;
  final String? probeMessage;
  final bool browserDnsLoading;
  final String? browserDnsMessage;
  final String? privateDnsWarning;
  final _ConnectivitySnapshot? snapshot;
  final _BrowserDnsSnapshot? browserDnsSnapshot;
  final VoidCallback onToggle;
  final Future<void> Function() onRecheckProbe;
  final Future<void> Function() onRecheckBrowserDns;
  final Future<void> Function(ProxyNode) onSwitchNode;
  final VoidCallback onTapNode;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final accent = isConnected ? _kConnected : _kBrand;
    final _fmt = (DateTime dt) =>
        '${dt.hour.toString().padLeft(2, '0')}:'
        '${dt.minute.toString().padLeft(2, '0')}:'
        '${dt.second.toString().padLeft(2, '0')}';
    final checkedAt = snapshot == null ? '--' : _fmt(snapshot!.checkedAt);
    final browserDnsCheckedAt = browserDnsSnapshot == null ? '--' : _fmt(browserDnsSnapshot!.checkedAt);

    return Container(
      decoration: const BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            Color(0xFF110F26),
            _kBg,
          ],
        ),
      ),
      child: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.fromLTRB(20, 18, 20, 22),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Container(
                    width: 36,
                    height: 36,
                    decoration: BoxDecoration(
                      color: _kBrand.withAlpha(24),
                      borderRadius: BorderRadius.circular(11),
                      border: Border.all(color: _kBrand.withAlpha(80)),
                    ),
                    child: const Icon(Icons.bolt, color: _kBrand, size: 20),
                  ),
                  const SizedBox(width: 10),
                  const Text(
                    'ClashForge',
                    style: TextStyle(
                      color: _kTextHi,
                      fontSize: 21,
                      fontWeight: FontWeight.w700,
                      letterSpacing: -0.3,
                    ),
                  ),
                  const Spacer(),
                  Container(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
                    decoration: BoxDecoration(
                      color: (isConnected ? _kConnected : _kTextFaint)
                          .withAlpha(16),
                      borderRadius: BorderRadius.circular(999),
                      border: Border.all(
                        color: (isConnected ? _kConnected : _kTextFaint)
                            .withAlpha(90),
                      ),
                    ),
                    child: Text(
                      isConnected ? l10n.statusActive : l10n.statusIdle,
                      style: TextStyle(
                        color: isConnected ? _kConnected : _kTextMuted,
                        fontSize: 10,
                        fontWeight: FontWeight.w800,
                        letterSpacing: 1.15,
                      ),
                    ),
                  ),
                ],
              ),
              const SizedBox(height: 18),
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  gradient: const LinearGradient(
                    begin: Alignment.topLeft,
                    end: Alignment.bottomRight,
                    colors: [_kCardGrad, _kCard],
                  ),
                  borderRadius: BorderRadius.circular(18),
                  border: Border.all(color: _kBorder),
                ),
                child: Row(
                  children: [
                    GestureDetector(
                      key: const Key('vpn_toggle'),
                      onTap: onToggle,
                      child: AnimatedContainer(
                        duration: const Duration(milliseconds: 250),
                        width: 88,
                        height: 88,
                        decoration: BoxDecoration(
                          shape: BoxShape.circle,
                          color: accent.withAlpha(16),
                          border: Border.all(color: accent, width: 1.4),
                          boxShadow: [
                            BoxShadow(
                                color: accent.withAlpha(70),
                                blurRadius: 20,
                                spreadRadius: 1),
                          ],
                        ),
                        child: isConnecting
                            ? Center(
                                child: SizedBox(
                                  width: 26,
                                  height: 26,
                                  child: CircularProgressIndicator(
                                    color: accent,
                                    strokeWidth: 2,
                                    backgroundColor: accent.withAlpha(35),
                                  ),
                                ),
                              )
                            : Icon(Icons.power_settings_new,
                                color: accent, size: 34),
                      ),
                    ),
                    const SizedBox(width: 16),
                    Expanded(
                      child: Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Text(
                            isConnected ? l10n.vpnRunning : l10n.vpnIdle,
                            style: TextStyle(
                              color: accent,
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            connectionStatus.isEmpty ? l10n.connTapToConnect : connectionStatus,
                            style: const TextStyle(
                                color: _kTextMuted, fontSize: 13, height: 1.4),
                          ),
                          const SizedBox(height: 10),
                          Row(
                            children: [
                              Expanded(
                                child: FilledButton(
                                  onPressed: onToggle,
                                  style: FilledButton.styleFrom(
                                    backgroundColor: accent.withAlpha(36),
                                    foregroundColor: accent,
                                    shape: RoundedRectangleBorder(
                                      borderRadius: BorderRadius.circular(10),
                                      side: BorderSide(
                                          color: accent.withAlpha(120)),
                                    ),
                                  ),
                                  child: Text(isConnected ? l10n.btnDisconnect : l10n.btnConnect),
                                ),
                              ),
                              const SizedBox(width: 8),
                              OutlinedButton(
                                onPressed: onTapNode,
                                style: OutlinedButton.styleFrom(
                                  foregroundColor: _kTextMuted,
                                  side: BorderSide(
                                      color: _kBorder.withAlpha(180)),
                                ),
                                child: Text(l10n.btnMoreNodes),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ],
                ),
              ),
              if (privateDnsWarning != null) ...[
                const SizedBox(height: 14),
                Container(
                  width: double.infinity,
                  padding:
                      const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                  decoration: BoxDecoration(
                    color: _kError.withAlpha(16),
                    borderRadius: BorderRadius.circular(12),
                    border: Border.all(color: _kError.withAlpha(100)),
                  ),
                  child: Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      const Padding(
                        padding: EdgeInsets.only(top: 1),
                        child: Icon(Icons.warning_amber_rounded,
                            size: 16, color: _kError),
                      ),
                      const SizedBox(width: 8),
                      Expanded(
                        child: Text(
                          privateDnsWarning!,
                          style: const TextStyle(
                              color: _kTextHi, fontSize: 12, height: 1.4),
                        ),
                      ),
                    ],
                  ),
                ),
              ],
              const SizedBox(height: 16),
              _HomeBlockCard(
                title: l10n.connectivityTitle,
                subtitle: l10n.connectivitySubtitle,
                trailing: TextButton.icon(
                  onPressed:
                      probeLoading ? null : () => unawaited(onRecheckProbe()),
                  style: TextButton.styleFrom(
                    foregroundColor: _kBrand,
                    padding:
                        const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  ),
                  icon: probeLoading
                      ? const SizedBox(
                          width: 12,
                          height: 12,
                          child: CircularProgressIndicator(
                              strokeWidth: 2, color: _kBrand),
                        )
                      : const Icon(Icons.refresh, size: 14),
                  label: Text(l10n.btnRecheck, style: const TextStyle(fontSize: 12)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          l10n.lastChecked(checkedAt),
                          style:
                              const TextStyle(color: _kTextFaint, fontSize: 11),
                        ),
                        const Spacer(),
                        if (probeMessage != null)
                          Flexible(
                            child: Text(
                              probeMessage!,
                              overflow: TextOverflow.ellipsis,
                              style:
                                  const TextStyle(color: _kError, fontSize: 11),
                            ),
                          ),
                      ],
                    ),
                    const SizedBox(height: 10),
                    if (snapshot == null && !probeLoading)
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.symmetric(
                            horizontal: 12, vertical: 14),
                        decoration: BoxDecoration(
                          color: _kBg,
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(color: _kBorder),
                        ),
                        child: Text(
                          l10n.hintClickRecheck,
                          style: const TextStyle(color: _kTextMuted, fontSize: 13),
                        ),
                      ),
                    if (snapshot != null)
                      _ConnectivityPane(snapshot: snapshot!),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              _HomeBlockCard(
                title: l10n.browserDnsTitle,
                subtitle: l10n.browserDnsSubtitle,
                trailing: TextButton.icon(
                  onPressed: browserDnsLoading
                      ? null
                      : () => unawaited(onRecheckBrowserDns()),
                  style: TextButton.styleFrom(
                    foregroundColor: _kBrand,
                    padding:
                        const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                  ),
                  icon: browserDnsLoading
                      ? const SizedBox(
                          width: 12,
                          height: 12,
                          child: CircularProgressIndicator(
                              strokeWidth: 2, color: _kBrand),
                        )
                      : const Icon(Icons.refresh, size: 14),
                  label: Text(l10n.btnRecheck, style: const TextStyle(fontSize: 12)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          l10n.lastChecked(browserDnsCheckedAt),
                          style:
                              const TextStyle(color: _kTextFaint, fontSize: 11),
                        ),
                        const Spacer(),
                        if (browserDnsMessage != null)
                          Flexible(
                            child: Text(
                              browserDnsMessage!,
                              overflow: TextOverflow.ellipsis,
                              style:
                                  const TextStyle(color: _kError, fontSize: 11),
                            ),
                          ),
                      ],
                    ),
                    const SizedBox(height: 10),
                    if (browserDnsSnapshot == null && !browserDnsLoading)
                      Container(
                        width: double.infinity,
                        padding: const EdgeInsets.symmetric(
                            horizontal: 12, vertical: 14),
                        decoration: BoxDecoration(
                          color: _kBg,
                          borderRadius: BorderRadius.circular(12),
                          border: Border.all(color: _kBorder),
                        ),
                        child: Text(
                          l10n.hintClickRecheckBrowserDns,
                          style: const TextStyle(color: _kTextMuted, fontSize: 13),
                        ),
                      ),
                    if (browserDnsSnapshot != null)
                      _BrowserDnsPane(snapshot: browserDnsSnapshot!),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              _HomeBlockCard(
                title: l10n.nodeSwitchTitle,
                subtitle: l10n.nodeSwitchSubtitle(_kMainProxyGroup),
                trailing: isSwitchingNode
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: _kBrand),
                      )
                    : null,
                child: nodes.isEmpty
                    ? Text(
                        l10n.hintNoNodes,
                        style: const TextStyle(color: _kTextMuted, fontSize: 13),
                      )
                    : Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          Wrap(
                            spacing: 8,
                            runSpacing: 8,
                            children: nodes.take(12).map((node) {
                              final active = selectedNode?.name == node.name;
                              return InkWell(
                                borderRadius: BorderRadius.circular(999),
                                onTap: isSwitchingNode
                                    ? null
                                    : () => unawaited(onSwitchNode(node)),
                                child: AnimatedContainer(
                                  duration: const Duration(milliseconds: 180),
                                  padding: const EdgeInsets.symmetric(
                                      horizontal: 12, vertical: 8),
                                  decoration: BoxDecoration(
                                    color:
                                        active ? _kBrand.withAlpha(30) : _kBg,
                                    borderRadius: BorderRadius.circular(999),
                                    border: Border.all(
                                      color: active
                                          ? _kBrand.withAlpha(140)
                                          : _kBorder,
                                    ),
                                  ),
                                  child: Text(
                                    node.name,
                                    style: TextStyle(
                                      color: active ? _kBrand : _kTextMuted,
                                      fontSize: 12,
                                      fontWeight: active
                                          ? FontWeight.w700
                                          : FontWeight.w500,
                                    ),
                                  ),
                                ),
                              );
                            }).toList(),
                          ),
                          const SizedBox(height: 10),
                          TextButton(
                            onPressed: onTapNode,
                            style: TextButton.styleFrom(
                              foregroundColor: _kTextMuted,
                              padding: EdgeInsets.zero,
                            ),
                            child: Text(l10n.linkViewAllNodes),
                          ),
                        ],
                      ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _HomeBlockCard extends StatelessWidget {
  const _HomeBlockCard({
    required this.title,
    required this.subtitle,
    required this.child,
    this.trailing,
  });

  final String title;
  final String subtitle;
  final Widget child;
  final Widget? trailing;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(14),
      decoration: BoxDecoration(
        gradient: const LinearGradient(
          begin: Alignment.topLeft,
          end: Alignment.bottomRight,
          colors: [_kCardGrad, _kCard],
        ),
        borderRadius: BorderRadius.circular(18),
        border: Border.all(color: _kBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      title,
                      style: const TextStyle(
                        color: _kTextHi,
                        fontSize: 15,
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    const SizedBox(height: 2),
                    Text(
                      subtitle,
                      style: const TextStyle(color: _kTextFaint, fontSize: 11),
                    ),
                  ],
                ),
              ),
              if (trailing != null) trailing!,
            ],
          ),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}

// ── Connectivity probe widgets ──────────────────────────────────────────────

class _ConnectivityPane extends StatelessWidget {
  const _ConnectivityPane({required this.snapshot});
  final _ConnectivitySnapshot snapshot;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        _SectionLabel(label: l10n.sectionExitIp),
        const SizedBox(height: 8),
        _IpGroup(
          categoryTag: l10n.categoryDirect,
          tagColor: _kBrand,
          subtitle: l10n.directSubtitle,
          results: snapshot.directIpResults,
        ),
        const SizedBox(height: 6),
        _IpGroup(
          categoryTag: l10n.categoryVpnExit,
          tagColor: const Color(0xFF64B5F6),
          subtitle: l10n.vpnExitSubtitle,
          results: snapshot.proxyIpResults,
        ),
        const SizedBox(height: 14),
        _SectionLabel(label: l10n.sectionAccessCheck),
        const SizedBox(height: 8),
        if (snapshot.domesticResults.isNotEmpty) ...[
          _CategoryTag(label: l10n.categoryDirectPath, color: _kBrand),
          const SizedBox(height: 6),
          ...snapshot.domesticResults.map((r) => _SiteCheckRow(result: r)),
          const SizedBox(height: 8),
        ],
        if (snapshot.foreignResults.isNotEmpty) ...[
          _CategoryTag(label: l10n.categoryVpnProxy, color: const Color(0xFF64B5F6)),
          const SizedBox(height: 6),
          ...snapshot.foreignResults.map((r) => _SiteCheckRow(result: r)),
          const SizedBox(height: 8),
        ],
        if (snapshot.aiResults.isNotEmpty) ...[
          _CategoryTag(label: l10n.categoryAiVpnProxy, color: const Color(0xFF9C6DFF)),
          const SizedBox(height: 6),
          ...snapshot.aiResults.map((r) => _SiteCheckRow(result: r)),
        ],
      ],
    );
  }
}

class _SectionLabel extends StatelessWidget {
  const _SectionLabel({required this.label});
  final String label;

  @override
  Widget build(BuildContext context) {
    return Text(
      label,
      style: const TextStyle(
          color: _kTextMuted, fontSize: 12, fontWeight: FontWeight.w600),
    );
  }
}

class _CategoryTag extends StatelessWidget {
  const _CategoryTag({required this.label, required this.color});
  final String label;
  final Color color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 3),
      decoration: BoxDecoration(
        color: color.withAlpha(36),
        borderRadius: BorderRadius.circular(999),
      ),
      child: Text(
        label,
        style: TextStyle(
            color: color, fontSize: 11, fontWeight: FontWeight.w700),
      ),
    );
  }
}

// Wraps a labelled row of one or two _IpCard tiles for a given direction.
class _IpGroup extends StatelessWidget {
  const _IpGroup({
    required this.categoryTag,
    required this.tagColor,
    required this.subtitle,
    required this.results,
  });
  final String categoryTag;
  final Color tagColor;
  final String subtitle;
  final List<_IpInfo> results;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Row(
          children: [
            _CategoryTag(label: categoryTag, color: tagColor),
            const SizedBox(width: 8),
            Text(subtitle,
                style: const TextStyle(color: _kTextFaint, fontSize: 10)),
          ],
        ),
        const SizedBox(height: 6),
        if (results.isEmpty)
          const _IpCard(info: null)
        else
          Column(
            children: [
              for (int i = 0; i < results.length; i++) ...[
                if (i > 0) const SizedBox(height: 6),
                _IpCard(info: results[i]),
              ],
            ],
          ),
      ],
    );
  }
}

class _IpCard extends StatelessWidget {
  const _IpCard({this.info});
  final _IpInfo? info;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final resolved = info != null;
    final badgeColor = resolved ? _kConnected : _kTextFaint;
    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: _kBg,
        borderRadius: BorderRadius.circular(10),
        border: Border.all(color: _kBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  info?.source ?? '—',
                  style: const TextStyle(
                      color: _kTextFaint,
                      fontSize: 10,
                      fontWeight: FontWeight.w500),
                ),
              ),
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                decoration: BoxDecoration(
                  color: badgeColor.withAlpha(20),
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(color: badgeColor.withAlpha(100)),
                ),
                child: Text(
                  resolved ? l10n.ipResolved : l10n.ipFailed,
                  style: TextStyle(
                      color: badgeColor,
                      fontSize: 9,
                      fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ),
          if (resolved) ...[
            const SizedBox(height: 6),
            Text(
              info!.ip,
              style: const TextStyle(
                  color: _kTextHi,
                  fontSize: 18,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.3),
            ),
            const SizedBox(height: 2),
            Text(
              info!.location,
              style: const TextStyle(color: _kTextMuted, fontSize: 11),
            ),
          ],
        ],
      ),
    );
  }
}

class _SiteCheckRow extends StatelessWidget {
  const _SiteCheckRow({required this.result});
  final _SiteCheckResult result;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final tone = result.ok ? _kConnected : _kError;
    return Padding(
      padding: const EdgeInsets.only(bottom: 7),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 11, vertical: 9),
        decoration: BoxDecoration(
          color: _kBg,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: _kBorder),
        ),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Expanded(
                  child: Text(
                    result.name,
                    style: const TextStyle(
                        color: _kTextHi,
                        fontSize: 13,
                        fontWeight: FontWeight.w600),
                  ),
                ),
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 7, vertical: 2),
                  decoration: BoxDecoration(
                    color: tone.withAlpha(20),
                    borderRadius: BorderRadius.circular(999),
                    border: Border.all(color: tone.withAlpha(100)),
                  ),
                  child: Text(
                    result.ok ? l10n.siteStatusOk : l10n.siteStatusError,
                    style: TextStyle(
                        color: tone,
                        fontSize: 10,
                        fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
            const SizedBox(height: 2),
            Text(
              result.description,
              style: const TextStyle(color: _kTextFaint, fontSize: 11),
            ),
            if (result.ok) ...[
              const SizedBox(height: 5),
              Row(
                children: [
                  if (result.latencyMs != null) ...[
                    Text(
                      '${result.latencyMs} ms',
                      style: const TextStyle(
                          color: _kTextMuted,
                          fontSize: 12,
                          fontWeight: FontWeight.w600),
                    ),
                    const SizedBox(width: 10),
                  ],
                  if (result.dnsLabel != null)
                    Expanded(
                      child: Text(
                        'DNS → ${result.dnsLabel}',
                        style: const TextStyle(
                            color: _kTextFaint, fontSize: 11),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                ],
              ),
            ],
            if (!result.ok && result.error != null) ...[
              const SizedBox(height: 4),
              Text(
                result.error!,
                style: const TextStyle(
                    color: _kError, fontSize: 11, height: 1.3),
                maxLines: 2,
                overflow: TextOverflow.ellipsis,
              ),
            ],
          ],
        ),
      ),
    );
  }
}

class _BrowserDnsPane extends StatelessWidget {
  const _BrowserDnsPane({required this.snapshot});

  final _BrowserDnsSnapshot snapshot;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final tone = snapshot.healthy ? _kConnected : _kError;

    return Container(
      width: double.infinity,
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: _kBg,
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: _kBorder),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Expanded(
                child: Text(
                  l10n.browserDnsPathLabel,
                  style: const TextStyle(
                      color: _kTextHi,
                      fontSize: 14,
                      fontWeight: FontWeight.w600),
                ),
              ),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 9, vertical: 4),
                decoration: BoxDecoration(
                  color: tone.withAlpha(16),
                  borderRadius: BorderRadius.circular(999),
                  border: Border.all(color: tone.withAlpha(120)),
                ),
                child: Text(
                  snapshot.healthy ? l10n.statusHealthy : l10n.statusRisk,
                  style: TextStyle(
                      color: tone, fontSize: 11, fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          ...snapshot.checks.map((item) {
            final okColor = item.ok ? _kConnected : _kError;
            return Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: Row(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Container(
                    margin: const EdgeInsets.only(top: 4),
                    width: 8,
                    height: 8,
                    decoration:
                        BoxDecoration(color: okColor, shape: BoxShape.circle),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Text(
                          item.name,
                          style: const TextStyle(
                              color: _kTextHi,
                              fontSize: 12,
                              fontWeight: FontWeight.w600),
                        ),
                        const SizedBox(height: 1),
                        Text(
                          item.detail,
                          style:
                              const TextStyle(color: _kTextMuted, fontSize: 11),
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            );
          }),
          const SizedBox(height: 4),
          Container(
            width: double.infinity,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 9),
            decoration: BoxDecoration(
              color: tone.withAlpha(12),
              borderRadius: BorderRadius.circular(10),
              border: Border.all(color: tone.withAlpha(80)),
            ),
            child: Text(
              snapshot.summary,
              style:
                  const TextStyle(color: _kTextHi, fontSize: 12, height: 1.4),
            ),
          ),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 2 — Routes (proxy groups)
// ─────────────────────────────────────────────────────────────
class _ProxiesTab extends StatefulWidget {
  const _ProxiesTab({
    required this.customProxyGroups,
    required this.nodes,
    required this.selectedNode,
    required this.proxyNowMap,
    required this.vpnRunning,
    required this.onGroupSwitch,
    required this.onRefresh,
    required this.onSelect,
    this.hideNodeDetails = false,
  });

  final List<Map<String, dynamic>> customProxyGroups;
  final List<ProxyNode> nodes;
  final ProxyNode? selectedNode;
  final Map<String, String> proxyNowMap;
  final bool vpnRunning;
  final Future<void> Function(String group, String member) onGroupSwitch;
  final Future<void> Function() onRefresh;
  final ValueChanged<ProxyNode> onSelect;
  final bool hideNodeDetails;

  @override
  State<_ProxiesTab> createState() => _ProxiesTabState();
}

class _ProxiesTabState extends State<_ProxiesTab> {
  @override
  void initState() {
    super.initState();
    if (widget.vpnRunning) {
      WidgetsBinding.instance
          .addPostFrameCallback((_) => widget.onRefresh());
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final groups = widget.customProxyGroups;

    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
            child: Row(
              children: [
                Text(l10n.proxiesTitle,
                    style: const TextStyle(
                        color: _kTextHi,
                        fontSize: 24,
                        fontWeight: FontWeight.bold,
                        letterSpacing: -0.5)),
                const Spacer(),
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                  decoration: BoxDecoration(
                    color: _kBrand.withAlpha(18),
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(color: _kBrand.withAlpha(55)),
                  ),
                  child: Text(
                    groups.isNotEmpty
                        ? '${groups.length} groups'
                        : l10n.nodesCount(widget.nodes.length),
                    style: const TextStyle(
                        color: _kBrand,
                        fontSize: 12,
                        fontWeight: FontWeight.w600),
                  ),
                ),
              ],
            ),
          ),
          Expanded(
            child: groups.isEmpty
                ? _buildFlatNodeList(l10n)
                : RefreshIndicator(
                    onRefresh: widget.onRefresh,
                    color: _kBrand,
                    child: ListView.separated(
                      padding: const EdgeInsets.fromLTRB(16, 0, 16, 24),
                      itemCount: groups.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 10),
                      itemBuilder: (context, i) {
                        final g = groups[i];
                        final name = (g['name'] as String?) ?? '';
                        final type =
                            ((g['type'] as String?) ?? '').toLowerCase();
                        final members =
                            (g['proxies'] as List?)?.cast<String>() ?? [];
                        return _ProxyGroupCard(
                          name: name,
                          type: type,
                          members: members,
                          currentNow: widget.proxyNowMap[name],
                          vpnRunning: widget.vpnRunning,
                          initiallyExpanded: i == 0,
                          onMemberTap: type == 'select'
                              ? (member) =>
                                  unawaited(widget.onGroupSwitch(name, member))
                              : null,
                        );
                      },
                    ),
                  ),
          ),
        ],
      ),
    );
  }

  Widget _buildFlatNodeList(AppLocalizations l10n) {
    final nodes = widget.nodes;
    final selectedNode = widget.selectedNode;
    if (nodes.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Container(
              width: 72,
              height: 72,
              decoration: BoxDecoration(
                color: _kBrand.withAlpha(15),
                shape: BoxShape.circle,
                border: Border.all(color: _kBrand.withAlpha(40)),
              ),
              child: const Icon(Icons.cloud_off_outlined,
                  size: 34, color: _kBrand),
            ),
            const SizedBox(height: 16),
            Text(l10n.noNodesYet,
                style: const TextStyle(
                    color: _kTextHi,
                    fontSize: 16,
                    fontWeight: FontWeight.w600)),
            const SizedBox(height: 6),
            Text(l10n.noNodesHint,
                textAlign: TextAlign.center,
                style: const TextStyle(
                    color: _kTextMuted, height: 1.5, fontSize: 13)),
          ],
        ),
      );
    }
    return ListView.separated(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      itemCount: nodes.length,
      separatorBuilder: (_, __) => const SizedBox(height: 6),
      itemBuilder: (context, i) {
        final node = nodes[i];
        final selected = node == selectedNode;
        return InkWell(
          borderRadius: BorderRadius.circular(14),
          onTap: () => widget.onSelect(node),
          child: AnimatedContainer(
            duration: const Duration(milliseconds: 200),
            padding:
                const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
            decoration: BoxDecoration(
              gradient: selected
                  ? LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [_kBrand.withAlpha(28), _kCard],
                    )
                  : null,
              color: selected ? null : _kCard,
              borderRadius: BorderRadius.circular(14),
              border: Border.all(
                color: selected ? _kBrand.withAlpha(120) : _kBorder,
                width: selected ? 1.5 : 1,
              ),
              boxShadow: selected
                  ? [
                      BoxShadow(
                          color: _kBrand.withAlpha(30),
                          blurRadius: 12,
                          spreadRadius: 0)
                    ]
                  : null,
            ),
            child: Row(
              children: [
                Container(
                  width: 36,
                  height: 36,
                  decoration: BoxDecoration(
                    color: _kBrand.withAlpha(selected ? 30 : 18),
                    borderRadius: BorderRadius.circular(9),
                    border: Border.all(
                        color: _kBrand.withAlpha(selected ? 80 : 40)),
                  ),
                  child: Icon(Icons.language,
                      color: selected ? _kBrand : _kTextMuted, size: 18),
                ),
                const SizedBox(width: 12),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(node.name,
                          style: const TextStyle(
                              color: _kTextHi,
                              fontWeight: FontWeight.w600,
                              fontSize: 14)),
                      if (!widget.hideNodeDetails) ...[
                        const SizedBox(height: 2),
                        Text(
                            '${node.type.toUpperCase()} · ${node.server}:${node.port}',
                            style: const TextStyle(
                                color: _kTextMuted, fontSize: 12)),
                      ],
                    ],
                  ),
                ),
                if (selected)
                  Container(
                    width: 22,
                    height: 22,
                    decoration: BoxDecoration(
                      color: _kBrand.withAlpha(30),
                      shape: BoxShape.circle,
                      border: Border.all(color: _kBrand.withAlpha(100)),
                    ),
                    child: const Icon(Icons.check, color: _kBrand, size: 13),
                  ),
              ],
            ),
          ),
        );
      },
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Proxy group card (one per group in the Routes tab)
// ─────────────────────────────────────────────────────────────
class _ProxyGroupCard extends StatelessWidget {
  const _ProxyGroupCard({
    required this.name,
    required this.type,
    required this.members,
    required this.vpnRunning,
    this.currentNow,
    this.initiallyExpanded = false,
    this.onMemberTap,
  });

  final String name;
  final String type;
  final List<String> members;
  final String? currentNow;
  final bool vpnRunning;
  final bool initiallyExpanded;
  final void Function(String member)? onMemberTap;

  @override
  Widget build(BuildContext context) {
    final isAuto = type == 'url-test' || type == 'fallback';
    final typeLabel = isAuto ? 'AUTO' : 'SELECT';
    final typeColor =
        isAuto ? const Color(0xFF64B5F6) : _kBrand;
    final nowText = currentNow ??
        (vpnRunning ? '…' : '—');

    return Container(
      decoration: BoxDecoration(
        color: _kCard,
        borderRadius: BorderRadius.circular(16),
        border: Border.all(color: _kBorder),
      ),
      clipBehavior: Clip.hardEdge,
      child: Theme(
        data: Theme.of(context).copyWith(dividerColor: Colors.transparent),
        child: ExpansionTile(
          initiallyExpanded: initiallyExpanded,
          tilePadding:
              const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
          childrenPadding: EdgeInsets.zero,
          title: Row(
            children: [
              Expanded(
                child: Text(name,
                    style: const TextStyle(
                        color: _kTextHi,
                        fontWeight: FontWeight.w600,
                        fontSize: 15)),
              ),
              const SizedBox(width: 8),
              Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                decoration: BoxDecoration(
                  color: typeColor.withAlpha(22),
                  borderRadius: BorderRadius.circular(6),
                  border: Border.all(color: typeColor.withAlpha(70)),
                ),
                child: Text(typeLabel,
                    style: TextStyle(
                        color: typeColor,
                        fontSize: 11,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 0.5)),
              ),
            ],
          ),
          subtitle: Padding(
            padding: const EdgeInsets.only(top: 2),
            child: Text(nowText,
                style: TextStyle(
                    color: currentNow != null ? _kBrand : _kTextMuted,
                    fontSize: 12,
                    fontWeight: FontWeight.w500)),
          ),
          children: [
            const Divider(height: 1, thickness: 1, color: _kBorder),
            ...members.map((member) => _GroupMemberTile(
                  member: member,
                  isCurrent: member == currentNow,
                  canTap: onMemberTap != null,
                  onTap: onMemberTap != null
                      ? () => onMemberTap!(member)
                      : null,
                )),
          ],
        ),
      ),
    );
  }
}

class _GroupMemberTile extends StatelessWidget {
  const _GroupMemberTile({
    required this.member,
    required this.isCurrent,
    required this.canTap,
    this.onTap,
  });

  final String member;
  final bool isCurrent;
  final bool canTap;
  final VoidCallback? onTap;

  static IconData _icon(String name) {
    if (name == 'DIRECT') return Icons.swap_horiz;
    if (name == 'REJECT') return Icons.block_outlined;
    if (name.contains('自动') || name.toLowerCase().contains('auto')) {
      return Icons.auto_awesome_outlined;
    }
    return Icons.language_outlined;
  }

  @override
  Widget build(BuildContext context) {
    return InkWell(
      onTap: onTap,
      child: Container(
        padding:
            const EdgeInsets.symmetric(horizontal: 16, vertical: 13),
        decoration: BoxDecoration(
          color: isCurrent ? _kBrand.withAlpha(10) : Colors.transparent,
          border: const Border(
              top: BorderSide(color: _kBorder, width: 0.5)),
        ),
        child: Row(
          children: [
            Container(
              width: 30,
              height: 30,
              decoration: BoxDecoration(
                color: isCurrent ? _kBrand.withAlpha(25) : _kBg,
                borderRadius: BorderRadius.circular(8),
                border: Border.all(
                    color:
                        isCurrent ? _kBrand.withAlpha(90) : _kBorder),
              ),
              child: Icon(_icon(member),
                  size: 15,
                  color: isCurrent ? _kBrand : _kTextMuted),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Text(member,
                  style: TextStyle(
                      color: isCurrent ? _kTextHi : _kTextMuted,
                      fontWeight: isCurrent
                          ? FontWeight.w600
                          : FontWeight.normal,
                      fontSize: 14)),
            ),
            if (isCurrent)
              const Icon(Icons.check_circle_outline,
                  color: _kBrand, size: 18)
            else if (canTap)
              Icon(Icons.chevron_right,
                  color: _kBorder.withAlpha(180), size: 18),
          ],
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 3 — Subscriptions
// ─────────────────────────────────────────────────────────────
typedef _OnImported = void Function(ParsedSubscription parsed,
    {String url, String nickname});
typedef _OnSubActivated = void Function(Subscription sub);
typedef _OnSubDeleted = void Function(Subscription sub);

class _SubscriptionsTab extends StatefulWidget {
  const _SubscriptionsTab({
    required this.onImported,
    required this.subscriptions,
    required this.activeSubId,
    required this.onActivate,
    required this.onDelete,
  });
  final _OnImported onImported;
  final List<Subscription> subscriptions;
  final String? activeSubId;
  final _OnSubActivated onActivate;
  final _OnSubDeleted onDelete;

  @override
  State<_SubscriptionsTab> createState() => _SubscriptionsTabState();
}

class _SubscriptionsTabState extends State<_SubscriptionsTab> {
  final _urlController = TextEditingController();
  final _pasteController = TextEditingController();
  bool _loading = false;
  bool _pasteLoading = false;
  String? _message;
  String? _pasteMessage;
  bool _success = false;
  bool _pasteSuccess = false;

  @override
  void initState() {
    super.initState();
    SubscriptionStore.loadUrl().then((url) {
      if (url.isNotEmpty && mounted) setState(() => _urlController.text = url);
    });
  }

  @override
  void dispose() {
    _urlController.dispose();
    _pasteController.dispose();
    super.dispose();
  }

  static const _httpChannel = MethodChannel('com.clashforge.mobile/http');

  // Uses Android native networking stack (Cronet first, HttpURLConnection fallback)
  // to improve compatibility with subscription endpoints.
  Future<(int, String)> _fetchUrlNative(String url) async {
    final result = await _httpChannel.invokeMapMethod<String, dynamic>(
        'fetchUrl', {'url': url, 'timeoutMs': 15000});
    return (result!['status'] as int, result['body'] as String);
  }

  Future<(int, String)> _fetchUrlDart(String url) async {
    final response =
        await http.get(Uri.parse(url)).timeout(const Duration(seconds: 15));
    return (response.statusCode, response.body);
  }

  Future<(int, String)> _fetchUrlWithFallback(String url) async {
    if (!Platform.isAndroid) {
      return _fetchUrlDart(url);
    }
    try {
      return await _fetchUrlNative(url);
    } on PlatformException catch (e) {
      AppLogger.instance.warn(
        'subscription',
        'Native fetch failed, fallback to Dart',
        fields: {'code': e.code, 'message': e.message ?? ''},
      );
      return _fetchUrlDart(url);
    }
  }

  Future<void> _import() async {
    final input = _urlController.text.trim();
    if (input.isEmpty) return;
    final logger = AppLogger.instance;
    setState(() {
      _loading = true;
      _message = null;
    });
    try {
      String content = input;

      if (input.startsWith('http://') || input.startsWith('https://')) {
        logger.info('subscription', 'Fetching URL',
            fields: {'url': _redactSubscriptionUrl(input)});
        final (statusCode, body) = await _fetchUrlWithFallback(input);
        logger.info('subscription', 'Fetch response',
            fields: {'status': statusCode, 'bytes': body.length});
        if (statusCode != 200) {
          if (!mounted) return;
          setState(() {
            _loading = false;
            _success = false;
            _message = AppLocalizations.of(context).fetchFailed(statusCode);
          });
          return;
        }
        content = body;
      }

      final parsed = SubscriptionParser.parse(content);
      logger.info('subscription', 'Parsed nodes',
          fields: {'count': parsed.proxies.length, 'has_custom_rules': parsed.hasCustomRules});
      if (parsed.proxies.isEmpty) {
        logger.warn('subscription', 'No nodes parsed');
      }

      final defaultName = await SubscriptionStore.generateDefaultNickname();
      if (!mounted) return;
      final nickname = await _showNicknameDialog(defaultName);
      if (!mounted) return;
      if (nickname == null) {
        setState(() {
          _loading = false;
          _success = false;
          _message = null;
        });
        return;
      }

      widget.onImported(parsed,
          url: input.startsWith('http') ? input : '', nickname: nickname);
      if (!mounted) return;
      setState(() {
        _loading = false;
        _success = true;
        _message = AppLocalizations.of(context).importedNodes(parsed.proxies.length, nickname);
      });
    } catch (e) {
      logger.error('subscription', 'Import error: $e');
      if (!mounted) return;
      setState(() {
        _loading = false;
        _success = false;
        _message = AppLocalizations.of(context).connError(e.toString());
      });
    }
  }

  Future<void> _importFromPaste() async {
    final content = _pasteController.text.trim();
    if (content.isEmpty) return;
    final logger = AppLogger.instance;
    setState(() {
      _pasteLoading = true;
      _pasteMessage = null;
    });
    try {
      final parsed = SubscriptionParser.parse(content);
      logger.info('subscription', 'Parsed pasted nodes',
          fields: {'count': parsed.proxies.length, 'has_custom_rules': parsed.hasCustomRules});
      if (parsed.proxies.isEmpty) {
        if (!mounted) return;
        setState(() {
          _pasteLoading = false;
          _pasteSuccess = false;
          _pasteMessage = AppLocalizations.of(context).noValidNodes;
        });
        return;
      }

      final defaultName = await SubscriptionStore.generateDefaultNickname();
      if (!mounted) return;
      final nickname = await _showNicknameDialog(defaultName);
      if (!mounted) return;
      if (nickname == null) {
        setState(() {
          _pasteLoading = false;
          _pasteSuccess = false;
          _pasteMessage = null;
        });
        return;
      }

      widget.onImported(parsed, url: '', nickname: nickname);
      if (!mounted) return;
      setState(() {
        _pasteLoading = false;
        _pasteSuccess = true;
        _pasteMessage = AppLocalizations.of(context).importedFromPaste(parsed.proxies.length);
      });
    } catch (e) {
      logger.error('subscription', 'Paste import error: $e');
      if (!mounted) return;
      setState(() {
        _pasteLoading = false;
        _pasteSuccess = false;
        _pasteMessage = AppLocalizations.of(context).parseFailedMsg(e);
      });
    }
  }

  String _redactSubscriptionUrl(String raw) {
    try {
      final uri = Uri.parse(raw);
      if (!uri.hasQuery) return raw;
      final redactedQuery = <String, String>{};
      for (final entry in uri.queryParameters.entries) {
        final key = entry.key.toLowerCase();
        if (key.contains('token') ||
            key.contains('key') ||
            key.contains('secret')) {
          redactedQuery[entry.key] = '***';
        } else {
          redactedQuery[entry.key] = entry.value;
        }
      }
      return uri.replace(queryParameters: redactedQuery).toString();
    } catch (_) {
      return raw;
    }
  }

  Future<String?> _showNicknameDialog(String defaultName) {
    final controller = TextEditingController(text: defaultName);
    final l10n = AppLocalizations.of(context);
    return showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: Text(l10n.subNameDialogTitle),
        content: TextField(
          controller: controller,
          autofocus: false,
          style: const TextStyle(color: _kTextHi, fontSize: 14),
          decoration: InputDecoration(
            hintText: l10n.subNameHint,
            hintStyle: const TextStyle(color: _kTextFaint),
            filled: true,
            fillColor: _kBg,
            contentPadding:
                const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
            border: OutlineInputBorder(
                borderRadius: BorderRadius.circular(10),
                borderSide: const BorderSide(color: _kBorder)),
            enabledBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(10),
                borderSide: const BorderSide(color: _kBorder)),
            focusedBorder: OutlineInputBorder(
                borderRadius: BorderRadius.circular(10),
                borderSide: const BorderSide(color: _kBrand)),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: Text(l10n.btnCancel, style: const TextStyle(color: _kTextMuted)),
          ),
          FilledButton(
            key: const Key('save_nickname'),
            style: FilledButton.styleFrom(
              backgroundColor: _kBrand,
              foregroundColor: Colors.white,
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(8)),
            ),
            onPressed: () {
              final name = controller.text.trim();
              Navigator.pop(ctx, name.isEmpty ? defaultName : name);
            },
            child: Text(l10n.btnSave),
          ),
        ],
      ),
    );
  }

  String _domainLabel(String url) {
    if (url.isEmpty) return '';
    try {
      final host = Uri.parse(url).host;
      return host.isEmpty ? '' : ' · $host';
    } catch (_) {
      return '';
    }
  }

  Future<void> _confirmDelete(Subscription sub) async {
    final l10n = AppLocalizations.of(context);
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(l10n.deleteSubTitle),
        content: Text(l10n.deleteSubContent(sub.nickname)),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: Text(l10n.btnCancel, style: const TextStyle(color: _kTextMuted)),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: _kError,
              foregroundColor: Colors.white,
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(8)),
            ),
            onPressed: () => Navigator.pop(ctx, true),
            child: Text(l10n.btnDelete),
          ),
        ],
      ),
    );
    if (confirmed == true && mounted) widget.onDelete(sub);
  }

  Widget _buildSubCard(Subscription sub) {
    final isActive = sub.id == widget.activeSubId;
    return Container(
      margin: const EdgeInsets.only(bottom: 10),
      decoration: BoxDecoration(
        color: _kCard,
        borderRadius: BorderRadius.circular(14),
        border: Border.all(
          color: isActive ? _kBrand.withAlpha(200) : _kBorder,
          width: isActive ? 1.5 : 1,
        ),
      ),
      child: InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: isActive ? null : () => widget.onActivate(sub),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
          child: Row(
            children: [
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        if (isActive)
                          Container(
                            width: 7,
                            height: 7,
                            margin: const EdgeInsets.only(right: 6),
                            decoration: const BoxDecoration(
                              color: _kBrand,
                              shape: BoxShape.circle,
                            ),
                          ),
                        Flexible(
                          child: Text(
                            sub.nickname,
                            style: const TextStyle(
                                color: _kTextHi,
                                fontWeight: FontWeight.w600,
                                fontSize: 14),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        if (isActive) ...[
                          const SizedBox(width: 8),
                          Builder(builder: (ctx) {
                            final l10n = AppLocalizations.of(ctx);
                            return Container(
                              padding: const EdgeInsets.symmetric(
                                  horizontal: 7, vertical: 2),
                              decoration: BoxDecoration(
                                color: _kBrand.withAlpha(30),
                                borderRadius: BorderRadius.circular(4),
                                border: Border.all(color: _kBrand.withAlpha(80)),
                              ),
                              child: Text(l10n.activeLabel,
                                  style: const TextStyle(
                                      color: _kBrand,
                                      fontSize: 10,
                                      fontWeight: FontWeight.w600)),
                            );
                          }),
                        ],
                      ],
                    ),
                    const SizedBox(height: 4),
                    Builder(builder: (ctx) {
                      final l10n = AppLocalizations.of(ctx);
                      return Text(
                        '${l10n.nodesCountSub(sub.nodes.length)}${sub.isBuiltIn ? '' : _domainLabel(sub.url)}',
                        style: const TextStyle(color: _kTextFaint, fontSize: 12),
                      );
                    }),
                  ],
                ),
              ),
              if (!isActive) ...[
                const SizedBox(width: 8),
                Builder(builder: (ctx) {
                  final l10n = AppLocalizations.of(ctx);
                  return TextButton(
                    onPressed: () => widget.onActivate(sub),
                    style: TextButton.styleFrom(
                      padding:
                          const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                      minimumSize: Size.zero,
                      tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                      foregroundColor: _kBrand,
                    ),
                    child: Text(l10n.btnSwitch,
                        style:
                            const TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                  );
                }),
              ],
              IconButton(
                icon: const Icon(Icons.delete_outline,
                    color: _kTextFaint, size: 18),
                onPressed: () => _confirmDelete(sub),
                padding: EdgeInsets.zero,
                constraints: const BoxConstraints(minWidth: 32, minHeight: 32),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return SafeArea(
      child: ListView(
        padding: const EdgeInsets.symmetric(horizontal: 24),
        children: [
          const SizedBox(height: 20),
          Text(l10n.subscriptionsTitle,
              style: const TextStyle(
                  color: _kTextHi,
                  fontSize: 24,
                  fontWeight: FontWeight.bold,
                  letterSpacing: -0.5)),
          const SizedBox(height: 24),

          // URL import card
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              gradient: const LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [_kCardGrad, _kCard],
              ),
              borderRadius: BorderRadius.circular(18),
              border: Border.all(color: _kBorder),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 32,
                      height: 32,
                      decoration: BoxDecoration(
                        color: _kBrand.withAlpha(22),
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(color: _kBrand.withAlpha(60)),
                      ),
                      child: const Icon(Icons.link, color: _kBrand, size: 17),
                    ),
                    const SizedBox(width: 10),
                    Text(l10n.subscriptionUrlLabel,
                        style: const TextStyle(
                            color: _kTextFaint,
                            fontSize: 11,
                            letterSpacing: 1.2,
                            fontWeight: FontWeight.w600)),
                  ],
                ),
                const SizedBox(height: 14),
                TextField(
                  key: const Key('subscription_url_field'),
                  controller: _urlController,
                  style: const TextStyle(color: _kTextHi, fontSize: 14),
                  decoration: InputDecoration(
                    hintText: 'https://',
                    hintStyle: const TextStyle(color: _kTextFaint),
                    filled: true,
                    fillColor: _kBg,
                    contentPadding: const EdgeInsets.symmetric(
                        horizontal: 14, vertical: 12),
                    border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: _kBorder)),
                    enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: _kBorder)),
                    focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide:
                            const BorderSide(color: _kBrand, width: 1.5)),
                    suffixIcon: _urlController.text.isNotEmpty
                        ? IconButton(
                            icon: const Icon(Icons.clear,
                                color: _kTextFaint, size: 18),
                            onPressed: () {
                              _urlController.clear();
                              setState(() {});
                            },
                          )
                        : null,
                  ),
                  onChanged: (_) => setState(() {}),
                ),
                const SizedBox(height: 14),
                SizedBox(
                  width: double.infinity,
                  height: 46,
                  child: FilledButton.icon(
                    onPressed: _loading ? null : _import,
                    style: FilledButton.styleFrom(
                      backgroundColor: _kBrand,
                      foregroundColor: Colors.white,
                      disabledBackgroundColor: _kBrand.withAlpha(80),
                      shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(11)),
                    ),
                    icon: _loading
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(
                                strokeWidth: 2, color: Colors.white54))
                        : const Icon(Icons.cloud_download, size: 18),
                    label: Text(_loading ? l10n.btnFetching : l10n.btnImport,
                        style: const TextStyle(fontWeight: FontWeight.w600)),
                  ),
                ),
              ],
            ),
          ),

          const SizedBox(height: 14),

          // Paste nodes card
          Container(
            padding: const EdgeInsets.all(20),
            decoration: BoxDecoration(
              gradient: const LinearGradient(
                begin: Alignment.topLeft,
                end: Alignment.bottomRight,
                colors: [_kCardGrad, _kCard],
              ),
              borderRadius: BorderRadius.circular(18),
              border: Border.all(color: _kBorder),
            ),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Container(
                      width: 32,
                      height: 32,
                      decoration: BoxDecoration(
                        color: _kConnected.withAlpha(22),
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(color: _kConnected.withAlpha(60)),
                      ),
                      child: const Icon(Icons.content_paste, color: _kConnected, size: 17),
                    ),
                    const SizedBox(width: 10),
                    Text(l10n.pasteNodesLabel,
                        style: const TextStyle(
                            color: _kTextFaint,
                            fontSize: 11,
                            letterSpacing: 1.2,
                            fontWeight: FontWeight.w600)),
                  ],
                ),
                const SizedBox(height: 8),
                Text(
                  l10n.pasteNodesDesc,
                  style: const TextStyle(color: _kTextMuted, fontSize: 12, height: 1.4),
                ),
                const SizedBox(height: 12),
                TextField(
                  controller: _pasteController,
                  maxLines: 5,
                  minLines: 3,
                  style: const TextStyle(color: _kTextHi, fontSize: 12, fontFamily: 'monospace'),
                  decoration: InputDecoration(
                    hintText: 'ss://...\nvmess://...\nvless://...',
                    hintStyle: const TextStyle(color: _kTextFaint, fontSize: 12),
                    filled: true,
                    fillColor: _kBg,
                    contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                    border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: _kBorder)),
                    enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: _kBorder)),
                    focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: _kConnected, width: 1.5)),
                  ),
                  onChanged: (_) => setState(() {}),
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  height: 46,
                  child: FilledButton.icon(
                    onPressed: _pasteLoading || _pasteController.text.trim().isEmpty
                        ? null
                        : _importFromPaste,
                    style: FilledButton.styleFrom(
                      backgroundColor: _kConnected,
                      foregroundColor: Colors.black87,
                      disabledBackgroundColor: _kConnected.withAlpha(60),
                      shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(11)),
                    ),
                    icon: _pasteLoading
                        ? const SizedBox(
                            width: 16,
                            height: 16,
                            child: CircularProgressIndicator(
                                strokeWidth: 2, color: Colors.black54))
                        : const Icon(Icons.bolt, size: 18),
                    label: Text(_pasteLoading ? l10n.btnParsing : l10n.btnImportAndGenerate,
                        style: const TextStyle(fontWeight: FontWeight.w700)),
                  ),
                ),
                if (_pasteMessage != null) ...[
                  const SizedBox(height: 12),
                  AnimatedContainer(
                    duration: const Duration(milliseconds: 250),
                    padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
                    decoration: BoxDecoration(
                      color: (_pasteSuccess ? _kConnected : _kError).withAlpha(15),
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(
                          color: (_pasteSuccess ? _kConnected : _kError).withAlpha(70)),
                    ),
                    child: Row(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
                        Icon(
                            _pasteSuccess
                                ? Icons.check_circle_outline
                                : Icons.error_outline,
                            color: _pasteSuccess ? _kConnected : _kError,
                            size: 16),
                        const SizedBox(width: 8),
                        Expanded(
                          child: Text(_pasteMessage!,
                              style: TextStyle(
                                  color: _pasteSuccess ? _kConnected : _kError,
                                  fontSize: 12,
                                  height: 1.4)),
                        ),
                      ],
                    ),
                  ),
                ],
              ],
            ),
          ),

          // URL result banner
          if (_message != null) ...[
            const SizedBox(height: 14),
            AnimatedContainer(
              duration: const Duration(milliseconds: 250),
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              decoration: BoxDecoration(
                color: (_success ? _kConnected : _kError).withAlpha(15),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(
                    color: (_success ? _kConnected : _kError).withAlpha(70)),
              ),
              child: Row(
                children: [
                  Icon(
                      _success
                          ? Icons.check_circle_outline
                          : Icons.error_outline,
                      color: _success ? _kConnected : _kError,
                      size: 18),
                  const SizedBox(width: 10),
                  Expanded(
                    child: Text(_message!,
                        style: TextStyle(
                            color: _success ? _kConnected : _kError,
                            fontSize: 13)),
                  ),
                ],
              ),
            ),
          ],

          // Saved subscriptions list
          if (widget.subscriptions.isNotEmpty) ...[
            const SizedBox(height: 28),
            Text(l10n.savedSubscriptions,
                style: const TextStyle(
                    color: _kTextMuted,
                    fontSize: 12,
                    letterSpacing: 0.8,
                    fontWeight: FontWeight.w600)),
            const SizedBox(height: 12),
            ...widget.subscriptions.map(_buildSubCard),
          ],

          const SizedBox(height: 20),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 4 — Settings
// ─────────────────────────────────────────────────────────────
class _SettingsTab extends StatelessWidget {
  const _SettingsTab({
    required this.nodeCount,
    required this.onLocaleChanged,
  });
  final int nodeCount;
  final void Function(Locale) onLocaleChanged;

  void _showLanguagePicker(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final current = Localizations.localeOf(context).languageCode;
    showModalBottomSheet(
      context: context,
      backgroundColor: _kCard,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
      ),
      builder: (_) => Padding(
        padding: const EdgeInsets.fromLTRB(24, 16, 24, 40),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Center(
              child: Container(
                width: 36,
                height: 4,
                decoration: BoxDecoration(
                    color: _kBorder, borderRadius: BorderRadius.circular(2)),
              ),
            ),
            const SizedBox(height: 20),
            Text(l10n.tileLanguageTitle,
                style: const TextStyle(
                    color: _kTextHi,
                    fontSize: 18,
                    fontWeight: FontWeight.bold)),
            const SizedBox(height: 16),
            _LanguageOption(
              label: l10n.langEnglish,
              selected: current == 'en',
              onTap: () {
                Navigator.pop(context);
                onLocaleChanged(const Locale('en'));
              },
            ),
            const SizedBox(height: 8),
            _LanguageOption(
              label: l10n.langChinese,
              selected: current == 'zh',
              onTap: () {
                Navigator.pop(context);
                onLocaleChanged(const Locale('zh'));
              },
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 24, 24),
            child: Text(l10n.settingsTitle,
                style: const TextStyle(
                    color: _kTextHi,
                    fontSize: 24,
                    fontWeight: FontWeight.bold,
                    letterSpacing: -0.5)),
          ),
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Column(
              children: [
                _SettingsTile(
                  iconColor: const Color(0xFF22D3EE),
                  icon: Icons.terminal_outlined,
                  title: l10n.tileLogsTitle,
                  subtitle: l10n.tileLogsSubtitle,
                  onTap: () => Navigator.push(
                      context,
                      MaterialPageRoute(
                        builder: (_) => const Scaffold(
                          backgroundColor: _kBg,
                          body: _LogsTab(),
                        ),
                      )),
                ),
                const SizedBox(height: 8),
                _SettingsTile(
                  iconColor: const Color(0xFF34D399),
                  icon: Icons.system_update_alt_outlined,
                  title: l10n.tileUpdatesTitle,
                  subtitle: l10n.tileUpdatesSubtitle,
                  onTap: () => showModalBottomSheet(
                    context: context,
                    backgroundColor: _kCard,
                    shape: const RoundedRectangleBorder(
                      borderRadius:
                          BorderRadius.vertical(top: Radius.circular(20)),
                    ),
                    builder: (_) => const _UpdateSheet(),
                  ),
                ),
                const SizedBox(height: 8),
                _SettingsTile(
                  iconColor: _kBrand,
                  icon: Icons.info_outline,
                  title: l10n.tileAboutTitle,
                  subtitle: l10n.tileAboutSubtitle,
                  onTap: () => Navigator.push(
                      context,
                      MaterialPageRoute(
                        builder: (_) => Scaffold(
                          backgroundColor: _kBg,
                          body: _AboutTab(nodeCount: nodeCount),
                        ),
                      )),
                ),
                const SizedBox(height: 8),
                _SettingsTile(
                  iconColor: const Color(0xFFFFB74D),
                  icon: Icons.language_outlined,
                  title: l10n.tileLanguageTitle,
                  subtitle: l10n.tileLanguageSubtitle,
                  onTap: () => _showLanguagePicker(context),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _LanguageOption extends StatelessWidget {
  const _LanguageOption({
    required this.label,
    required this.selected,
    required this.onTap,
  });
  final String label;
  final bool selected;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(12),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          color: selected ? _kBrand.withAlpha(20) : _kBg,
          borderRadius: BorderRadius.circular(12),
          border: Border.all(
              color: selected ? _kBrand.withAlpha(160) : _kBorder),
        ),
        child: Row(
          children: [
            Text(label,
                style: TextStyle(
                    color: selected ? _kBrand : _kTextHi,
                    fontSize: 15,
                    fontWeight:
                        selected ? FontWeight.w600 : FontWeight.normal)),
            const Spacer(),
            if (selected)
              const Icon(Icons.check_circle, color: _kBrand, size: 18),
          ],
        ),
      ),
    );
  }
}

class _SettingsTile extends StatelessWidget {
  const _SettingsTile({
    required this.iconColor,
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.onTap,
  });

  final Color iconColor;
  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return InkWell(
      borderRadius: BorderRadius.circular(14),
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
        decoration: BoxDecoration(
          gradient: const LinearGradient(
            begin: Alignment.topLeft,
            end: Alignment.bottomRight,
            colors: [_kCardGrad, _kCard],
          ),
          borderRadius: BorderRadius.circular(14),
          border: Border.all(color: _kBorder),
        ),
        child: Row(
          children: [
            Container(
              width: 42,
              height: 42,
              decoration: BoxDecoration(
                color: iconColor.withAlpha(22),
                borderRadius: BorderRadius.circular(11),
                border: Border.all(color: iconColor.withAlpha(60)),
              ),
              child: Icon(icon, color: iconColor, size: 20),
            ),
            const SizedBox(width: 14),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(title,
                      style: const TextStyle(
                          color: _kTextHi,
                          fontWeight: FontWeight.w600,
                          fontSize: 15)),
                  const SizedBox(height: 2),
                  Text(subtitle,
                      style: const TextStyle(color: _kTextMuted, fontSize: 13)),
                ],
              ),
            ),
            const Icon(Icons.chevron_right, color: _kTextFaint, size: 20),
          ],
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Logs screen (also reachable from Settings)
// ─────────────────────────────────────────────────────────────
class _LogsTab extends StatefulWidget {
  const _LogsTab();

  @override
  State<_LogsTab> createState() => _LogsTabState();
}

class _LogsTabState extends State<_LogsTab> {
  final ScrollController _scroll = ScrollController();
  String _filter = 'all';
  bool _autoScroll = true;

  static const _levels = ['all', 'debug', 'info', 'warn', 'error'];
  static const _levelColors = {
    'debug': Color(0xFF9B8CC4),
    'info': _kBrand,
    'warn': Color(0xFFFFB74D),
    'error': _kError,
  };

  @override
  void initState() {
    super.initState();
    AppLogger.instance.addListener(_onLog);
  }

  @override
  void dispose() {
    AppLogger.instance.removeListener(_onLog);
    _scroll.dispose();
    super.dispose();
  }

  void _onLog() {
    setState(() {});
    if (_autoScroll && _scroll.hasClients) {
      WidgetsBinding.instance.addPostFrameCallback((_) {
        if (_scroll.hasClients) {
          _scroll.animateTo(_scroll.position.maxScrollExtent,
              duration: const Duration(milliseconds: 150),
              curve: Curves.easeOut);
        }
      });
    }
  }

  List<LogEntry> get _visible {
    final all = AppLogger.instance.entries;
    if (_filter == 'all') return all;
    return all.where((e) => e.level == _filter).toList();
  }

  Future<void> _copyAll() async {
    await Clipboard.setData(ClipboardData(text: AppLogger.instance.export()));
    if (mounted) {
      final l10n = AppLocalizations.of(context);
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
            content: Text(l10n.logsCopied),
            duration: const Duration(seconds: 2)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final entries = _visible;

    final l10n = AppLocalizations.of(context);
    return SafeArea(
      child: Column(
        children: [
          // Header
          Padding(
            padding: const EdgeInsets.fromLTRB(8, 14, 8, 8),
            child: Row(
              children: [
                if (Navigator.canPop(context))
                  IconButton(
                    icon: const Icon(Icons.arrow_back, color: _kTextMuted),
                    onPressed: () => Navigator.pop(context),
                    padding: EdgeInsets.zero,
                  ),
                Text(l10n.logsTitle,
                    style: const TextStyle(
                        color: _kTextHi,
                        fontSize: 22,
                        fontWeight: FontWeight.bold,
                        letterSpacing: -0.5)),
                const Spacer(),
                IconButton(
                  tooltip: l10n.tooltipCopyAll,
                  icon: const Icon(Icons.copy_outlined,
                      size: 19, color: _kTextMuted),
                  onPressed: _copyAll,
                ),
                IconButton(
                  tooltip: _autoScroll ? l10n.tooltipAutoScrollOn : l10n.tooltipAutoScrollOff,
                  icon: Icon(Icons.vertical_align_bottom,
                      size: 19, color: _autoScroll ? _kBrand : _kTextFaint),
                  onPressed: () => setState(() => _autoScroll = !_autoScroll),
                ),
                IconButton(
                  tooltip: l10n.tooltipClear,
                  icon: const Icon(Icons.delete_outline,
                      size: 19, color: _kTextMuted),
                  onPressed: () {
                    AppLogger.instance.clear();
                    setState(() {});
                  },
                ),
              ],
            ),
          ),

          // Level filter chips
          SizedBox(
            height: 32,
            child: ListView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.symmetric(horizontal: 14),
              children: _levels.map((lvl) {
                final active = _filter == lvl;
                final color =
                    lvl == 'all' ? _kBrand : (_levelColors[lvl] ?? _kTextMuted);
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: GestureDetector(
                    onTap: () => setState(() => _filter = lvl),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 150),
                      padding: const EdgeInsets.symmetric(
                          horizontal: 12, vertical: 4),
                      decoration: BoxDecoration(
                        color:
                            active ? color.withAlpha(28) : Colors.transparent,
                        borderRadius: BorderRadius.circular(20),
                        border: Border.all(color: active ? color : _kBorder),
                      ),
                      child: Text(
                        lvl.toUpperCase(),
                        style: TextStyle(
                          color: active ? color : _kTextFaint,
                          fontSize: 11,
                          fontWeight: FontWeight.w700,
                          letterSpacing: 0.8,
                        ),
                      ),
                    ),
                  ),
                );
              }).toList(),
            ),
          ),
          const SizedBox(height: 8),

          Expanded(
            child: entries.isEmpty
                ? Center(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Icon(Icons.terminal,
                            size: 40, color: _kTextFaint),
                        const SizedBox(height: 12),
                        Text(
                          _filter == 'all'
                              ? l10n.noLogsYet
                              : l10n.noFilterLogs(_filter),
                          style: const TextStyle(color: _kTextMuted),
                        ),
                      ],
                    ),
                  )
                : ListView.builder(
                    controller: _scroll,
                    padding:
                        const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                    itemCount: entries.length,
                    itemBuilder: (context, i) => _LogRow(entry: entries[i]),
                  ),
          ),
        ],
      ),
    );
  }
}

class _LogRow extends StatelessWidget {
  const _LogRow({required this.entry});
  final LogEntry entry;

  static const _bg = {
    'debug': Color(0xFF9B8CC4),
    'info': _kBrand,
    'warn': Color(0xFFFFB74D),
    'error': _kError,
  };

  @override
  Widget build(BuildContext context) {
    final color = _bg[entry.level] ?? const Color(0xFF9B8CC4);

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(entry.timeLabel,
              style: const TextStyle(
                  color: _kTextFaint, fontSize: 10, fontFamily: 'monospace')),
          const SizedBox(width: 6),
          Container(
            width: 38,
            padding: const EdgeInsets.symmetric(vertical: 1),
            decoration: BoxDecoration(
              color: color.withAlpha(22),
              borderRadius: BorderRadius.circular(4),
              border: Border.all(color: color.withAlpha(70), width: 0.5),
            ),
            child: Text(
              entry.level.toUpperCase(),
              textAlign: TextAlign.center,
              style: TextStyle(
                  color: color,
                  fontSize: 9,
                  fontWeight: FontWeight.w800,
                  letterSpacing: 0.5),
            ),
          ),
          const SizedBox(width: 6),
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
            decoration: BoxDecoration(
              color: _kBorder.withAlpha(100),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(entry.component,
                style: const TextStyle(
                    color: _kTextMuted, fontSize: 10, fontFamily: 'monospace')),
          ),
          const SizedBox(width: 6),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(entry.message,
                    style: const TextStyle(
                        color: _kTextHi,
                        fontSize: 12,
                        fontFamily: 'monospace')),
                if (entry.fields.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Wrap(
                    spacing: 4,
                    runSpacing: 3,
                    children: entry.fields.entries
                        .map((kv) => Container(
                              padding: const EdgeInsets.symmetric(
                                  horizontal: 5, vertical: 1),
                              decoration: BoxDecoration(
                                color: const Color(0xFF1E1C3A),
                                borderRadius: BorderRadius.circular(3),
                              ),
                              child: Text('${kv.key}=${kv.value}',
                                  style: const TextStyle(
                                      color: _kTextMuted,
                                      fontSize: 10,
                                      fontFamily: 'monospace')),
                            ))
                        .toList(),
                  ),
                ],
              ],
            ),
          ),
        ],
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// About screen (reachable from Settings)
// ─────────────────────────────────────────────────────────────
class _AboutTab extends StatefulWidget {
  const _AboutTab({required this.nodeCount});
  final int nodeCount;

  @override
  State<_AboutTab> createState() => _AboutTabState();
}

class _AboutTabState extends State<_AboutTab> {
  Map<String, dynamic> _info = {};
  bool _loading = false;

  UpdateInfo? _updateInfo;
  bool _updateChecking = false;
  bool _updateChecked = false;

  @override
  void initState() {
    super.initState();
    _refresh();
    _checkUpdate();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    final info = await VpnManager.getSystemInfo();
    if (mounted) {
      setState(() {
        _info = info;
        _loading = false;
      });
    }
  }

  Future<void> _checkUpdate() async {
    if (_updateChecking) return;
    setState(() {
      _updateChecking = true;
      _updateChecked = false;
    });
    final info = await fetchLatestRelease();
    if (mounted) {
      setState(() {
        _updateInfo = info;
        _updateChecking = false;
        _updateChecked = true;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    Widget row(String label, String value, {Color? valueColor}) => Padding(
          padding: const EdgeInsets.symmetric(vertical: 10),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: [
              Text(label,
                  style: const TextStyle(color: _kTextMuted, fontSize: 14)),
              Text(value,
                  style: TextStyle(
                      color: valueColor ?? _kTextHi,
                      fontSize: 14,
                      fontWeight: FontWeight.w600)),
            ],
          ),
        );

    final mihomoRunning = _info['mihomo_running'] as bool? ?? false;
    final vpnRunning = _info['vpn_running'] as bool? ?? false;
    final pid = _info['mihomo_pid'] as int? ?? -1;
    final appVersion = _info['app_version'] as String? ?? '—';
    final buildNum = _info['build_number'];
    final appPss = _info['memory_app_pss_mb'] as double? ?? 0.0;
    final avail = _info['memory_available_mb'] as double? ?? 0.0;
    final abi = _info['device_abi'] as String? ?? '—';

    Widget section(String title, List<Widget> rows) => Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          decoration: BoxDecoration(
            gradient: const LinearGradient(
              begin: Alignment.topLeft,
              end: Alignment.bottomRight,
              colors: [_kCardGrad, _kCard],
            ),
            borderRadius: BorderRadius.circular(16),
            border: Border.all(color: _kBorder),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Padding(
                padding: const EdgeInsets.only(top: 8, bottom: 4),
                child: Text(title,
                    style: const TextStyle(
                        color: _kTextFaint,
                        fontSize: 11,
                        letterSpacing: 1.2,
                        fontWeight: FontWeight.w600)),
              ),
              const Divider(color: _kBorder, height: 1),
              ...rows,
            ],
          ),
        );

    return SafeArea(
      child: RefreshIndicator(
        onRefresh: _refresh,
        color: _kBrand,
        child: ListView(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          children: [
            const SizedBox(height: 20),
            Row(children: [
              if (Navigator.canPop(context))
                IconButton(
                  icon: const Icon(Icons.arrow_back, color: _kTextMuted),
                  onPressed: () => Navigator.pop(context),
                  padding: EdgeInsets.zero,
                ),
              Text(l10n.aboutTitle,
                  style: const TextStyle(
                      color: _kTextHi,
                      fontSize: 24,
                      fontWeight: FontWeight.bold,
                      letterSpacing: -0.5)),
              const Spacer(),
              if (_loading)
                const SizedBox(
                    width: 18,
                    height: 18,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: _kBrand))
              else
                IconButton(
                    icon:
                        const Icon(Icons.refresh, color: _kTextMuted, size: 20),
                    onPressed: _refresh,
                    padding: EdgeInsets.zero),
            ]),
            const SizedBox(height: 20),
            section(l10n.sectionApplication, [
              row(l10n.rowVersion, '$appVersion ($buildNum)'),
              row(l10n.rowNodesLoaded, '${widget.nodeCount}'),
              row(l10n.rowDeviceAbi, abi),
            ]),
            const SizedBox(height: 14),
            section(l10n.sectionUpdate, [
              if (_updateChecking)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 12),
                  child: Row(children: [
                    const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: _kBrand)),
                    const SizedBox(width: 12),
                    Text(l10n.checkingUpdates,
                        style: const TextStyle(color: _kTextMuted, fontSize: 14)),
                  ]),
                )
              else if (!_updateChecked)
                row(l10n.updateStatus, '—')
              else if (_updateInfo == null)
                row(l10n.updateStatus, l10n.updateCouldNotCheck, valueColor: _kError)
              else if (!_updateInfo!.isNewerThan(appVersion))
                row(l10n.updateStatus, l10n.updateUpToDate, valueColor: _kConnected)
              else ...[
                row(l10n.rowLatest, _updateInfo!.tag, valueColor: _kBrand),
                Padding(
                  padding: const EdgeInsets.only(bottom: 10, top: 4),
                  child: SizedBox(
                    width: double.infinity,
                    child: ElevatedButton.icon(
                      icon: const Icon(Icons.download_rounded, size: 18),
                      label: Text(l10n.downloadVersion(_updateInfo!.tag)),
                      onPressed: () => _launchUrl(_updateInfo!.htmlUrl),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: _kBrand,
                        foregroundColor: Colors.white,
                        padding: const EdgeInsets.symmetric(vertical: 12),
                        shape: RoundedRectangleBorder(
                            borderRadius: BorderRadius.circular(10)),
                      ),
                    ),
                  ),
                ),
              ],
              if (_updateChecked && _updateInfo != null)
                Padding(
                  padding: const EdgeInsets.only(bottom: 4),
                  child: TextButton(
                    onPressed: _checkUpdate,
                    style: TextButton.styleFrom(padding: EdgeInsets.zero),
                    child: Text(l10n.btnRecheck2,
                        style: const TextStyle(color: _kTextFaint, fontSize: 12)),
                  ),
                ),
            ]),
            const SizedBox(height: 14),
            section(l10n.sectionRuntime, [
              row(l10n.rowVpn, vpnRunning ? l10n.rowRunning : l10n.rowStopped,
                  valueColor: vpnRunning ? _kConnected : _kTextMuted),
              row(l10n.rowMihomo,
                  mihomoRunning ? l10n.rowMihomoRunning(pid) : l10n.rowStopped,
                  valueColor: mihomoRunning ? _kConnected : _kTextMuted),
            ]),
            const SizedBox(height: 14),
            section(l10n.sectionMemory, [
              row(l10n.rowAppPss, '${appPss.toStringAsFixed(1)} MB'),
              row(l10n.rowAvailable, '${avail.toStringAsFixed(0)} MB'),
            ]),
            const SizedBox(height: 20),
            Center(
                child: Text(l10n.pullDownToRefresh,
                    style: const TextStyle(color: _kTextFaint, fontSize: 12))),
          ],
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Update check bottom sheet (from Settings → Check for Updates)
// ─────────────────────────────────────────────────────────────
class _UpdateSheet extends StatefulWidget {
  const _UpdateSheet();

  @override
  State<_UpdateSheet> createState() => _UpdateSheetState();
}

class _UpdateSheetState extends State<_UpdateSheet> {
  UpdateInfo? _info;
  bool _loading = true;
  String _currentVersion = '';

  @override
  void initState() {
    super.initState();
    _load();
  }

  Future<void> _load() async {
    final sysInfo = await VpnManager.getSystemInfo();
    final cv = sysInfo['app_version'] as String? ?? '';
    final info = await fetchLatestRelease();
    if (mounted) {
      setState(() {
        _currentVersion = cv;
        _info = info;
        _loading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final isNewer = _info != null && _info!.isNewerThan(_currentVersion);

    return Padding(
      padding: EdgeInsets.fromLTRB(
          24, 16, 24, MediaQuery.of(context).viewInsets.bottom + 32),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Center(
            child: Container(
              width: 36,
              height: 4,
              decoration: BoxDecoration(
                  color: _kBorder, borderRadius: BorderRadius.circular(2)),
            ),
          ),
          const SizedBox(height: 20),
          Row(children: [
            Container(
              width: 40,
              height: 40,
              decoration: BoxDecoration(
                color: const Color(0xFF34D399).withAlpha(22),
                borderRadius: BorderRadius.circular(10),
                border:
                    Border.all(color: const Color(0xFF34D399).withAlpha(60)),
              ),
              child: const Icon(Icons.system_update_alt_outlined,
                  color: Color(0xFF34D399), size: 20),
            ),
            const SizedBox(width: 12),
            Text(l10n.updateSheetTitle,
                style: const TextStyle(
                    color: _kTextHi,
                    fontSize: 18,
                    fontWeight: FontWeight.bold)),
          ]),
          const SizedBox(height: 24),
          if (_loading) ...[
            const Center(child: CircularProgressIndicator(color: _kBrand)),
            const SizedBox(height: 12),
            Center(
                child: Text(l10n.updateChecking,
                    style: const TextStyle(color: _kTextMuted, fontSize: 13))),
          ] else if (_info == null) ...[
            Row(children: [
              const Icon(Icons.warning_amber_rounded, color: _kError, size: 22),
              const SizedBox(width: 10),
              Expanded(
                child: Text(
                    l10n.updateCouldNotCheckLong,
                    style: const TextStyle(color: _kTextMuted, fontSize: 14)),
              ),
            ]),
          ] else if (!isNewer) ...[
            Row(children: [
              const Icon(Icons.check_circle_outline,
                  color: _kConnected, size: 22),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(l10n.updateUpToDateTitle,
                        style: const TextStyle(
                            color: _kConnected,
                            fontSize: 15,
                            fontWeight: FontWeight.w600)),
                    Text(l10n.updateVersionLabel(_info!.tag),
                        style:
                            const TextStyle(color: _kTextMuted, fontSize: 13)),
                  ],
                ),
              ),
            ]),
          ] else ...[
            Row(children: [
              const Icon(Icons.new_releases_outlined, color: _kBrand, size: 22),
              const SizedBox(width: 10),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(l10n.updateAvailableTitle,
                        style: const TextStyle(
                            color: _kBrand,
                            fontSize: 15,
                            fontWeight: FontWeight.w600)),
                    if (_currentVersion.isNotEmpty)
                      Text('$_currentVersion  →  ${_info!.tag}',
                          style: const TextStyle(
                              color: _kTextMuted, fontSize: 13)),
                  ],
                ),
              ),
            ]),
            const SizedBox(height: 16),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton.icon(
                icon: const Icon(Icons.download_rounded, size: 18),
                label: Text(l10n.downloadBtn(_info!.tag)),
                onPressed: () => _launchUrl(_info!.htmlUrl),
                style: ElevatedButton.styleFrom(
                  backgroundColor: _kBrand,
                  foregroundColor: Colors.white,
                  padding: const EdgeInsets.symmetric(vertical: 14),
                  shape: RoundedRectangleBorder(
                      borderRadius: BorderRadius.circular(12)),
                ),
              ),
            ),
          ],
          const SizedBox(height: 8),
          if (!_loading)
            Center(
              child: TextButton(
                onPressed: () => _launchUrl(
                    'https://github.com/wujun4code/clashforge/releases'),
                child: Text(l10n.allReleases,
                    style: const TextStyle(color: _kTextMuted, fontSize: 13)),
              ),
            ),
        ],
      ),
    );
  }
}
