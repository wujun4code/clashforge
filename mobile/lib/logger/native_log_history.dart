import 'dart:convert';
import 'log_entry.dart';

/// Page-local bounded snapshot. Subscribe before reading history; shared native
/// event IDs merge the overlap without collapsing legitimate repeated messages.
class NativeLogHistory {
  NativeLogHistory({this.capacity = 500});
  final int capacity;
  final List<Map<String, dynamic>> _records = [];
  List<LogEntry> get entries => _records.map(_entry).toList(growable: false);

  void addLive(dynamic event) {
    final record = _decode(event.toString());
    if (record == null) return;
    final fields = record['fields'];
    if (fields is! Map || fields['_nativeEventId'] == null) {
      // iOS/legacy streams have no shared ID. Never collapse two live
      // occurrences merely because all their text/timestamp fields match.
      _merge([..._records, record], historyFirst: true);
    } else {
      _merge([record]);
    }
  }

  void addHistory(String ndjson) {
    _merge(ndjson.split('\n').map(_decode).whereType<Map<String, dynamic>>(),
        historyFirst: true);
  }

  void clear() => _records.clear();

  static Map<String, dynamic>? _decode(String line) {
    try {
      final value = jsonDecode(line);
      if (value is Map<String, dynamic> &&
          value['ts'] is num &&
          value['message'] is String) {
        return value;
      }
    } catch (_) {/* A partial/invalid record must not hide other records. */}
    return null;
  }

  String _identity(Map<String, dynamic> record) {
    final fields = record['fields'];
    final id = fields is Map ? fields['_nativeEventId'] : null;
    // Legacy entries cannot overlap a new bridge event (all new events have IDs).
    return id == null ? jsonEncode(record) : 'id:$id';
  }

  void _merge(Iterable<Map<String, dynamic>> incoming,
      {bool historyFirst = false}) {
    // Snapshot order wins equal-timestamp ties and its persisted timestamp wins
    // over the independently stamped live envelope. Match occurrence counts,
    // not a Set of text: legacy logs may legitimately repeat byte-for-byte.
    final primary = historyFirst ? incoming.toList() : _records.toList();
    final secondary = historyFirst ? _records.toList() : incoming.toList();
    final existing = <String, int>{};
    for (final record in primary) {
      existing.update(_identity(record), (n) => n + 1, ifAbsent: () => 1);
    }
    final merged = [...primary];
    final seen = <String, int>{};
    for (final record in secondary) {
      final key = _identity(record);
      final count = seen.update(key, (n) => n + 1, ifAbsent: () => 1);
      if (count > (existing[key] ?? 0)) merged.add(record);
    }
    // Stable ordering for equal timestamps, retaining source occurrence order.
    final indexed = merged.asMap().entries.toList();
    indexed.sort((a, b) {
      final byTime = (a.value['ts'] as num).compareTo(b.value['ts'] as num);
      return byTime != 0 ? byTime : a.key.compareTo(b.key);
    });
    _records
      ..clear()
      ..addAll(indexed
          .skip(indexed.length > capacity ? indexed.length - capacity : 0)
          .map((item) => item.value));
  }

  static LogEntry _entry(Map<String, dynamic> record) {
    final fields = Map<String, dynamic>.from(record['fields'] as Map? ?? {});
    fields.remove('_nativeEventId');
    return LogEntry(
      time: DateTime.fromMillisecondsSinceEpoch((record['ts'] as num).toInt()),
      level: record['level'] as String? ?? 'info',
      component: record['component'] as String? ?? 'native',
      message: record['message'] as String,
      fields: fields,
    );
  }
}
