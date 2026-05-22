import 'dart:async';
import 'dart:convert';
import 'dart:io';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'package:url_launcher/url_launcher.dart';
import 'subscription/subscription_parser.dart';
import 'subscription/subscription_store.dart';
import 'subscription/proxy_node.dart';
import 'config/vpn_manager.dart';
import 'config/config_generator.dart';
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
const _kMixedProxyPort = 7892;
const _kMainProxyGroup = '🚀 Proxy';

class _ProbeTarget {
  const _ProbeTarget({
    required this.name,
    required this.url,
    required this.description,
  });

  final String name;
  final String url;
  final String description;
}

class _ProbeCheckResult {
  const _ProbeCheckResult({
    required this.name,
    required this.url,
    required this.description,
    required this.ok,
    this.latencyMs,
    this.error,
  });

  final String name;
  final String url;
  final String description;
  final bool ok;
  final int? latencyMs;
  final String? error;
}

class _ProbeScopeResult {
  const _ProbeScopeResult({
    required this.title,
    required this.via,
    required this.results,
    this.exitIp,
    this.error,
  });

  final String title;
  final String via;
  final List<_ProbeCheckResult> results;
  final String? exitIp;
  final String? error;

  bool get healthy => results.isNotEmpty && results.every((item) => item.ok);
}

class _ConnectivitySnapshot {
  const _ConnectivitySnapshot({
    required this.checkedAt,
    required this.proxySide,
    required this.directSide,
  });

  final DateTime checkedAt;
  final _ProbeScopeResult proxySide;
  final _ProbeScopeResult directSide;
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
  static const _probeTargets = <_ProbeTarget>[
    _ProbeTarget(
      name: 'Google 204',
      url: 'https://www.gstatic.com/generate_204',
      description: '基础连通性',
    ),
    _ProbeTarget(
      name: 'Cloudflare 204',
      url: 'https://cp.cloudflare.com/generate_204',
      description: '国际出口探测',
    ),
    _ProbeTarget(
      name: 'GitHub',
      url: 'https://github.com',
      description: '常用站点可达性',
    ),
  ];
  static const _ipProbeUrls = <String>[
    'https://api.ipify.org',
    'https://ifconfig.me/ip',
  ];

  int _tabIndex = 0;
  bool _isConnected = false;
  bool _isConnecting = false;
  bool _probeLoading = false;
  bool _switchingNode = false;
  String _connectionStatus = 'Tap to connect';
  String? _probeMessage;
  _ConnectivitySnapshot? _connectivitySnapshot;
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
    final subs     = await SubscriptionStore.loadSubscriptions();
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

  void _onNodesImported(List<ProxyNode> nodes,
      {String url = '', String nickname = ''}) {
    final id  = '${DateTime.now().millisecondsSinceEpoch}';
    final sub = Subscription(id: id, nickname: nickname, url: url, nodes: nodes);

    final previousSelected = _selectedNode?.name;
    ProxyNode? nextSelected;
    if (previousSelected != null) {
      for (final item in nodes) {
        if (item.name == previousSelected) {
          nextSelected = item;
          break;
        }
      }
    }
    nextSelected ??= nodes.isEmpty ? null : nodes.first;

    setState(() {
      _subscriptions.add(sub);
      _activeSubscriptionId = id;
      _nodes
        ..clear()
        ..addAll(nodes);
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
          _probeMessage = null;
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
        final configMap = ConfigGenerator.generate(
          nodes: _nodes,
          geodataPath: filesDir,
          selectedNodeName: _selectedNode!.name,
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
    } finally {
      if (mounted) {
        setState(() => _switchingNode = false);
      }
    }
  }

  Future<void> _runConnectivityChecks() async {
    if (_probeLoading) return;
    final logger = AppLogger.instance;

    setState(() {
      _probeLoading = true;
      _probeMessage = null;
    });

    try {
      final results = await Future.wait([
        _runProbeScope(viaProxy: true),
        _runProbeScope(viaProxy: false),
      ]);

      if (!mounted) return;
      setState(() {
        _connectivitySnapshot = _ConnectivitySnapshot(
          checkedAt: DateTime.now(),
          proxySide: results[0],
          directSide: results[1],
        );
      });
    } catch (e) {
      logger.error('probe', 'Connectivity probe failed: $e');
      if (!mounted) return;
      setState(() => _probeMessage = 'Probe failed: $e');
    } finally {
      if (mounted) {
        setState(() => _probeLoading = false);
      }
    }
  }

  Future<_ProbeScopeResult> _runProbeScope({required bool viaProxy}) async {
    final client = HttpClient()..connectionTimeout = const Duration(seconds: 6);

    if (viaProxy) {
      client.findProxy =
          (_) => 'PROXY $_kClashControllerHost:$_kMixedProxyPort;';
    }

    try {
      final checks = <_ProbeCheckResult>[];
      for (final target in _probeTargets) {
        checks.add(await _runSingleProbe(client, target));
      }
      final ip = viaProxy ? await _probeExitIp(client) : null;
      return _ProbeScopeResult(
        title: viaProxy ? '代理侧' : '本机侧',
        via: viaProxy ? '经 Mihomo mixed 端口' : '手机系统直连网络',
        results: checks,
        exitIp: ip,
      );
    } catch (e) {
      return _ProbeScopeResult(
        title: viaProxy ? '代理侧' : '本机侧',
        via: viaProxy ? '经 Mihomo mixed 端口' : '手机系统直连网络',
        results: const [],
        error: e.toString(),
      );
    } finally {
      client.close(force: true);
    }
  }

  Future<_ProbeCheckResult> _runSingleProbe(
      HttpClient client, _ProbeTarget target) async {
    final watch = Stopwatch()..start();
    try {
      final uri = Uri.parse(target.url);
      final req = await client.getUrl(uri).timeout(const Duration(seconds: 6));
      req.followRedirects = true;
      req.headers.set(HttpHeaders.userAgentHeader, 'ClashForgeMobile/1.0');
      final res = await req.close().timeout(const Duration(seconds: 8));
      await res.drain<void>();
      final ok = res.statusCode >= 200 && res.statusCode < 400;
      return _ProbeCheckResult(
        name: target.name,
        url: target.url,
        description: target.description,
        ok: ok,
        latencyMs: watch.elapsedMilliseconds,
        error: ok ? null : 'HTTP ${res.statusCode}',
      );
    } on TimeoutException {
      return _ProbeCheckResult(
        name: target.name,
        url: target.url,
        description: target.description,
        ok: false,
        error: 'timeout',
      );
    } on SocketException catch (e) {
      return _ProbeCheckResult(
        name: target.name,
        url: target.url,
        description: target.description,
        ok: false,
        error: e.message,
      );
    } catch (e) {
      return _ProbeCheckResult(
        name: target.name,
        url: target.url,
        description: target.description,
        ok: false,
        error: e.toString(),
      );
    } finally {
      watch.stop();
    }
  }

  Future<String?> _probeExitIp(HttpClient client) async {
    for (final url in _ipProbeUrls) {
      try {
        final req = await client
            .getUrl(Uri.parse(url))
            .timeout(const Duration(seconds: 6));
        req.headers.set(HttpHeaders.userAgentHeader, 'ClashForgeMobile/1.0');
        final res = await req.close().timeout(const Duration(seconds: 6));
        final body =
            await utf8.decodeStream(res).timeout(const Duration(seconds: 6));
        if (res.statusCode >= 200 && res.statusCode < 300) {
          final ip = body.trim();
          if (ip.isNotEmpty) return ip;
        }
      } catch (_) {
        continue;
      }
    }
    return null;
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
        snapshot: _connectivitySnapshot,
        onToggle: _toggleVpn,
        onRecheckProbe: _runConnectivityChecks,
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
    required this.snapshot,
    required this.onToggle,
    required this.onRecheckProbe,
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
  final _ConnectivitySnapshot? snapshot;
  final VoidCallback onToggle;
  final Future<void> Function() onRecheckProbe;
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
              const SizedBox(height: 16),
              _HomeBlockCard(
                title: '连通性检测',
                subtitle: '切换节点后建议重新检测',
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
                    if (snapshot != null) ...[
                      _ProbeScopePane(scope: snapshot!.proxySide),
                      const SizedBox(height: 8),
                      _ProbeScopePane(scope: snapshot!.directSide),
                    ],
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

class _ProbeScopePane extends StatelessWidget {
  const _ProbeScopePane({required this.scope});
  final _ProbeScopeResult scope;

  @override
  Widget build(BuildContext context) {
    final tone = scope.healthy ? _kConnected : _kError;

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
                  scope.title,
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
                  scope.healthy ? '正常' : '异常',
                  style: TextStyle(
                      color: tone, fontSize: 11, fontWeight: FontWeight.w600),
                ),
              ),
            ],
          ),
          const SizedBox(height: 3),
          Text(scope.via,
              style: const TextStyle(color: _kTextFaint, fontSize: 11)),
          if (scope.exitIp != null) ...[
            const SizedBox(height: 6),
            Text(
              '出口 IP: ${scope.exitIp}',
              style: const TextStyle(color: _kTextMuted, fontSize: 12),
            ),
          ],
          if (scope.error != null) ...[
            const SizedBox(height: 8),
            Text(
              scope.error!,
              style: const TextStyle(color: _kError, fontSize: 12),
            ),
          ],
          if (scope.results.isNotEmpty) ...[
            const SizedBox(height: 8),
            ...scope.results.map((result) {
              final okColor = result.ok ? _kConnected : _kError;
              return Padding(
                padding: const EdgeInsets.only(bottom: 6),
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
                            result.name,
                            style: const TextStyle(
                                color: _kTextHi,
                                fontSize: 12,
                                fontWeight: FontWeight.w600),
                          ),
                          Text(
                            result.description,
                            style: const TextStyle(
                                color: _kTextFaint, fontSize: 11),
                          ),
                        ],
                      ),
                    ),
                    Text(
                      result.ok
                          ? '${result.latencyMs ?? 0}ms'
                          : (result.error ?? 'failed'),
                      style: TextStyle(
                          color: okColor,
                          fontSize: 11,
                          fontWeight: FontWeight.w600),
                    ),
                  ],
                ),
              );
            }),
          ],
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
typedef _OnImported    = void Function(List<ProxyNode> nodes, {String url, String nickname});
typedef _OnSubActivated = void Function(Subscription sub);
typedef _OnSubDeleted   = void Function(Subscription sub);

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
  bool _loading = false;
  String? _message;
  bool _success = false;

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
    super.dispose();
  }

  static const _httpChannel = MethodChannel('com.clashforge.mobile/http');

  // Uses Android's HttpURLConnection — system TLS stack has a browser-compatible
  // JA3 fingerprint, bypassing servers that block Dart's dart:io TLS client.
  Future<(int, String)> _fetchUrlNative(String url) async {
    final result = await _httpChannel.invokeMapMethod<String, dynamic>(
        'fetchUrl', {'url': url, 'timeoutMs': 15000});
    return (result!['status'] as int, result['body'] as String);
  }

  Future<(int, String)> _fetchUrlDart(String url) async {
    final response = await http.get(Uri.parse(url));
    return (response.statusCode, response.body);
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
        // Use Android's native HttpURLConnection (system TLS stack) to bypass
        // TLS fingerprinting on subscription servers that block Dart's dart:io.
        final (statusCode, body) = Platform.isAndroid
            ? await _fetchUrlNative(input)
            : await _fetchUrlDart(input);
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

      final nodes = SubscriptionParser.parse(content);
      logger.info('subscription', 'Parsed nodes',
          fields: {'count': nodes.length});
      if (nodes.isEmpty) {
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

      widget.onImported(nodes,
          url: input.startsWith('http') ? input : '', nickname: nickname);
      setState(() {
        _loading = false;
        _success = true;
        _message = 'Imported ${nodes.length} nodes as "$nickname"';
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
                              border:
                                  Border.all(color: _kBrand.withAlpha(80)),
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
                      style: const TextStyle(
                          color: _kTextFaint, fontSize: 12),
                    ),
                  ],
                ),
              ),
              if (!isActive) ...[
                const SizedBox(width: 8),
                TextButton(
                  onPressed: () => widget.onActivate(sub),
                  style: TextButton.styleFrom(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 12, vertical: 6),
                    minimumSize: Size.zero,
                    tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                    foregroundColor: _kBrand,
                  ),
                  child: const Text('切换',
                      style: TextStyle(
                          fontSize: 13, fontWeight: FontWeight.w600)),
                ),
              ],
              IconButton(
                icon: const Icon(Icons.delete_outline,
                    color: _kTextFaint, size: 18),
                onPressed: () => _confirmDelete(sub),
                padding: EdgeInsets.zero,
                constraints:
                    const BoxConstraints(minWidth: 32, minHeight: 32),
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
                      child:
                          const Icon(Icons.link, color: _kBrand, size: 17),
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
                        borderSide: const BorderSide(
                            color: _kBrand, width: 1.5)),
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
                                strokeWidth: 2,
                                color: Colors.white54))
                        : const Icon(Icons.cloud_download, size: 18),
                    label: Text(_loading ? 'Fetching…' : 'Import',
                        style: const TextStyle(
                            fontWeight: FontWeight.w600)),
                  ),
                ),
              ],
            ),
          ),

          // Result banner
          if (_message != null) ...[
            const SizedBox(height: 14),
            AnimatedContainer(
              duration: const Duration(milliseconds: 250),
              padding: const EdgeInsets.symmetric(
                  horizontal: 16, vertical: 12),
              decoration: BoxDecoration(
                color: (_success ? _kConnected : _kError).withAlpha(15),
                borderRadius: BorderRadius.circular(12),
                border: Border.all(
                    color:
                        (_success ? _kConnected : _kError).withAlpha(70)),
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
