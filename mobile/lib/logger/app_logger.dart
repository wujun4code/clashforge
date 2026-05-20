import 'package:flutter/foundation.dart';
import 'log_entry.dart';

class AppLogger extends ChangeNotifier {
  AppLogger._();
  static final AppLogger instance = AppLogger._();

  static const int _capacity = 500;
  final List<LogEntry> _buf = [];

  List<LogEntry> get entries => List.unmodifiable(_buf);

  void log(
    String level,
    String component,
    String message, {
    Map<String, dynamic> fields = const {},
  }) {
    if (_buf.length >= _capacity) _buf.removeAt(0);
    _buf.add(LogEntry(
      time: DateTime.now(),
      level: level,
      component: component,
      message: message,
      fields: fields,
    ));
    notifyListeners();
  }

  void debug(String component, String message, {Map<String, dynamic> fields = const {}}) =>
      log('debug', component, message, fields: fields);

  void info(String component, String message, {Map<String, dynamic> fields = const {}}) =>
      log('info', component, message, fields: fields);

  void warn(String component, String message, {Map<String, dynamic> fields = const {}}) =>
      log('warn', component, message, fields: fields);

  void error(String component, String message, {Map<String, dynamic> fields = const {}}) =>
      log('error', component, message, fields: fields);

  void clear() {
    _buf.clear();
    notifyListeners();
  }

  String export() => _buf.map((e) => e.toString()).join('\n');
}
