import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:pointycastle/export.dart' as pc;
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

Future<void> _launchUrl(String url) async {
  final uri = Uri.parse(url);
  if (await canLaunchUrl(uri)) {
    await launchUrl(uri, mode: LaunchMode.externalApplication);
  }
}

void main() {
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
class ClashForgeApp extends StatelessWidget {
  const ClashForgeApp({super.key});

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
      home: const HomeScreen(),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Root screen — shared state
// ─────────────────────────────────────────────────────────────
class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});
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

  // (name, description, url, domain-for-DNS-query)
  static const _domesticSites = [
    ('淘宝', '验证国内主要电商平台直连可达性', 'https://www.taobao.com', 'www.taobao.com'),
    ('网易云音乐', '验证国内常见内容站点延迟', 'https://music.163.com', 'music.163.com'),
  ];
  static const _foreignSites = [
    ('GitHub', '验证国际开发站点的代理访问效果', 'https://github.com', 'github.com'),
    ('Google', '验证 Google 搜索是否可通过代理访问', 'https://www.google.com', 'www.google.com'),
  ];
  static const _aiSites = [
    ('OpenAI', '验证 ChatGPT 是否可通过代理访问', 'https://chat.openai.com', 'chat.openai.com'),
    ('Claude', '验证 Claude AI 是否可通过代理访问', 'https://claude.ai', 'claude.ai'),
  ];

  int _tabIndex = 0;
  bool _isConnected = false;
  bool _isConnecting = false;
  bool _probeLoading = false;
  bool _browserDnsLoading = false;
  bool _switchingNode = false;
  String _connectionStatus = 'Tap to connect';
  String? _probeMessage;
  String? _browserDnsMessage;
  String? _privateDnsWarning;
  _ConnectivitySnapshot? _connectivitySnapshot;
  _BrowserDnsSnapshot? _browserDnsSnapshot;
  final List<ProxyNode> _nodes = [];
  ProxyNode? _selectedNode;
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
    if (_subscriptions.any((s) => s.url == url)) return;
    AppLogger.instance.info('app', 'Importing built-in subscription');
    try {
      final response =
          await http.get(Uri.parse(url)).timeout(const Duration(seconds: 20));
      if (response.statusCode != 200) return;
      final content = _decryptResponseOrFallback(response.bodyBytes, response.body);
      final parsed = SubscriptionParser.parse(content);
      if (parsed.proxies.isEmpty) return;
      _onNodesImported(parsed, url: url, nickname: 'Free');
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
      {String url = '', String nickname = ''}) {
    final id = '${DateTime.now().millisecondsSinceEpoch}';
    final sub = Subscription(
      id: id,
      nickname: nickname,
      url: url,
      nodes: parsed.proxies,
      customRules: parsed.rules,
      customProxyGroups: parsed.proxyGroups,
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
          _connectionStatus = 'Tap to connect';
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
        logger.info('vpn', 'Starting VPN', fields: {
          'node': _selectedNode!.name,
          'type': _selectedNode!.type,
          'server': _selectedNode!.server,
          'port': _selectedNode!.port,
        });
        final filesDir = await VpnManager.getFilesDir();
        Subscription? activeSub;
        for (final s in _subscriptions) {
          if (s.id == _activeSubscriptionId) { activeSub = s; break; }
        }
        final configMap = ConfigGenerator.generate(
          nodes: _nodes,
          geodataPath: filesDir,
          selectedNodeName: _selectedNode!.name,
          customRules: activeSub?.customRules ?? const [],
          customProxyGroups: activeSub?.customProxyGroups ?? const [],
        );
        final writeResult = await VpnManager.writeConfig(_mapToYaml(configMap));
        logger.debug('vpn', 'Config write result: $writeResult');

        final res = await VpnManager.startVpn();
        logger.info('vpn', 'VPN start result: $res');
        if (res == 'permission_needed') {
          setState(
              () => _connectionStatus = 'Grant VPN permission, then tap again');
        } else {
          setState(() {
            _isConnected = true;
            _connectionStatus = 'Connected';
            _probeMessage = null;
          });
          unawaited(_bootstrapAfterConnect());
        }
      }
    } catch (e, st) {
      logger.error('vpn', 'Toggle failed: $e',
          fields: {'stack': st.toString().split('\n').first});
      setState(() {
        _isConnected = false;
        _connectionStatus = 'Error: $e';
      });
    } finally {
      setState(() => _isConnecting = false);
    }
  }

  Future<void> _bootstrapAfterConnect() async {
    await _refreshPrivateDnsWarning();

    // Retry applying node selection until mihomo's controller is ready.
    // The controller typically starts within 1–2 s; 8 × 500 ms = up to 4 s.
    for (var attempt = 0; attempt < 8; attempt++) {
      await Future.delayed(const Duration(milliseconds: 500));
      if (!_isConnected || !mounted) return;
      try {
        final uri = Uri.parse(
          'http://$_kClashControllerHost:$_kClashControllerPort/proxies/'
          '${Uri.encodeComponent(_kMainProxyGroup)}',
        );
        await http
            .put(
              uri,
              headers: {'Content-Type': 'application/json'},
              body: jsonEncode({'name': _selectedNode?.name ?? ''}),
            )
            .timeout(const Duration(seconds: 2));
        if (mounted) {
          setState(() => _connectionStatus = 'Connected');
        }
        break; // controller responded — stop retrying
      } catch (_) {
        // controller not ready yet — keep retrying
      }
    }
    await _runConnectivityChecks();
    await _runBrowserDnsDiagnostics();
  }

  Future<void> _refreshPrivateDnsWarning() async {
    final info = await VpnManager.getSystemInfo();
    final mode =
        (info['private_dns_mode'] ?? '').toString().trim().toLowerCase();
    final specifier = (info['private_dns_specifier'] ?? '').toString().trim();

    String? nextWarning;
    if (mode == 'hostname' || mode == 'opportunistic') {
      final modeText = mode == 'hostname' ? '严格主机名模式' : '自动模式';
      final suffix =
          mode == 'hostname' && specifier.isNotEmpty ? ' ($specifier)' : '';
      nextWarning = '检测到系统 Private DNS $modeText$suffix，Android 可能无法劫持 DNS 请求，'
          '浏览器可能报 DNS_PROBE_FINISHED_BAD_CONFIG。请先在系统设置中关闭 Private DNS 后重试。';
    }

    if (!mounted) return;
    if (_privateDnsWarning == nextWarning) return;
    setState(() => _privateDnsWarning = nextWarning);
  }

  Future<void> _applyNodeSelectionIfRunning(
      {required bool triggerProbe}) async {
    if (!_isConnected || _selectedNode == null) return;

    final logger = AppLogger.instance;
    try {
      final uri = Uri.parse(
        'http://$_kClashControllerHost:$_kClashControllerPort/proxies/'
        '${Uri.encodeComponent(_kMainProxyGroup)}',
      );
      final res = await http
          .put(
            uri,
            headers: {'Content-Type': 'application/json'},
            body: jsonEncode({'name': _selectedNode!.name}),
          )
          .timeout(const Duration(seconds: 5));

      if (res.statusCode != 200 && res.statusCode != 204) {
        throw Exception('HTTP ${res.statusCode}');
      }
      logger.info('proxy', 'Applied node selection',
          fields: {'group': _kMainProxyGroup, 'node': _selectedNode!.name});
      if (triggerProbe) {
        await _runConnectivityChecks();
      }
    } catch (e) {
      logger.warn('proxy', 'Apply node selection failed: $e',
          fields: {'group': _kMainProxyGroup, 'node': _selectedNode!.name});
      if (mounted) {
        setState(() => _connectionStatus =
            'Node switch pending, retry after core is ready');
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
      setState(() => _connectionStatus = 'Node selected, tap connect');
      return;
    }

    try {
      await _applyNodeSelectionIfRunning(triggerProbe: false);
      if (mounted) {
        setState(() => _connectionStatus = 'Switched to ${node.name}');
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

    final directClient = HttpClient()
      ..connectionTimeout = const Duration(seconds: 8);
    final proxyClient = HttpClient()
      ..connectionTimeout = const Duration(seconds: 8)
      ..findProxy = (_) => 'PROXY $_kClashControllerHost:$_kHttpProxyPort;';

    try {
      // Start all probes concurrently
      final directIpFut = _fetchAllIpInfos(directClient, _directIpCandidates);
      final proxyIpFut = _fetchAllIpInfos(proxyClient, _proxyIpCandidates);
      final domesticFut = _checkSites(directClient, _domesticSites);
      final foreignFut = _checkSites(proxyClient, _foreignSites);
      final aiFut = _checkSites(proxyClient, _aiSites);

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
      setState(() => _probeMessage = 'Probe failed: $e');
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

    try {
      final checks = <_BrowserDnsCheckResult>[];
      final info = await VpnManager.getSystemInfo();

      final modeRaw =
          (info['private_dns_mode'] ?? '').toString().trim().toLowerCase();
      final specifier = (info['private_dns_specifier'] ?? '').toString().trim();
      final privateDnsOn = modeRaw == 'hostname' || modeRaw == 'opportunistic';
      checks.add(
        _BrowserDnsCheckResult(
          name: '系统 Private DNS',
          ok: !privateDnsOn,
          detail: privateDnsOn
              ? (modeRaw == 'hostname' && specifier.isNotEmpty
                  ? '开启（严格主机名：$specifier）'
                  : '开启（$modeRaw）')
              : '关闭',
        ),
      );

      final mihomoAnswers = await _queryMihomoDnsA('www.google.com');
      checks.add(
        _BrowserDnsCheckResult(
          name: 'Mihomo DNS 解析',
          ok: mihomoAnswers.isNotEmpty,
          detail: mihomoAnswers.isNotEmpty
              ? 'www.google.com -> ${mihomoAnswers.take(3).join(', ')}'
              : '无返回记录（可能导致浏览器域名无法打开）',
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
        systemDnsDetail = systemDnsOk ? ips.take(3).join(', ') : 'lookup 无可用地址';
      } catch (e) {
        systemDnsDetail = e.toString();
      }
      checks.add(
        _BrowserDnsCheckResult(
          name: '系统 DNS 解析',
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
            ? '经 HTTP 代理访问 gstatic 成功（${watch.elapsedMilliseconds}ms）'
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
          name: '代理链路',
          ok: proxySmokeOk,
          detail: proxySmokeDetail,
        ),
      );

      final summary = _buildBrowserDnsSummary(checks);
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
      setState(() => _browserDnsMessage = '专项检测失败: $e');
    } finally {
      if (mounted) {
        setState(() => _browserDnsLoading = false);
      }
    }
  }

  String _buildBrowserDnsSummary(List<_BrowserDnsCheckResult> checks) {
    bool failed(String name) =>
        checks.any((item) => item.name == name && !item.ok);

    if (failed('系统 Private DNS')) {
      return '检测到 Private DNS 已开启。该状态下浏览器可能出现 DNS_PROBE_FINISHED_BAD_CONFIG，建议先关闭系统 Private DNS 再重试。';
    }
    if (failed('Mihomo DNS 解析')) {
      return 'Mihomo DNS 当前未能解析目标域名，建议检查订阅节点、上游 DNS 和 DNS 防污染设置。';
    }
    if (failed('系统 DNS 解析')) {
      return '系统 DNS 解析异常。若连通性页面“代理侧”正常，优先排查系统 DNS / 路由器 DNS。';
    }
    if (failed('代理链路')) {
      return '代理链路探测失败。DNS 可能正常，但出站链路不可达。';
    }
    return '浏览器 DNS 链路整体正常。若浏览器仍报错，请切换网络后再测一次。';
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
        nodes: _nodes,
        selectedNode: _selectedNode,
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
      _SettingsTab(nodeCount: _nodes.length),
    ];

    return Scaffold(
      body: tabs[_tabIndex],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _tabIndex,
        onDestinationSelected: (i) => setState(() => _tabIndex = i),
        destinations: const [
          NavigationDestination(
            icon: Icon(Icons.shield_outlined),
            selectedIcon: Icon(Icons.shield),
            label: 'Home',
          ),
          NavigationDestination(
            icon: Icon(Icons.language_outlined),
            selectedIcon: Icon(Icons.language),
            label: 'Proxies',
          ),
          NavigationDestination(
            icon: Icon(Icons.cloud_download_outlined),
            selectedIcon: Icon(Icons.cloud_download),
            label: 'Subscriptions',
          ),
          NavigationDestination(
            icon: Icon(Icons.settings_outlined),
            selectedIcon: Icon(Icons.settings),
            label: 'Settings',
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
    final accent = isConnected ? _kConnected : _kBrand;
    final checkedAt = snapshot == null
        ? '--'
        : '${snapshot!.checkedAt.hour.toString().padLeft(2, '0')}:'
            '${snapshot!.checkedAt.minute.toString().padLeft(2, '0')}:'
            '${snapshot!.checkedAt.second.toString().padLeft(2, '0')}';
    final browserDnsCheckedAt = browserDnsSnapshot == null
        ? '--'
        : '${browserDnsSnapshot!.checkedAt.hour.toString().padLeft(2, '0')}:'
            '${browserDnsSnapshot!.checkedAt.minute.toString().padLeft(2, '0')}:'
            '${browserDnsSnapshot!.checkedAt.second.toString().padLeft(2, '0')}';

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
                      isConnected ? 'ACTIVE' : 'IDLE',
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
                            isConnected ? 'VPN 运行中' : 'VPN 未连接',
                            style: TextStyle(
                              color: accent,
                              fontSize: 16,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 4),
                          Text(
                            connectionStatus,
                            style: const TextStyle(
                                color: _kTextMuted, fontSize: 13, height: 1.4),
                          ),
                          const SizedBox(height: 10),
                          Row(
                            children: [
                              FilledButton(
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
                                child: Text(isConnected ? '断开' : '连接'),
                              ),
                              const SizedBox(width: 8),
                              OutlinedButton(
                                onPressed: onTapNode,
                                style: OutlinedButton.styleFrom(
                                  foregroundColor: _kTextMuted,
                                  side: BorderSide(
                                      color: _kBorder.withAlpha(180)),
                                ),
                                child: const Text('更多节点'),
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
                title: '连通性检测',
                subtitle: '切换节点后自动重新执行连通性检测',
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
                  label: const Text('重测', style: TextStyle(fontSize: 12)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          '最近检测: $checkedAt',
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
                        child: const Text(
                          '点击“重测”开始连通性检查',
                          style: TextStyle(color: _kTextMuted, fontSize: 13),
                        ),
                      ),
                    if (snapshot != null)
                      _ConnectivityPane(snapshot: snapshot!),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              _HomeBlockCard(
                title: '浏览器 DNS 专项检测',
                subtitle: '定位“连通性通过但浏览器打不开域名”',
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
                  label: const Text('重测', style: TextStyle(fontSize: 12)),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        Text(
                          '最近检测: $browserDnsCheckedAt',
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
                        child: const Text(
                          '点击“重测”开始浏览器 DNS 专项排查',
                          style: TextStyle(color: _kTextMuted, fontSize: 13),
                        ),
                      ),
                    if (browserDnsSnapshot != null)
                      _BrowserDnsPane(snapshot: browserDnsSnapshot!),
                  ],
                ),
              ),
              const SizedBox(height: 14),
              _HomeBlockCard(
                title: '节点切换',
                subtitle: '当前组：$_kMainProxyGroup',
                trailing: isSwitchingNode
                    ? const SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: _kBrand),
                      )
                    : null,
                child: nodes.isEmpty
                    ? const Text(
                        '暂无节点，请先在 Subscriptions 导入订阅',
                        style: TextStyle(color: _kTextMuted, fontSize: 13),
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
                            child: const Text('查看完整节点列表  →'),
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
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // ── 出口 IP ──────────────────────────────────────────────
        const _SectionLabel(label: '出口 IP'),
        const SizedBox(height: 8),
        _IpGroup(
          categoryTag: '直连',
          tagColor: _kBrand,
          subtitle: '绕过手机 VPN，经路由器直出',
          results: snapshot.directIpResults,
        ),
        const SizedBox(height: 6),
        _IpGroup(
          categoryTag: 'VPN 出口',
          tagColor: const Color(0xFF64B5F6),
          subtitle: '经手机 ClashForge 代理',
          results: snapshot.proxyIpResults,
        ),
        const SizedBox(height: 14),
        // ── 访问检查 ─────────────────────────────────────────────
        const _SectionLabel(label: '访问检查'),
        const SizedBox(height: 8),
        if (snapshot.domesticResults.isNotEmpty) ...[
          const _CategoryTag(label: '直连路径', color: _kBrand),
          const SizedBox(height: 6),
          ...snapshot.domesticResults.map((r) => _SiteCheckRow(result: r)),
          const SizedBox(height: 8),
        ],
        if (snapshot.foreignResults.isNotEmpty) ...[
          const _CategoryTag(label: 'VPN 代理', color: Color(0xFF64B5F6)),
          const SizedBox(height: 6),
          ...snapshot.foreignResults.map((r) => _SiteCheckRow(result: r)),
          const SizedBox(height: 8),
        ],
        if (snapshot.aiResults.isNotEmpty) ...[
          const _CategoryTag(label: 'AI · VPN 代理', color: Color(0xFF9C6DFF)),
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
                  resolved ? '已解析' : '未能获取',
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
                    result.ok ? '正常' : '异常',
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
              const Expanded(
                child: Text(
                  '浏览器 DNS 路径',
                  style: TextStyle(
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
                  snapshot.healthy ? '正常' : '风险',
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
// Tab 2 — Proxies
// ─────────────────────────────────────────────────────────────
class _ProxiesTab extends StatelessWidget {
  const _ProxiesTab(
      {required this.nodes,
      required this.selectedNode,
      required this.onSelect});

  final List<ProxyNode> nodes;
  final ProxyNode? selectedNode;
  final ValueChanged<ProxyNode> onSelect;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Padding(
            padding: const EdgeInsets.fromLTRB(24, 20, 24, 16),
            child: Row(
              children: [
                const Text('Proxies',
                    style: TextStyle(
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
                  child: Text('${nodes.length} nodes',
                      style: const TextStyle(
                          color: _kBrand,
                          fontSize: 12,
                          fontWeight: FontWeight.w600)),
                ),
              ],
            ),
          ),
          Expanded(
            child: nodes.isEmpty
                ? Center(
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
                        const Text('No nodes yet',
                            style: TextStyle(
                                color: _kTextHi,
                                fontSize: 16,
                                fontWeight: FontWeight.w600)),
                        const SizedBox(height: 6),
                        const Text(
                            'Add a subscription in the\nSubscriptions tab',
                            textAlign: TextAlign.center,
                            style: TextStyle(
                                color: _kTextMuted, height: 1.5, fontSize: 13)),
                      ],
                    ),
                  )
                : ListView.separated(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
                    itemCount: nodes.length,
                    separatorBuilder: (_, __) => const SizedBox(height: 6),
                    itemBuilder: (context, i) {
                      final node = nodes[i];
                      final selected = node == selectedNode;
                      return InkWell(
                        borderRadius: BorderRadius.circular(14),
                        onTap: () => onSelect(node),
                        child: AnimatedContainer(
                          duration: const Duration(milliseconds: 200),
                          padding: const EdgeInsets.symmetric(
                              horizontal: 16, vertical: 14),
                          decoration: BoxDecoration(
                            gradient: selected
                                ? LinearGradient(
                                    begin: Alignment.topLeft,
                                    end: Alignment.bottomRight,
                                    colors: [
                                      _kBrand.withAlpha(28),
                                      _kCard,
                                    ],
                                  )
                                : null,
                            color: selected ? null : _kCard,
                            borderRadius: BorderRadius.circular(14),
                            border: Border.all(
                              color:
                                  selected ? _kBrand.withAlpha(120) : _kBorder,
                              width: selected ? 1.5 : 1,
                            ),
                            boxShadow: selected
                                ? [
                                    BoxShadow(
                                      color: _kBrand.withAlpha(30),
                                      blurRadius: 12,
                                      spreadRadius: 0,
                                    )
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
                                      color: _kBrand
                                          .withAlpha(selected ? 80 : 40)),
                                ),
                                child: Icon(Icons.language,
                                    color: selected ? _kBrand : _kTextMuted,
                                    size: 18),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(node.name,
                                        style: TextStyle(
                                          color: selected ? _kTextHi : _kTextHi,
                                          fontWeight: FontWeight.w600,
                                          fontSize: 14,
                                        )),
                                    const SizedBox(height: 2),
                                    Text(
                                        '${node.type.toUpperCase()} · ${node.server}:${node.port}',
                                        style: const TextStyle(
                                            color: _kTextMuted, fontSize: 12)),
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
                                    border: Border.all(
                                        color: _kBrand.withAlpha(100)),
                                  ),
                                  child: const Icon(Icons.check,
                                      color: _kBrand, size: 13),
                                ),
                            ],
                          ),
                        ),
                      );
                    },
                  ),
          ),
        ],
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
          setState(() {
            _loading = false;
            _success = false;
            _message = 'Fetch failed: HTTP $statusCode';
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
      setState(() {
        _loading = false;
        _success = true;
        _message = 'Imported ${parsed.proxies.length} nodes as "$nickname"';
      });
    } catch (e) {
      logger.error('subscription', 'Import error: $e');
      setState(() {
        _loading = false;
        _success = false;
        _message = 'Error: $e';
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
        setState(() {
          _pasteLoading = false;
          _pasteSuccess = false;
          _pasteMessage = '未识别到有效节点，请检查格式（支持 ss:// vmess:// trojan:// vless:// 及 Clash YAML）';
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
      setState(() {
        _pasteLoading = false;
        _pasteSuccess = true;
        _pasteMessage =
            '已导入 ${parsed.proxies.length} 个节点，配置已生成（使用 Loyalsoldier 规则），可直接启动 VPN';
      });
    } catch (e) {
      logger.error('subscription', 'Paste import error: $e');
      setState(() {
        _pasteLoading = false;
        _pasteSuccess = false;
        _pasteMessage = '解析失败: $e';
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
    return showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        title: const Text('Name this subscription'),
        content: TextField(
          controller: controller,
          autofocus: true,
          style: const TextStyle(color: _kTextHi, fontSize: 14),
          decoration: InputDecoration(
            hintText: 'e.g. Work VPN',
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
            child: const Text('Cancel', style: TextStyle(color: _kTextMuted)),
          ),
          FilledButton(
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
            child: const Text('Save'),
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
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: const Text('删除订阅'),
        content: Text('确认删除 "${sub.nickname}"？\n该操作不可撤销。'),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx, false),
            child: const Text('取消', style: TextStyle(color: _kTextMuted)),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: _kError,
              foregroundColor: Colors.white,
              shape: RoundedRectangleBorder(
                  borderRadius: BorderRadius.circular(8)),
            ),
            onPressed: () => Navigator.pop(ctx, true),
            child: const Text('删除'),
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
                          Container(
                            padding: const EdgeInsets.symmetric(
                                horizontal: 7, vertical: 2),
                            decoration: BoxDecoration(
                              color: _kBrand.withAlpha(30),
                              borderRadius: BorderRadius.circular(4),
                              border: Border.all(color: _kBrand.withAlpha(80)),
                            ),
                            child: const Text('使用中',
                                style: TextStyle(
                                    color: _kBrand,
                                    fontSize: 10,
                                    fontWeight: FontWeight.w600)),
                          ),
                        ],
                      ],
                    ),
                    const SizedBox(height: 4),
                    Text(
                      '${sub.nodes.length} 个节点${_domainLabel(sub.url)}',
                      style: const TextStyle(color: _kTextFaint, fontSize: 12),
                    ),
                  ],
                ),
              ),
              if (!isActive) ...[
                const SizedBox(width: 8),
                TextButton(
                  onPressed: () => widget.onActivate(sub),
                  style: TextButton.styleFrom(
                    padding:
                        const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                    minimumSize: Size.zero,
                    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    foregroundColor: _kBrand,
                  ),
                  child: const Text('切换',
                      style:
                          TextStyle(fontSize: 13, fontWeight: FontWeight.w600)),
                ),
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
    return SafeArea(
      child: ListView(
        padding: const EdgeInsets.symmetric(horizontal: 24),
        children: [
          const SizedBox(height: 20),
          const Text('Subscriptions',
              style: TextStyle(
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
                    const Text('SUBSCRIPTION URL',
                        style: TextStyle(
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
                    label: Text(_loading ? 'Fetching…' : 'Import',
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
                    const Text('粘贴节点文本',
                        style: TextStyle(
                            color: _kTextFaint,
                            fontSize: 11,
                            letterSpacing: 1.2,
                            fontWeight: FontWeight.w600)),
                  ],
                ),
                const SizedBox(height: 8),
                const Text(
                  '支持 ss:// vmess:// trojan:// vless:// 链接或 Clash YAML，自动套用 Loyalsoldier 规则生成完整配置',
                  style: TextStyle(color: _kTextMuted, fontSize: 12, height: 1.4),
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
                    label: Text(_pasteLoading ? '解析中…' : '导入并生成配置',
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
            const Text('已保存的订阅',
                style: TextStyle(
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
  const _SettingsTab({required this.nodeCount});
  final int nodeCount;

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          const Padding(
            padding: EdgeInsets.fromLTRB(24, 20, 24, 24),
            child: Text('Settings',
                style: TextStyle(
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
                  title: 'Logs',
                  subtitle: 'Runtime events, VPN & core output',
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
                  title: 'Check for Updates',
                  subtitle: 'See if a newer version is available',
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
                  title: 'About',
                  subtitle: 'App version, runtime status, memory',
                  onTap: () => Navigator.push(
                      context,
                      MaterialPageRoute(
                        builder: (_) => Scaffold(
                          backgroundColor: _kBg,
                          body: _AboutTab(nodeCount: nodeCount),
                        ),
                      )),
                ),
              ],
            ),
          ),
        ],
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
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
            content: Text('Logs copied to clipboard'),
            duration: Duration(seconds: 2)),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    final entries = _visible;

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
                const Text('Logs',
                    style: TextStyle(
                        color: _kTextHi,
                        fontSize: 22,
                        fontWeight: FontWeight.bold,
                        letterSpacing: -0.5)),
                const Spacer(),
                IconButton(
                  tooltip: 'Copy all',
                  icon: const Icon(Icons.copy_outlined,
                      size: 19, color: _kTextMuted),
                  onPressed: _copyAll,
                ),
                IconButton(
                  tooltip: _autoScroll ? 'Auto-scroll on' : 'Auto-scroll off',
                  icon: Icon(Icons.vertical_align_bottom,
                      size: 19, color: _autoScroll ? _kBrand : _kTextFaint),
                  onPressed: () => setState(() => _autoScroll = !_autoScroll),
                ),
                IconButton(
                  tooltip: 'Clear',
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
                              ? 'No logs yet.'
                              : 'No $_filter logs.',
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
              const Text('About',
                  style: TextStyle(
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
            section('APPLICATION', [
              row('Version', '$appVersion ($buildNum)'),
              row('Nodes loaded', '${widget.nodeCount}'),
              row('Device ABI', abi),
            ]),
            const SizedBox(height: 14),
            section('UPDATE', [
              if (_updateChecking)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 12),
                  child: Row(children: [
                    SizedBox(
                        width: 16,
                        height: 16,
                        child: CircularProgressIndicator(
                            strokeWidth: 2, color: _kBrand)),
                    SizedBox(width: 12),
                    Text('Checking for updates…',
                        style: TextStyle(color: _kTextMuted, fontSize: 14)),
                  ]),
                )
              else if (!_updateChecked)
                row('Status', '—')
              else if (_updateInfo == null)
                row('Status', 'Could not check', valueColor: _kError)
              else if (!_updateInfo!.isNewerThan(appVersion))
                row('Status', 'Up to date  ✓', valueColor: _kConnected)
              else ...[
                row('Latest', _updateInfo!.tag, valueColor: _kBrand),
                Padding(
                  padding: const EdgeInsets.only(bottom: 10, top: 4),
                  child: SizedBox(
                    width: double.infinity,
                    child: ElevatedButton.icon(
                      icon: const Icon(Icons.download_rounded, size: 18),
                      label: Text('Download ${_updateInfo!.tag}'),
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
                    child: const Text('Re-check',
                        style: TextStyle(color: _kTextFaint, fontSize: 12)),
                  ),
                ),
            ]),
            const SizedBox(height: 14),
            section('RUNTIME', [
              row('VPN', vpnRunning ? 'Running' : 'Stopped',
                  valueColor: vpnRunning ? _kConnected : _kTextMuted),
              row('Mihomo', mihomoRunning ? 'Running (PID $pid)' : 'Stopped',
                  valueColor: mihomoRunning ? _kConnected : _kTextMuted),
            ]),
            const SizedBox(height: 14),
            section('MEMORY', [
              row('App (PSS)', '${appPss.toStringAsFixed(1)} MB'),
              row('Available', '${avail.toStringAsFixed(0)} MB'),
            ]),
            const SizedBox(height: 20),
            const Center(
                child: Text('Pull down to refresh',
                    style: TextStyle(color: _kTextFaint, fontSize: 12))),
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
            const Text('Check for Updates',
                style: TextStyle(
                    color: _kTextHi,
                    fontSize: 18,
                    fontWeight: FontWeight.bold)),
          ]),
          const SizedBox(height: 24),
          if (_loading) ...[
            const Center(child: CircularProgressIndicator(color: _kBrand)),
            const SizedBox(height: 12),
            const Center(
                child: Text('Checking…',
                    style: TextStyle(color: _kTextMuted, fontSize: 13))),
          ] else if (_info == null) ...[
            const Row(children: [
              Icon(Icons.warning_amber_rounded, color: _kError, size: 22),
              SizedBox(width: 10),
              Expanded(
                child: Text(
                    'Could not check for updates.\nVerify internet connection.',
                    style: TextStyle(color: _kTextMuted, fontSize: 14)),
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
                    const Text('You are up to date',
                        style: TextStyle(
                            color: _kConnected,
                            fontSize: 15,
                            fontWeight: FontWeight.w600)),
                    Text('Version: ${_info!.tag}',
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
                    const Text('Update available',
                        style: TextStyle(
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
                label: Text('Download ${_info!.tag}'),
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
                child: const Text('All releases →',
                    style: TextStyle(color: _kTextMuted, fontSize: 13)),
              ),
            ),
        ],
      ),
    );
  }
}
