import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:http/http.dart' as http;
import 'subscription/subscription_parser.dart';
import 'subscription/subscription_store.dart';
import 'subscription/proxy_node.dart';
import 'config/vpn_manager.dart';
import 'config/config_generator.dart';
import 'logger/app_logger.dart';
import 'logger/log_entry.dart';

void main() {
  runApp(const ClashForgeApp());
}

// ─── Minimal YAML serialiser (no external dependency) ────────────────────────

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
  // Quote strings that YAML could misparse
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

class ClashForgeApp extends StatelessWidget {
  const ClashForgeApp({super.key});

  @override
  Widget build(BuildContext context) {
    return MaterialApp(
      title: 'ClashForge',
      debugShowCheckedModeBanner: false,
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: const Color(0xFF00B4D8),
          brightness: Brightness.dark,
        ),
        useMaterial3: true,
        scaffoldBackgroundColor: const Color(0xFF0A0A0F),
        cardTheme: CardThemeData(
          color: const Color(0xFF16161E),
          elevation: 0,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        ),
      ),
      home: const HomeScreen(),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Root screen — owns all shared state
// ─────────────────────────────────────────────────────────────

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  // Native log channel (Kotlin → Flutter)
  static const _logChannel = EventChannel('com.clashforge.mobile/logs');

  int _tabIndex = 0;
  bool _isConnected = false;
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
      setState(() {
        _nodes.addAll(nodes);
        _selectedNode = nodes.first;
      });
      AppLogger.instance.info('app', 'Loaded saved nodes', fields: {'count': nodes.length});
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

  void _onNodesImported(List<ProxyNode> nodes, {String url = '', String nickname = ''}) {
    setState(() {
      _nodes
        ..clear()
        ..addAll(nodes);
      _selectedNode ??= nodes.isEmpty ? null : nodes.first;
    });
    SubscriptionStore.save(nodes, url: url, nickname: nickname);
  }

  void _onNodeSelected(ProxyNode node) {
    setState(() {
      _selectedNode = node;
      _tabIndex = 0;
    });
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
        });
      } else {
        if (_selectedNode == null) {
          logger.warn('vpn', 'No node selected — cannot start VPN');
          setState(() => _connectionStatus = 'Select a node first');
          return;
        }
        logger.info('vpn', 'Starting VPN', fields: {
          'node': _selectedNode!.name,
          'type': _selectedNode!.type,
          'server': _selectedNode!.server,
          'port': _selectedNode!.port,
        });

        // Write mihomo config.yaml to native filesDir before starting VPN
        final filesDir = await VpnManager.getFilesDir();
        logger.debug('vpn', 'Writing config.yaml', fields: {'filesDir': filesDir});
        final configMap = ConfigGenerator.generate(
          nodes: _nodes,
          geodataPath: filesDir,
        );
        final configYaml = _mapToYaml(configMap);
        final writeResult = await VpnManager.writeConfig(configYaml);
        logger.debug('vpn', 'Config write result: $writeResult');

        final res = await VpnManager.startVpn();
        logger.info('vpn', 'VPN start result: $res');
        if (res == 'permission_needed') {
          // VPN permission dialog shown; service starts asynchronously via onActivityResult.
          // Keep _isConnected = false so the user can tap again once permission is granted.
          setState(() => _connectionStatus = 'VPN permission requested — tap again after granting');
        } else {
          setState(() {
            _isConnected = true;
            _connectionStatus = 'Connected';
          });
        }
      }
    } catch (e, st) {
      logger.error('vpn', 'VPN toggle failed: $e', fields: {'stack': st.toString().split('\n').first});
      setState(() {
        _isConnected = false;
        _connectionStatus = 'Error: $e';
      });
    } finally {
      setState(() => _isConnecting = false);
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
        onToggle: _toggleVpn,
        onTapNode: () => setState(() => _tabIndex = 1),
      ),
      _ProxiesTab(
        nodes: _nodes,
        selectedNode: _selectedNode,
        onSelect: _onNodeSelected,
      ),
      _SubscriptionsTab(onImported: _onNodesImported),
      _SettingsTab(nodeCount: _nodes.length),
    ];

    return Scaffold(
      body: tabs[_tabIndex],
      bottomNavigationBar: NavigationBar(
        selectedIndex: _tabIndex,
        onDestinationSelected: (i) => setState(() => _tabIndex = i),
        backgroundColor: const Color(0xFF16161E),
        surfaceTintColor: Colors.transparent,
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
    final accent = isConnected ? const Color(0xFF00E676) : const Color(0xFF00B4D8);

    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 24),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            const SizedBox(height: 20),
            const Text(
              'ClashForge',
              style: TextStyle(
                color: Colors.white,
                fontSize: 24,
                fontWeight: FontWeight.bold,
                letterSpacing: -0.5,
              ),
            ),
            const Spacer(),
            Center(
              child: GestureDetector(
                onTap: onToggle,
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 300),
                  width: 180,
                  height: 180,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: accent.withAlpha(18),
                    border: Border.all(color: accent, width: 3),
                    boxShadow: [
                      BoxShadow(
                        color: accent.withAlpha(isConnected ? 80 : 40),
                        blurRadius: isConnected ? 48 : 24,
                        spreadRadius: isConnected ? 8 : 2,
                      ),
                    ],
                  ),
                  child: isConnecting
                      ? Center(child: CircularProgressIndicator(color: accent, strokeWidth: 2.5))
                      : Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Icon(isConnected ? Icons.shield : Icons.shield_outlined, size: 56, color: accent),
                            const SizedBox(height: 8),
                            Text(
                              isConnected ? 'ON' : 'OFF',
                              style: TextStyle(
                                color: accent,
                                fontWeight: FontWeight.bold,
                                fontSize: 16,
                                letterSpacing: 3,
                              ),
                            ),
                          ],
                        ),
                ),
              ),
            ),
            const SizedBox(height: 20),
            Center(
              child: Text(
                connectionStatus,
                style: const TextStyle(color: Colors.white54, fontSize: 14),
              ),
            ),
            const Spacer(),
            GestureDetector(
              onTap: onTapNode,
              child: Container(
                width: double.infinity,
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: const Color(0xFF16161E),
                  borderRadius: BorderRadius.circular(16),
                  border: Border.all(color: Colors.white12),
                ),
                child: Row(
                  children: [
                    Container(
                      width: 40,
                      height: 40,
                      decoration: BoxDecoration(
                        color: const Color(0xFF00B4D8).withAlpha(20),
                        borderRadius: BorderRadius.circular(10),
                      ),
                      child: const Icon(Icons.language, color: Color(0xFF00B4D8), size: 20),
                    ),
                    const SizedBox(width: 12),
                    Expanded(
                      child: selectedNode == null
                          ? const Text('No node selected', style: TextStyle(color: Colors.white38))
                          : Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(selectedNode!.name,
                                    style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600)),
                                const SizedBox(height: 2),
                                Text(
                                  '${selectedNode!.type.toUpperCase()} · ${selectedNode!.server}:${selectedNode!.port}',
                                  style: const TextStyle(color: Colors.white38, fontSize: 12),
                                ),
                              ],
                            ),
                    ),
                    const Icon(Icons.chevron_right, color: Colors.white24),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 32),
          ],
        ),
      ),
    );
  }
}

// ─────────────────────────────────────────────────────────────
// Tab 2 — Proxy node list
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
                    style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: -0.5)),
                const Spacer(),
                Text('${nodes.length} nodes', style: const TextStyle(color: Colors.white38, fontSize: 13)),
              ],
            ),
          ),
          Expanded(
            child: nodes.isEmpty
                ? const Center(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Icon(Icons.cloud_off_outlined, size: 48, color: Colors.white24),
                        SizedBox(height: 12),
                        Text('No nodes loaded.\nImport a subscription first.',
                            textAlign: TextAlign.center,
                            style: TextStyle(color: Colors.white38, height: 1.6)),
                      ],
                    ),
                  )
                : ListView.separated(
                    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
                    itemCount: nodes.length,
                    separatorBuilder: (_, __) => const SizedBox(height: 6),
                    itemBuilder: (context, i) {
                      final node = nodes[i];
                      final selected = node == selectedNode;
                      const accent = Color(0xFF00B4D8);
                      return InkWell(
                        borderRadius: BorderRadius.circular(14),
                        onTap: () => onSelect(node),
                        child: AnimatedContainer(
                          duration: const Duration(milliseconds: 200),
                          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 14),
                          decoration: BoxDecoration(
                            color: selected ? accent.withAlpha(25) : const Color(0xFF16161E),
                            borderRadius: BorderRadius.circular(14),
                            border: Border.all(color: selected ? accent : Colors.white12),
                          ),
                          child: Row(
                            children: [
                              Container(
                                width: 36,
                                height: 36,
                                decoration: BoxDecoration(
                                  color: accent.withAlpha(20),
                                  borderRadius: BorderRadius.circular(8),
                                ),
                                child: const Icon(Icons.language, color: accent, size: 18),
                              ),
                              const SizedBox(width: 12),
                              Expanded(
                                child: Column(
                                  crossAxisAlignment: CrossAxisAlignment.start,
                                  children: [
                                    Text(node.name,
                                        style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600, fontSize: 14)),
                                    const SizedBox(height: 2),
                                    Text('${node.type.toUpperCase()} · ${node.server}:${node.port}',
                                        style: const TextStyle(color: Colors.white38, fontSize: 12)),
                                  ],
                                ),
                              ),
                              if (selected) const Icon(Icons.check_circle, color: accent, size: 20),
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
          logger.error('subscription', 'Fetch failed', fields: {'status': response.statusCode});
          setState(() { _loading = false; _success = false; _message = 'Fetch failed: HTTP ${response.statusCode}'; });
          return;
        }
        content = response.body;
      }

      final nodes = SubscriptionParser.parse(content);
      logger.info('subscription', 'Parsed nodes', fields: {'count': nodes.length});
      if (nodes.isEmpty) {
        logger.warn('subscription', 'No nodes parsed — check subscription format');
      }

      // Ask user for a nickname before saving
      final defaultName = await SubscriptionStore.generateDefaultNickname();
      if (!mounted) return;
      final nickname = await _showNicknameDialog(defaultName);
      if (!mounted) return;
      if (nickname == null) {
        setState(() { _loading = false; _success = false; _message = null; });
        return;
      }

      widget.onImported(nodes, url: input.startsWith('http') ? input : '', nickname: nickname);
      setState(() { _loading = false; _success = true; _message = 'Imported ${nodes.length} nodes as "$nickname"'; });
    } catch (e) {
      logger.error('subscription', 'Import error: $e');
      setState(() { _loading = false; _success = false; _message = 'Error: $e'; });
    }
  }

  Future<String?> _showNicknameDialog(String defaultName) async {
    final controller = TextEditingController(text: defaultName);
    return showDialog<String>(
      context: context,
      barrierDismissible: false,
      builder: (ctx) => AlertDialog(
        backgroundColor: const Color(0xFF16161E),
        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(16)),
        title: const Text('Name this subscription',
            style: TextStyle(color: Colors.white, fontSize: 17, fontWeight: FontWeight.w600)),
        content: TextField(
          controller: controller,
          autofocus: true,
          style: const TextStyle(color: Colors.white, fontSize: 14),
          decoration: InputDecoration(
            hintText: 'e.g. Work VPN',
            hintStyle: const TextStyle(color: Colors.white38),
            filled: true,
            fillColor: const Color(0xFF0A0A0F),
            contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Colors.white12)),
            enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Colors.white12)),
            focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Color(0xFF00B4D8))),
          ),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.pop(ctx),
            child: const Text('Cancel', style: TextStyle(color: Colors.white54)),
          ),
          FilledButton(
            style: FilledButton.styleFrom(
              backgroundColor: const Color(0xFF00B4D8),
              foregroundColor: Colors.black,
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
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
                style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: -0.5)),
            const SizedBox(height: 24),
            Container(
              padding: const EdgeInsets.all(20),
              decoration: BoxDecoration(
                color: const Color(0xFF16161E),
                borderRadius: BorderRadius.circular(16),
                border: Border.all(color: Colors.white12),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  const Text('SUBSCRIPTION URL',
                      style: TextStyle(color: Colors.white38, fontSize: 11, letterSpacing: 1.2, fontWeight: FontWeight.w600)),
                  const SizedBox(height: 10),
                  TextField(
                    controller: _urlController,
                    style: const TextStyle(color: Colors.white, fontSize: 14),
                    decoration: InputDecoration(
                      hintText: 'https://',
                      hintStyle: const TextStyle(color: Colors.white24),
                      filled: true,
                      fillColor: const Color(0xFF0A0A0F),
                      contentPadding: const EdgeInsets.symmetric(horizontal: 14, vertical: 12),
                      border: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Colors.white12)),
                      enabledBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Colors.white12)),
                      focusedBorder: OutlineInputBorder(borderRadius: BorderRadius.circular(10), borderSide: const BorderSide(color: Color(0xFF00B4D8))),
                      suffixIcon: _urlController.text.isNotEmpty
                          ? IconButton(
                              icon: const Icon(Icons.clear, color: Colors.white38, size: 18),
                              onPressed: () { _urlController.clear(); setState(() {}); },
                            )
                          : null,
                    ),
                    onChanged: (_) => setState(() {}),
                  ),
                  const SizedBox(height: 16),
                  SizedBox(
                    width: double.infinity,
                    height: 46,
                    child: FilledButton.icon(
                      onPressed: _loading ? null : _import,
                      style: FilledButton.styleFrom(
                        backgroundColor: const Color(0xFF00B4D8),
                        foregroundColor: Colors.black,
                        shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(10)),
                      ),
                      icon: _loading
                          ? const SizedBox(width: 16, height: 16, child: CircularProgressIndicator(strokeWidth: 2, color: Colors.black54))
                          : const Icon(Icons.cloud_download, size: 18),
                      label: Text(_loading ? 'Importing…' : 'Import'),
                    ),
                  ),
                ],
              ),
            ),
            if (_message != null) ...[
              const SizedBox(height: 16),
              Container(
                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                decoration: BoxDecoration(
                  color: _success ? const Color(0xFF00E676).withAlpha(18) : Colors.red.withAlpha(18),
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(color: _success ? const Color(0xFF00E676).withAlpha(80) : Colors.red.withAlpha(80)),
                ),
                child: Row(
                  children: [
                    Icon(_success ? Icons.check_circle_outline : Icons.error_outline,
                        color: _success ? const Color(0xFF00E676) : Colors.red, size: 18),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(_message!,
                          style: TextStyle(color: _success ? const Color(0xFF00E676) : Colors.red, fontSize: 13)),
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
            padding: EdgeInsets.fromLTRB(24, 20, 24, 20),
            child: Text('Settings',
                style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: -0.5)),
          ),
          _SettingsTile(
            icon: Icons.terminal_outlined,
            title: 'Logs',
            subtitle: 'View runtime and system logs',
            onTap: () => Navigator.push(
              context,
              MaterialPageRoute(builder: (_) => const Scaffold(
                backgroundColor: Color(0xFF0A0A0F),
                body: _LogsTab(),
              )),
            ),
          ),
          const SizedBox(height: 8),
          _SettingsTile(
            icon: Icons.info_outline,
            title: 'About',
            subtitle: 'App version, runtime status, memory',
            onTap: () => Navigator.push(
              context,
              MaterialPageRoute(builder: (_) => Scaffold(
                backgroundColor: const Color(0xFF0A0A0F),
                body: _AboutTab(nodeCount: nodeCount),
              )),
            ),
          ),
        ],
      ),
    );
  }
}

class _SettingsTile extends StatelessWidget {
  const _SettingsTile({
    required this.icon,
    required this.title,
    required this.subtitle,
    required this.onTap,
  });

  final IconData icon;
  final String title;
  final String subtitle;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      child: InkWell(
        borderRadius: BorderRadius.circular(14),
        onTap: onTap,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 16),
          decoration: BoxDecoration(
            color: const Color(0xFF16161E),
            borderRadius: BorderRadius.circular(14),
            border: Border.all(color: Colors.white12),
          ),
          child: Row(
            children: [
              Container(
                width: 40,
                height: 40,
                decoration: BoxDecoration(
                  color: const Color(0xFF00B4D8).withAlpha(20),
                  borderRadius: BorderRadius.circular(10),
                ),
                child: Icon(icon, color: const Color(0xFF00B4D8), size: 20),
              ),
              const SizedBox(width: 14),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(title, style: const TextStyle(color: Colors.white, fontWeight: FontWeight.w600, fontSize: 15)),
                    const SizedBox(height: 2),
                    Text(subtitle, style: const TextStyle(color: Colors.white38, fontSize: 13)),
                  ],
                ),
              ),
              const Icon(Icons.chevron_right, color: Colors.white24),
            ],
          ),
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
  String _filter = 'all'; // all | debug | info | warn | error
  bool _autoScroll = true;

  static const _levels = ['all', 'debug', 'info', 'warn', 'error'];

  static const _levelColors = {
    'debug': Color(0xFF90A4AE),
    'info': Color(0xFF00B4D8),
    'warn': Color(0xFFFFB74D),
    'error': Color(0xFFEF5350),
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
    final text = AppLogger.instance.export();
    await Clipboard.setData(ClipboardData(text: text));
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(content: Text('Logs copied to clipboard'), duration: Duration(seconds: 2)),
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
            padding: const EdgeInsets.fromLTRB(8, 16, 12, 8),
            child: Row(
              children: [
                if (Navigator.canPop(context))
                  IconButton(
                    icon: const Icon(Icons.arrow_back, color: Colors.white54),
                    onPressed: () => Navigator.pop(context),
                    padding: EdgeInsets.zero,
                  ),
                const Text('Logs',
                    style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: -0.5)),
                const Spacer(),
                IconButton(
                  tooltip: 'Copy all',
                  icon: const Icon(Icons.copy_outlined, size: 20, color: Colors.white54),
                  onPressed: _copyAll,
                ),
                IconButton(
                  tooltip: _autoScroll ? 'Auto-scroll on' : 'Auto-scroll off',
                  icon: Icon(Icons.vertical_align_bottom,
                      size: 20, color: _autoScroll ? const Color(0xFF00B4D8) : Colors.white24),
                  onPressed: () => setState(() => _autoScroll = !_autoScroll),
                ),
                IconButton(
                  tooltip: 'Clear',
                  icon: const Icon(Icons.delete_outline, size: 20, color: Colors.white54),
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
              padding: const EdgeInsets.symmetric(horizontal: 16),
              children: _levels.map((lvl) {
                final active = _filter == lvl;
                final color = lvl == 'all' ? const Color(0xFF00B4D8) : (_levelColors[lvl] ?? Colors.white54);
                return Padding(
                  padding: const EdgeInsets.only(right: 8),
                  child: GestureDetector(
                    onTap: () => setState(() => _filter = lvl),
                    child: AnimatedContainer(
                      duration: const Duration(milliseconds: 150),
                      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 4),
                      decoration: BoxDecoration(
                        color: active ? color.withAlpha(30) : Colors.transparent,
                        borderRadius: BorderRadius.circular(20),
                        border: Border.all(color: active ? color : Colors.white12),
                      ),
                      child: Text(
                        lvl.toUpperCase(),
                        style: TextStyle(
                          color: active ? color : Colors.white38,
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

          // Log entries
          Expanded(
            child: entries.isEmpty
                ? Center(
                    child: Column(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Icon(Icons.terminal, size: 40, color: Colors.white12),
                        const SizedBox(height: 12),
                        Text(
                          _filter == 'all' ? 'No logs yet.' : 'No $_filter logs.',
                          style: const TextStyle(color: Colors.white24),
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
    'debug': Color(0xFF90A4AE),
    'info': Color(0xFF00B4D8),
    'warn': Color(0xFFFFB74D),
    'error': Color(0xFFEF5350),
  };

  @override
  Widget build(BuildContext context) {
    final color = _bg[entry.level] ?? const Color(0xFF90A4AE);

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 3),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Timestamp
          Text(entry.timeLabel,
              style: const TextStyle(color: Colors.white24, fontSize: 10, fontFamily: 'monospace')),
          const SizedBox(width: 6),

          // Level badge
          Container(
            width: 38,
            padding: const EdgeInsets.symmetric(vertical: 1),
            decoration: BoxDecoration(
              color: color.withAlpha(25),
              borderRadius: BorderRadius.circular(4),
              border: Border.all(color: color.withAlpha(80), width: 0.5),
            ),
            child: Text(
              entry.level.toUpperCase(),
              textAlign: TextAlign.center,
              style: TextStyle(color: color, fontSize: 9, fontWeight: FontWeight.w800, letterSpacing: 0.5),
            ),
          ),
          const SizedBox(width: 6),

          // Component chip
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
            decoration: BoxDecoration(
              color: Colors.white.withAlpha(10),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              entry.component,
              style: const TextStyle(color: Colors.white38, fontSize: 10, fontFamily: 'monospace'),
            ),
          ),
          const SizedBox(width: 6),

          // Message + fields
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text(entry.message,
                    style: const TextStyle(color: Color(0xDEFFFFFF), fontSize: 12, fontFamily: 'monospace')),
                if (entry.fields.isNotEmpty) ...[
                  const SizedBox(height: 3),
                  Wrap(
                    spacing: 4,
                    runSpacing: 3,
                    children: entry.fields.entries.map((kv) {
                      return Container(
                        padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 1),
                        decoration: BoxDecoration(
                          color: const Color(0xFF1E2A35),
                          borderRadius: BorderRadius.circular(3),
                        ),
                        child: Text(
                          '${kv.key}=${kv.value}',
                          style: const TextStyle(color: Colors.white54, fontSize: 10, fontFamily: 'monospace'),
                        ),
                      );
                    }).toList(),
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
// Tab 5 — About
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

  @override
  void initState() {
    super.initState();
    _refresh();
  }

  Future<void> _refresh() async {
    setState(() => _loading = true);
    final info = await VpnManager.getSystemInfo();
    if (mounted) setState(() { _info = info; _loading = false; });
  }

  @override
  Widget build(BuildContext context) {
    const accent = Color(0xFF00B4D8);

    Widget _row(String label, String value, {Color? valueColor}) => Padding(
      padding: const EdgeInsets.symmetric(vertical: 10),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.spaceBetween,
        children: [
          Text(label, style: const TextStyle(color: Colors.white54, fontSize: 14)),
          Text(value, style: TextStyle(color: valueColor ?? Colors.white, fontSize: 14, fontWeight: FontWeight.w600)),
        ],
      ),
    );

    final mihomoRunning = _info['mihomo_running'] as bool? ?? false;
    final vpnRunning    = _info['vpn_running']    as bool? ?? false;
    final pid           = _info['mihomo_pid']     as int?  ?? -1;
    final appVersion    = _info['app_version']    as String? ?? '—';
    final buildNum      = _info['build_number'];
    final appPss        = _info['memory_app_pss_mb']   as double? ?? 0.0;
    final avail         = _info['memory_available_mb'] as double? ?? 0.0;
    final abi           = _info['device_abi']     as String? ?? '—';

    return SafeArea(
      child: RefreshIndicator(
        onRefresh: _refresh,
        color: accent,
        child: ListView(
          padding: const EdgeInsets.symmetric(horizontal: 24),
          children: [
            const SizedBox(height: 20),
            Row(children: [
              if (Navigator.canPop(context))
                IconButton(
                  icon: const Icon(Icons.arrow_back, color: Colors.white54),
                  onPressed: () => Navigator.pop(context),
                  padding: EdgeInsets.zero,
                ),
              const Text('About', style: TextStyle(color: Colors.white, fontSize: 24, fontWeight: FontWeight.bold, letterSpacing: -0.5)),
              const Spacer(),
              if (_loading)
                const SizedBox(width: 18, height: 18, child: CircularProgressIndicator(strokeWidth: 2, color: accent))
              else
                IconButton(icon: const Icon(Icons.refresh, color: Colors.white38, size: 20), onPressed: _refresh, padding: EdgeInsets.zero),
            ]),
            const SizedBox(height: 20),

            // App section
            _section('APPLICATION', [
              _row('Version',      '$appVersion ($buildNum)'),
              _row('Nodes loaded', '${widget.nodeCount}'),
              _row('Device ABI',   abi),
            ]),
            const SizedBox(height: 16),

            // Runtime section
            _section('RUNTIME', [
              _row('VPN',    vpnRunning    ? 'Running'  : 'Stopped',
                  valueColor: vpnRunning    ? const Color(0xFF00E676) : Colors.white38),
              _row('Mihomo', mihomoRunning ? 'Running (PID $pid)' : 'Stopped',
                  valueColor: mihomoRunning ? const Color(0xFF00E676) : Colors.white38),
            ]),
            const SizedBox(height: 16),

            // Memory section
            _section('MEMORY', [
              _row('App (PSS)',  '${appPss.toStringAsFixed(1)} MB'),
              _row('Available',  '${avail.toStringAsFixed(0)} MB'),
            ]),
            const SizedBox(height: 24),
            const Center(child: Text('Pull down to refresh', style: TextStyle(color: Colors.white24, fontSize: 12))),
          ],
        ),
      ),
    );
  }

  Widget _section(String title, List<Widget> rows) => Container(
    padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
    decoration: BoxDecoration(
      color: const Color(0xFF16161E),
      borderRadius: BorderRadius.circular(16),
      border: Border.all(color: Colors.white12),
    ),
    child: Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Padding(
          padding: const EdgeInsets.only(top: 8, bottom: 4),
          child: Text(title, style: const TextStyle(color: Colors.white24, fontSize: 11, letterSpacing: 1.2, fontWeight: FontWeight.w600)),
        ),
        const Divider(color: Colors.white12, height: 1),
        ...rows,
      ],
    ),
  );
}
