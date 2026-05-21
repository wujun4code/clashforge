import 'dart:convert';
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
const _kBg         = Color(0xFF0B0A1A);  // deep indigo-black
const _kSurface    = Color(0xFF13112B);  // nav bar / surface
const _kCard       = Color(0xFF1A1830);  // card bg
const _kCardGrad   = Color(0xFF201D3A);  // card gradient end
const _kBorder     = Color(0xFF2A2750);  // card border
const _kBrand      = Color(0xFF863BFF);  // violet — primary brand
const _kConnected  = Color(0xFF22D3EE);  // neon cyan — VPN on
const _kError      = Color(0xFFEF5350);
const _kTextHi     = Color(0xFFEEEBFF);  // lavender-white
const _kTextMuted  = Color(0xFF9B8CC4);  // muted lavender
const _kTextFaint  = Color(0xFF4A4570);  // very faint

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
      s == 'true' || s == 'false' || s == 'null') {
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
    final scheme = ColorScheme.fromSeed(seedColor: seed, brightness: Brightness.dark).copyWith(
      primary:             seed,
      onPrimary:           Colors.white,
      secondary:           _kConnected,
      surface:             _kSurface,
      onSurface:           _kTextHi,
      surfaceContainerHighest: _kCard,
      secondaryContainer:  seed.withAlpha(40),
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
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
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
          titleTextStyle: TextStyle(color: _kTextHi, fontSize: 17, fontWeight: FontWeight.w600),
        ),
        snackBarTheme: SnackBarThemeData(
          backgroundColor: _kCardGrad,
          contentTextStyle: const TextStyle(color: _kTextHi),
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
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

  int _tabIndex = 0;
  bool _isConnected  = false;
  bool _isConnecting = false;
  String _connectionStatus = 'Tap to connect';
  final List<ProxyNode> _nodes = [];
  ProxyNode? _selectedNode;

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
    final nodes = await SubscriptionStore.loadNodes();
    if (nodes.isNotEmpty) {
      setState(() { _nodes.addAll(nodes); _selectedNode = nodes.first; });
      AppLogger.instance.info('app', 'Loaded saved nodes', fields: {'count': nodes.length});
    }
  }

  void _onNativeLog(dynamic event) {
    try {
      final map = json.decode(event as String) as Map<String, dynamic>;
      final level     = map['level']     as String? ?? 'info';
      final component = map['component'] as String? ?? 'native';
      final message   = map['message']   as String? ?? '';
      final fields    = (map['fields']   as Map<String, dynamic>?) ?? {};
      AppLogger.instance.log(level, component, message, fields: fields);
    } catch (_) {
      AppLogger.instance.debug('native', event.toString());
    }
  }

  void _onNodesImported(List<ProxyNode> nodes, {String url = '', String nickname = ''}) {
    setState(() {
      _nodes..clear()..addAll(nodes);
      _selectedNode ??= nodes.isEmpty ? null : nodes.first;
    });
    SubscriptionStore.save(nodes, url: url, nickname: nickname);
  }

  void _onNodeSelected(ProxyNode node) {
    setState(() { _selectedNode = node; _tabIndex = 0; });
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
        setState(() { _isConnected = false; _connectionStatus = 'Tap to connect'; });
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
        final filesDir  = await VpnManager.getFilesDir();
        final configMap = ConfigGenerator.generate(nodes: _nodes, geodataPath: filesDir);
        final writeResult = await VpnManager.writeConfig(_mapToYaml(configMap));
        logger.debug('vpn', 'Config write result: $writeResult');

        final res = await VpnManager.startVpn();
        logger.info('vpn', 'VPN start result: $res');
        if (res == 'permission_needed') {
          setState(() => _connectionStatus = 'Grant VPN permission, then tap again');
        } else {
          setState(() { _isConnected = true; _connectionStatus = 'Connected'; });
        }
      }
    } catch (e, st) {
      logger.error('vpn', 'Toggle failed: $e',
          fields: {'stack': st.toString().split('\n').first});
      setState(() { _isConnected = false; _connectionStatus = 'Error: $e'; });
    } finally {
      setState(() => _isConnecting = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tabs = <Widget>[
      _HomeTab(
        isConnected:      _isConnected,
        isConnecting:     _isConnecting,
        connectionStatus: _connectionStatus,
        selectedNode:     _selectedNode,
        onToggle:         _toggleVpn,
        onTapNode:        () => setState(() => _tabIndex = 1),
      ),
      _ProxiesTab(nodes: _nodes, selectedNode: _selectedNode, onSelect: _onNodeSelected),
      _SubscriptionsTab(onImported: _onNodesImported),
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
    required this.onToggle,
    required this.onTapNode,
  });

  final bool isConnected;
  final bool isConnecting;
  final String connectionStatus;
  final ProxyNode? selectedNode;
  final VoidCallback onToggle;
  final VoidCallback onTapNode;

  @override
  Widget build(BuildContext context) {
    final accent = isConnected ? _kConnected : _kBrand;

    return Container(
      decoration: BoxDecoration(
        gradient: LinearGradient(
          begin: Alignment.topCenter,
          end: Alignment.bottomCenter,
          colors: [
            const Color(0xFF110F26),
            _kBg,
          ],
        ),
      ),
      child: SafeArea(
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              const SizedBox(height: 20),

              // ── Header ──────────────────────────────────────
              Row(
                children: [
                  Container(
                    width: 36, height: 36,
                    decoration: BoxDecoration(
                      color: _kBrand.withAlpha(28),
                      borderRadius: BorderRadius.circular(10),
                      border: Border.all(color: _kBrand.withAlpha(90), width: 1),
                    ),
                    child: const Icon(Icons.bolt, color: _kBrand, size: 20),
                  ),
                  const SizedBox(width: 10),
                  const Text('ClashForge',
                    style: TextStyle(color: _kTextHi, fontSize: 20,
                        fontWeight: FontWeight.w700, letterSpacing: -0.4)),
                  const Spacer(),
                  // Connected badge in header
                  AnimatedSwitcher(
                    duration: const Duration(milliseconds: 300),
                    child: isConnected
                        ? Container(
                            key: const ValueKey('on'),
                            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                            decoration: BoxDecoration(
                              color: _kConnected.withAlpha(18),
                              borderRadius: BorderRadius.circular(12),
                              border: Border.all(color: _kConnected.withAlpha(70)),
                            ),
                            child: Row(
                              mainAxisSize: MainAxisSize.min,
                              children: [
                                Container(width: 6, height: 6,
                                  decoration: BoxDecoration(
                                    color: _kConnected,
                                    shape: BoxShape.circle,
                                    boxShadow: [BoxShadow(
                                        color: _kConnected.withAlpha(140), blurRadius: 6)],
                                  ),
                                ),
                                const SizedBox(width: 5),
                                const Text('ACTIVE',
                                    style: TextStyle(color: _kConnected, fontSize: 10,
                                        fontWeight: FontWeight.w800, letterSpacing: 1.2)),
                              ],
                            ),
                          )
                        : const SizedBox.shrink(key: ValueKey('off')),
                  ),
                ],
              ),

              const Spacer(),

              // ── VPN Power Button ─────────────────────────────
              Center(
                child: GestureDetector(
                  key: const Key('vpn_toggle'),
                  onTap: onToggle,
                  child: AnimatedContainer(
                    duration: const Duration(milliseconds: 400),
                    curve: Curves.easeInOut,
                    width: 196,
                    height: 196,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: accent.withAlpha(isConnected ? 18 : 12),
                      border: Border.all(color: accent, width: isConnected ? 2.0 : 1.5),
                      boxShadow: [
                        BoxShadow(
                          color: accent.withAlpha(isConnected ? 100 : 55),
                          blurRadius: isConnected ? 48 : 24,
                          spreadRadius: isConnected ? 6 : 1,
                        ),
                        BoxShadow(
                          color: accent.withAlpha(isConnected ? 50 : 22),
                          blurRadius: isConnected ? 90 : 50,
                          spreadRadius: isConnected ? 14 : 4,
                        ),
                      ],
                    ),
                    child: isConnecting
                        ? Center(
                            child: SizedBox(
                              width: 40, height: 40,
                              child: CircularProgressIndicator(
                                color: accent, strokeWidth: 2,
                                backgroundColor: accent.withAlpha(25),
                              ),
                            ),
                          )
                        : Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              Icon(
                                Icons.power_settings_new,
                                size: 62,
                                color: accent,
                              ),
                              const SizedBox(height: 6),
                              Text(
                                isConnected ? 'ON' : 'OFF',
                                style: TextStyle(
                                  color: accent,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w800,
                                  letterSpacing: 4,
                                ),
                              ),
                            ],
                          ),
                  ),
                ),
              ),

              const SizedBox(height: 20),

              // ── Status pill ──────────────────────────────────
              Center(
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 300),
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 6),
                  decoration: BoxDecoration(
                    color: accent.withAlpha(12),
                    borderRadius: BorderRadius.circular(20),
                    border: Border.all(color: accent.withAlpha(55), width: 1),
                  ),
                  child: Text(
                    connectionStatus,
                    style: TextStyle(color: accent, fontSize: 13, fontWeight: FontWeight.w500),
                  ),
                ),
              ),

              const Spacer(),

              // ── Node selector card ───────────────────────────
              GestureDetector(
                onTap: onTapNode,
                child: Container(
                  width: double.infinity,
                  padding: const EdgeInsets.all(16),
                  decoration: BoxDecoration(
                    gradient: LinearGradient(
                      begin: Alignment.topLeft,
                      end: Alignment.bottomRight,
                      colors: [_kCardGrad, _kCard],
                    ),
                    borderRadius: BorderRadius.circular(18),
                    border: Border.all(color: _kBorder.withAlpha(180), width: 1),
                  ),
                  child: Row(
                    children: [
                      Container(
                        width: 42, height: 42,
                        decoration: BoxDecoration(
                          color: _kBrand.withAlpha(22),
                          borderRadius: BorderRadius.circular(11),
                          border: Border.all(color: _kBrand.withAlpha(60), width: 1),
                        ),
                        child: const Icon(Icons.language, color: _kBrand, size: 20),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: selectedNode == null
                            ? Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  const Text('No node selected',
                                      style: TextStyle(
                                          color: _kTextMuted, fontWeight: FontWeight.w600)),
                                  const SizedBox(height: 2),
                                  Text('Go to Proxies to choose one',
                                      style: TextStyle(color: _kTextFaint, fontSize: 12)),
                                ],
                              )
                            : Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(selectedNode!.name,
                                      style: const TextStyle(
                                          color: _kTextHi, fontWeight: FontWeight.w600,
                                          fontSize: 15)),
                                  const SizedBox(height: 3),
                                  Text(
                                    '${selectedNode!.type.toUpperCase()}  ·  '
                                    '${selectedNode!.server}:${selectedNode!.port}',
                                    style: const TextStyle(
                                        color: _kTextMuted, fontSize: 12),
                                  ),
                                ],
                              ),
                      ),
                      Icon(Icons.chevron_right, color: _kTextFaint, size: 20),
                    ],
                  ),
                ),
              ),
              const SizedBox(height: 28),
            ],
          ),
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 2 — Proxies
// ─────────────────────────────────────────────────────────────
class _ProxiesTab extends StatelessWidget {
  const _ProxiesTab({required this.nodes, required this.selectedNode, required this.onSelect});

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
                    style: TextStyle(color: _kTextHi, fontSize: 24,
                        fontWeight: FontWeight.bold, letterSpacing: -0.5)),
                const Spacer(),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
                  decoration: BoxDecoration(
                    color: _kBrand.withAlpha(18),
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(color: _kBrand.withAlpha(55)),
                  ),
                  child: Text('${nodes.length} nodes',
                      style: const TextStyle(color: _kBrand, fontSize: 12,
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
                          width: 72, height: 72,
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
                            style: TextStyle(color: _kTextHi, fontSize: 16,
                                fontWeight: FontWeight.w600)),
                        const SizedBox(height: 6),
                        const Text('Add a subscription in the\nSubscriptions tab',
                            textAlign: TextAlign.center,
                            style: TextStyle(color: _kTextMuted, height: 1.5, fontSize: 13)),
                      ],
                    ),
                  )
                : ListView.separated(
                    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
                    itemCount: nodes.length,
                    separatorBuilder: (_, __) => const SizedBox(height: 6),
                    itemBuilder: (context, i) {
                      final node     = nodes[i];
                      final selected = node == selectedNode;
                      return InkWell(
                        borderRadius: BorderRadius.circular(14),
                        onTap: () => onSelect(node),
                        child: AnimatedContainer(
                          duration: const Duration(milliseconds: 200),
                          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
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
                              color: selected ? _kBrand.withAlpha(120) : _kBorder,
                              width: selected ? 1.5 : 1,
                            ),
                            boxShadow: selected
                                ? [BoxShadow(
                                    color: _kBrand.withAlpha(30),
                                    blurRadius: 12,
                                    spreadRadius: 0,
                                  )]
                                : null,
                          ),
                          child: Row(
                            children: [
                              Container(
                                width: 36, height: 36,
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
                                        style: TextStyle(
                                          color: selected ? _kTextHi : _kTextHi,
                                          fontWeight: FontWeight.w600,
                                          fontSize: 14,
                                        )),
                                    const SizedBox(height: 2),
                                    Text('${node.type.toUpperCase()} · ${node.server}:${node.port}',
                                        style: const TextStyle(
                                            color: _kTextMuted, fontSize: 12)),
                                  ],
                                ),
                              ),
                              if (selected)
                                Container(
                                  width: 22, height: 22,
                                  decoration: BoxDecoration(
                                    color: _kBrand.withAlpha(30),
                                    shape: BoxShape.circle,
                                    border: Border.all(color: _kBrand.withAlpha(100)),
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
typedef _OnImported = void Function(List<ProxyNode> nodes, {String url, String nickname});

class _SubscriptionsTab extends StatefulWidget {
  const _SubscriptionsTab({required this.onImported});
  final _OnImported onImported;

  @override
  State<_SubscriptionsTab> createState() => _SubscriptionsTabState();
}

class _SubscriptionsTabState extends State<_SubscriptionsTab> {
  final _urlController = TextEditingController();
  bool    _loading = false;
  String? _message;
  bool    _success = false;

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

  Future<void> _import() async {
    final input = _urlController.text.trim();
    if (input.isEmpty) return;
    final logger = AppLogger.instance;
    setState(() { _loading = true; _message = null; });
    try {
      String content = input;

      if (input.startsWith('http://') || input.startsWith('https://')) {
        logger.info('subscription', 'Fetching URL', fields: {'url': input});
        final response = await http.get(Uri.parse(input));
        logger.info('subscription', 'Fetch response', fields: {
          'status': response.statusCode,
          'bytes': response.contentLength ?? response.bodyBytes.length,
        });
        if (response.statusCode != 200) {
          setState(() { _loading = false; _success = false;
            _message = 'Fetch failed: HTTP ${response.statusCode}'; });
          return;
        }
        content = response.body;
      }

      final nodes = SubscriptionParser.parse(content);
      logger.info('subscription', 'Parsed nodes', fields: {'count': nodes.length});
      if (nodes.isEmpty) {
        logger.warn('subscription', 'No nodes parsed');
      }

      final defaultName = await SubscriptionStore.generateDefaultNickname();
      if (!mounted) return;
      final nickname = await _showNicknameDialog(defaultName);
      if (!mounted) return;
      if (nickname == null) {
        setState(() { _loading = false; _success = false; _message = null; });
        return;
      }

      widget.onImported(nodes,
          url: input.startsWith('http') ? input : '',
          nickname: nickname);
      setState(() {
        _loading = false; _success = true;
        _message = 'Imported ${nodes.length} nodes as "$nickname"';
      });
    } catch (e) {
      logger.error('subscription', 'Import error: $e');
      setState(() { _loading = false; _success = false; _message = 'Error: $e'; });
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
            child: const Text('Cancel',
                style: TextStyle(color: _kTextMuted)),
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

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SizedBox(height: 20),
            const Text('Subscriptions',
                style: TextStyle(color: _kTextHi, fontSize: 24,
                    fontWeight: FontWeight.bold, letterSpacing: -0.5)),
            const SizedBox(height: 24),

            // URL card
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
                        width: 32, height: 32,
                        decoration: BoxDecoration(
                          color: _kBrand.withAlpha(22),
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(color: _kBrand.withAlpha(60)),
                        ),
                        child: const Icon(Icons.link, color: _kBrand, size: 17),
                      ),
                      const SizedBox(width: 10),
                      const Text('SUBSCRIPTION URL',
                          style: TextStyle(color: _kTextFaint, fontSize: 11,
                              letterSpacing: 1.2, fontWeight: FontWeight.w600)),
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
                          borderSide: const BorderSide(color: _kBrand, width: 1.5)),
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
                              width: 16, height: 16,
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

            // Result banner
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
                    Icon(_success ? Icons.check_circle_outline : Icons.error_outline,
                        color: _success ? _kConnected : _kError, size: 18),
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
          ],
        ),
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
                style: TextStyle(color: _kTextHi, fontSize: 24,
                    fontWeight: FontWeight.bold, letterSpacing: -0.5)),
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
                  onTap: () => Navigator.push(context, MaterialPageRoute(
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
                      borderRadius: BorderRadius.vertical(top: Radius.circular(20)),
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
                  onTap: () => Navigator.push(context, MaterialPageRoute(
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

  final Color    iconColor;
  final IconData icon;
  final String   title;
  final String   subtitle;
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
              width: 42, height: 42,
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
                      style: const TextStyle(color: _kTextHi,
                          fontWeight: FontWeight.w600, fontSize: 15)),
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
  String _filter    = 'all';
  bool   _autoScroll = true;

  static const _levels = ['all', 'debug', 'info', 'warn', 'error'];
  static const _levelColors = {
    'debug': Color(0xFF9B8CC4),
    'info' : _kBrand,
    'warn' : Color(0xFFFFB74D),
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
              duration: const Duration(milliseconds: 150), curve: Curves.easeOut);
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
        const SnackBar(content: Text('Logs copied to clipboard'),
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
                    style: TextStyle(color: _kTextHi, fontSize: 22,
                        fontWeight: FontWeight.bold, letterSpacing: -0.5)),
                const Spacer(),
                IconButton(
                  tooltip: 'Copy all',
                  icon: const Icon(Icons.copy_outlined, size: 19, color: _kTextMuted),
                  onPressed: _copyAll,
                ),
                IconButton(
                  tooltip: _autoScroll ? 'Auto-scroll on' : 'Auto-scroll off',
                  icon: Icon(Icons.vertical_align_bottom, size: 19,
                      color: _autoScroll ? _kBrand : _kTextFaint),
                  onPressed: () => setState(() => _autoScroll = !_autoScroll),
                ),
                IconButton(
                  tooltip: 'Clear',
                  icon: const Icon(Icons.delete_outline, size: 19, color: _kTextMuted),
                  onPressed: () { AppLogger.instance.clear(); setState(() {}); },
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
                final color  = lvl == 'all'
                    ? _kBrand
                    : (_levelColors[lvl] ?? _kTextMuted);
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: GestureDetector(
                    onTap: () => setState(() => _filter = lvl),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 150),
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                      decoration: BoxDecoration(
                        color: active ? color.withAlpha(28) : Colors.transparent,
                        borderRadius: BorderRadius.circular(20),
                        border: Border.all(
                            color: active ? color : _kBorder),
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
                        Icon(Icons.terminal, size: 40, color: _kTextFaint),
                        const SizedBox(height: 12),
                        Text(
                          _filter == 'all' ? 'No logs yet.' : 'No $_filter logs.',
                          style: const TextStyle(color: _kTextMuted),
                        ),
                      ],
                    ),
                  )
                : ListView.builder(
                    controller: _scroll,
                    padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
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
    'info' : _kBrand,
    'warn' : Color(0xFFFFB74D),
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
              style: const TextStyle(color: _kTextFaint, fontSize: 10,
                  fontFamily: 'monospace')),
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
              style: TextStyle(color: color, fontSize: 9,
                  fontWeight: FontWeight.w800, letterSpacing: 0.5),
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
                style: const TextStyle(color: _kTextMuted, fontSize: 10,
                    fontFamily: 'monospace')),
          ),
          const SizedBox(width: 6),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(entry.message,
                    style: const TextStyle(color: _kTextHi, fontSize: 12,
                        fontFamily: 'monospace')),
                if (entry.fields.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Wrap(
                    spacing: 4,
                    runSpacing: 3,
                    children: entry.fields.entries.map((kv) => Container(
                      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                      decoration: BoxDecoration(
                        color: const Color(0xFF1E1C3A),
                        borderRadius: BorderRadius.circular(3),
                      ),
                      child: Text('${kv.key}=${kv.value}',
                          style: const TextStyle(color: _kTextMuted, fontSize: 10,
                              fontFamily: 'monospace')),
                    )).toList(),
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
    if (mounted) setState(() { _info = info; _loading = false; });
  }

  Future<void> _checkUpdate() async {
    if (_updateChecking) return;
    setState(() { _updateChecking = true; _updateChecked = false; });
    final info = await fetchLatestRelease();
    if (mounted) setState(() { _updateInfo = info; _updateChecking = false; _updateChecked = true; });
  }

  @override
  Widget build(BuildContext context) {
    Widget row(String label, String value, {Color? valueColor}) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(color: _kTextMuted, fontSize: 14)),
          Text(value, style: TextStyle(
              color: valueColor ?? _kTextHi, fontSize: 14,
              fontWeight: FontWeight.w600)),
        ],
      ),
    );

    final mihomoRunning = _info['mihomo_running'] as bool?   ?? false;
    final vpnRunning    = _info['vpn_running']    as bool?   ?? false;
    final pid           = _info['mihomo_pid']     as int?    ?? -1;
    final appVersion    = _info['app_version']    as String? ?? '—';
    final buildNum      = _info['build_number'];
    final appPss        = _info['memory_app_pss_mb']   as double? ?? 0.0;
    final avail         = _info['memory_available_mb'] as double? ?? 0.0;
    final abi           = _info['device_abi']          as String? ?? '—';

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
                style: const TextStyle(color: _kTextFaint, fontSize: 11,
                    letterSpacing: 1.2, fontWeight: FontWeight.w600)),
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
                  style: TextStyle(color: _kTextHi, fontSize: 24,
                      fontWeight: FontWeight.bold, letterSpacing: -0.5)),
              const Spacer(),
              if (_loading)
                const SizedBox(width: 18, height: 18,
                    child: CircularProgressIndicator(
                        strokeWidth: 2, color: _kBrand))
              else
                IconButton(
                    icon: const Icon(Icons.refresh, color: _kTextMuted, size: 20),
                    onPressed: _refresh,
                    padding: EdgeInsets.zero),
            ]),
            const SizedBox(height: 20),

            section('APPLICATION', [
              row('Version',      '$appVersion ($buildNum)'),
              row('Nodes loaded', '${widget.nodeCount}'),
              row('Device ABI',   abi),
            ]),
            const SizedBox(height: 14),

            section('UPDATE', [
              if (_updateChecking)
                const Padding(
                  padding: EdgeInsets.symmetric(vertical: 12),
                  child: Row(children: [
                    SizedBox(width: 16, height: 16,
                        child: CircularProgressIndicator(strokeWidth: 2, color: _kBrand)),
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
              row('VPN',    vpnRunning    ? 'Running'  : 'Stopped',
                  valueColor: vpnRunning    ? _kConnected : _kTextMuted),
              row('Mihomo', mihomoRunning ? 'Running (PID $pid)' : 'Stopped',
                  valueColor: mihomoRunning ? _kConnected : _kTextMuted),
            ]),
            const SizedBox(height: 14),

            section('MEMORY', [
              row('App (PSS)',  '${appPss.toStringAsFixed(1)} MB'),
              row('Available', '${avail.toStringAsFixed(0)} MB'),
            ]),
            const SizedBox(height: 20),
            const Center(child: Text('Pull down to refresh',
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
                  color: _kBorder,
                  borderRadius: BorderRadius.circular(2)),
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
                border: Border.all(
                    color: const Color(0xFF34D399).withAlpha(60)),
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
            const Center(
                child: CircularProgressIndicator(color: _kBrand)),
            const SizedBox(height: 12),
            const Center(
                child: Text('Checking…',
                    style: TextStyle(color: _kTextMuted, fontSize: 13))),
          ] else if (_info == null) ...[
            Row(children: const [
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
                        style: const TextStyle(
                            color: _kTextMuted, fontSize: 13)),
                  ],
                ),
              ),
            ]),
          ] else ...[
            Row(children: [
              const Icon(Icons.new_releases_outlined,
                  color: _kBrand, size: 22),
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
