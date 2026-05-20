class LogEntry {
  final DateTime time;
  final String level; // debug | info | warn | error
  final String component;
  final String message;
  final Map<String, dynamic> fields;

  const LogEntry({
    required this.time,
    required this.level,
    required this.component,
    required this.message,
    this.fields = const {},
  });

  String get timeLabel {
    final t = time;
    final h = t.hour.toString().padLeft(2, '0');
    final m = t.minute.toString().padLeft(2, '0');
    final s = t.second.toString().padLeft(2, '0');
    final ms = t.millisecond.toString().padLeft(3, '0');
    return '$h:$m:$s.$ms';
  }

  @override
  String toString() {
    final buf = StringBuffer('[${time.toIso8601String()}] [${level.toUpperCase()}] [$component] $message');
    if (fields.isNotEmpty) {
      buf.write(' | ');
      buf.write(fields.entries.map((e) => '${e.key}=${e.value}').join(' '));
    }
    return buf.toString();
  }
}
