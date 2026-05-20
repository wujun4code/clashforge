import 'package:flutter/material.dart';
import 'package:http/http.dart' as http;
import 'subscription/subscription_parser.dart';
import 'subscription/proxy_node.dart';
import 'config/vpn_manager.dart';

void main() {
  runApp(const ClashForgeApp());
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
        cardTheme: CardTheme(
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
  int _tabIndex = 0;
  bool _isConnected = false;
  bool _isConnecting = false;
  String _connectionStatus = 'Tap to connect';
  final List<ProxyNode> _nodes = [];
  ProxyNode? _selectedNode;

  void _onNodesImported(List<ProxyNode> nodes) {
    setState(() {
      _nodes
        ..clear()
        ..addAll(nodes);
      _selectedNode ??= nodes.isEmpty ? null : nodes.first;
    });
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
    try {
      if (_isConnected) {
        await VpnManager.stopVpn();
        setState(() {
          _isConnected = false;
          _connectionStatus = 'Tap to connect';
        });
      } else {
        final res = await VpnManager.startVpn();
        setState(() {
          _isConnected = true;
          _connectionStatus = 'Connected ($res)';
        });
      }
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

            // Connection button
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
                      ? Center(
                          child: CircularProgressIndicator(
                            color: accent,
                            strokeWidth: 2.5,
                          ),
                        )
                      : Column(
                          mainAxisAlignment: MainAxisAlignment.center,
                          children: [
                            Icon(
                              isConnected ? Icons.shield : Icons.shield_outlined,
                              size: 56,
                              color: accent,
                            ),
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
                style: const TextStyle(
                  color: Colors.white54,
                  fontSize: 14,
                ),
              ),
            ),
            const Spacer(),

            // Selected node card
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
                          ? const Text(
                              'No node selected',
                              style: TextStyle(color: Colors.white38),
                            )
                          : Column(
                              crossAxisAlignment: CrossAxisAlignment.start,
                              children: [
                                Text(
                                  selectedNode!.name,
                                  style: const TextStyle(
                                    color: Colors.white,
                                    fontWeight: FontWeight.w600,
                                  ),
                                ),
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
  const _ProxiesTab({
    required this.nodes,
    required this.selectedNode,
    required this.onSelect,
  });

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
                const Text(
                  'Proxies',
                  style: TextStyle(
                    color: Colors.white,
                    fontSize: 24,
                    fontWeight: FontWeight.bold,
                    letterSpacing: -0.5,
                  ),
                ),
                const Spacer(),
                Text(
                  '${nodes.length} nodes',
                  style: const TextStyle(color: Colors.white38, fontSize: 13),
                ),
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
                        Text(
                          'No nodes loaded.\nImport a subscription first.',
                          textAlign: TextAlign.center,
                          style: TextStyle(color: Colors.white38, height: 1.6),
                        ),
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
                            border: Border.all(
                              color: selected ? accent : Colors.white12,
                            ),
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
                                    Text(
                                      node.name,
                                      style: const TextStyle(
                                        color: Colors.white,
                                        fontWeight: FontWeight.w600,
                                        fontSize: 14,
                                      ),
                                    ),
                                    const SizedBox(height: 2),
                                    Text(
                                      '${node.type.toUpperCase()} · ${node.server}:${node.port}',
                                      style: const TextStyle(color: Colors.white38, fontSize: 12),
                                    ),
                                  ],
                                ),
                              ),
                              if (selected)
                                const Icon(Icons.check_circle, color: accent, size: 20),
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

class _SubscriptionsTab extends StatefulWidget {
  const _SubscriptionsTab({required this.onImported});
  final ValueChanged<List<ProxyNode>> onImported;

  @override
  State<_SubscriptionsTab> createState() => _SubscriptionsTabState();
}

class _SubscriptionsTabState extends State<_SubscriptionsTab> {
  final _urlController = TextEditingController();
  bool _loading = false;
  String? _message;
  bool _success = false;

  @override
  void dispose() {
    _urlController.dispose();
    super.dispose();
  }

  Future<void> _import() async {
    final input = _urlController.text.trim();
    if (input.isEmpty) return;
    setState(() {
      _loading = true;
      _message = null;
    });
    try {
      String content = input;

      // If it's a URL, fetch the content first
      if (input.startsWith('http://') || input.startsWith('https://')) {
        final response = await http.get(Uri.parse(input));
        if (response.statusCode != 200) {
          setState(() {
            _loading = false;
            _success = false;
            _message = 'Fetch failed: HTTP ${response.statusCode}';
          });
          return;
        }
        content = response.body;
      }

      final nodes = SubscriptionParser.parse(content);
      widget.onImported(nodes);
      setState(() {
        _loading = false;
        _success = true;
        _message = 'Imported ${nodes.length} nodes successfully';
      });
    } catch (e) {
      setState(() {
        _loading = false;
        _success = false;
        _message = 'Error: $e';
      });
    }
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
            const Text(
              'Subscriptions',
              style: TextStyle(
                color: Colors.white,
                fontSize: 24,
                fontWeight: FontWeight.bold,
                letterSpacing: -0.5,
              ),
            ),
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
                  const Text(
                    'SUBSCRIPTION URL',
                    style: TextStyle(
                      color: Colors.white38,
                      fontSize: 11,
                      letterSpacing: 1.2,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
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
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: Colors.white12),
                      ),
                      enabledBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: Colors.white12),
                      ),
                      focusedBorder: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(10),
                        borderSide: const BorderSide(color: Color(0xFF00B4D8)),
                      ),
                      suffixIcon: _urlController.text.isNotEmpty
                          ? IconButton(
                              icon: const Icon(Icons.clear, color: Colors.white38, size: 18),
                              onPressed: () {
                                _urlController.clear();
                                setState(() {});
                              },
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
                        shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(10),
                        ),
                      ),
                      icon: _loading
                          ? const SizedBox(
                              width: 16,
                              height: 16,
                              child: CircularProgressIndicator(
                                strokeWidth: 2,
                                color: Colors.black54,
                              ),
                            )
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
                  color: _success
                      ? const Color(0xFF00E676).withAlpha(18)
                      : Colors.red.withAlpha(18),
                  borderRadius: BorderRadius.circular(10),
                  border: Border.all(
                    color: _success
                        ? const Color(0xFF00E676).withAlpha(80)
                        : Colors.red.withAlpha(80),
                  ),
                ),
                child: Row(
                  children: [
                    Icon(
                      _success ? Icons.check_circle_outline : Icons.error_outline,
                      color: _success ? const Color(0xFF00E676) : Colors.red,
                      size: 18,
                    ),
                    const SizedBox(width: 10),
                    Expanded(
                      child: Text(
                        _message!,
                        style: TextStyle(
                          color: _success ? const Color(0xFF00E676) : Colors.red,
                          fontSize: 13,
                        ),
                      ),
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
