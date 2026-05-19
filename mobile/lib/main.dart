import 'package:flutter/material.dart';
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
      title: 'ClashForge Mobile',
      theme: ThemeData(
        colorScheme: ColorScheme.fromSeed(
          seedColor: Colors.deepOrange,
          brightness: Brightness.dark,
        ),
        useMaterial3: true,
      ),
      home: const HomeScreen(),
    );
  }
}

class HomeScreen extends StatefulWidget {
  const HomeScreen({super.key});

  @override
  State<HomeScreen> createState() => _HomeScreenState();
}

class _HomeScreenState extends State<HomeScreen> {
  bool _isConnected = false;
  String _statusMessage = 'Disconnected';
  final List<ProxyNode> _nodes = [];
  String _subscriptionUrl = '';

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('ClashForge Mobile'),
        centerTitle: true,
      ),
      body: Padding(
        padding: const EdgeInsets.all(16.0),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            const SizedBox(height: 30),
            // Giant connection toggle button (Home Tab principle)
            Center(
              child: GestureDetector(
                onTap: _toggleVpn,
                child: Container(
                  width: 160,
                  height: 160,
                  decoration: BoxDecoration(
                    shape: BoxShape.circle,
                    color: _isConnected ? Colors.green.withAlpha(51) : Colors.red.withAlpha(51),
                    border: Border.all(
                      color: _isConnected ? Colors.green : Colors.red,
                      width: 4,
                    ),
                  ),
                  child: Icon(
                    Icons.power_settings_new,
                    size: 64,
                    color: _isConnected ? Colors.green : Colors.red,
                  ),
                ),
              ),
            ),
            const SizedBox(height: 20),
            Text(
              _statusMessage,
              style: const TextStyle(fontSize: 18, fontWeight: FontWeight.bold),
            ),
            const SizedBox(height: 40),
            // Import section
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16.0),
                child: Column(
                  children: [
                    TextField(
                      decoration: const InputDecoration(
                        labelText: 'Subscription URL / Content',
                        border: OutlineInputBorder(),
                      ),
                      onChanged: (val) {
                        _subscriptionUrl = val;
                      },
                    ),
                    const SizedBox(height: 12),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceEvenly,
                      children: [
                        ElevatedButton.icon(
                          onPressed: _importSubscription,
                          icon: const Icon(Icons.download),
                          label: const Text('Import'),
                        ),
                      ],
                    )
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            Expanded(
              child: _nodes.isEmpty
                  ? const Center(child: Text('No proxy nodes loaded.'))
                  : ListView.builder(
                      itemCount: _nodes.length,
                      itemBuilder: (context, idx) {
                        final node = _nodes[idx];
                        return ListTile(
                          leading: const Icon(Icons.lan),
                          title: Text(node.name),
                          subtitle: Text('${node.type.toUpperCase()} - ${node.server}:${node.port}'),
                        );
                      },
                    ),
            ),
          ],
        ),
      ),
    );
  }

  void _importSubscription() {
    if (_subscriptionUrl.isEmpty) return;
    try {
      final parsed = SubscriptionParser.parse(_subscriptionUrl);
      setState(() {
        _nodes.clear();
        _nodes.addAll(parsed);
        _statusMessage = 'Imported ${_nodes.length} nodes successfully';
      });
    } catch (e) {
      setState(() {
        _statusMessage = 'Failed to parse subscription: $e';
      });
    }
  }

  void _toggleVpn() async {
    if (_isConnected) {
      await VpnManager.stopVpn();
      setState(() {
        _isConnected = false;
        _statusMessage = 'Disconnected';
      });
    } else {
      final res = await VpnManager.startVpn();
      setState(() {
        _isConnected = true;
        _statusMessage = 'Connected (Status: $res)';
      });
    }
  }
}
