import 'package:flutter/material.dart';
import 'package:flutter_localizations/flutter_localizations.dart';

// ─── Public API ───────────────────────────────────────────────────────────────

abstract class AppLocalizations {
  static AppLocalizations of(BuildContext context) {
    return Localizations.of<AppLocalizations>(context, AppLocalizations)!;
  }

  static const LocalizationsDelegate<AppLocalizations> delegate =
      _AppLocalizationsDelegate();

  static const List<LocalizationsDelegate<dynamic>> localizationsDelegates = [
    delegate,
    GlobalMaterialLocalizations.delegate,
    GlobalWidgetsLocalizations.delegate,
    GlobalCupertinoLocalizations.delegate,
  ];

  static const List<Locale> supportedLocales = [
    Locale('en'),
    Locale('zh'),
  ];

  // ── Navigation ─────────────────────────────────────────────────────────────
  String get navHome;
  String get navProxies;
  String get navSubscriptions;
  String get navSettings;

  // ── Status badges ──────────────────────────────────────────────────────────
  String get statusActive;
  String get statusIdle;

  // ── VPN card ───────────────────────────────────────────────────────────────
  String get vpnRunning;
  String get vpnIdle;
  String get btnConnect;
  String get btnDisconnect;
  String get btnMoreNodes;

  // ── Connection status messages ─────────────────────────────────────────────
  String get connTapToConnect;
  String get connGrantPermission;
  String get connConnected;
  String get connNodeSelectedTapConnect;
  String get connNodeSwitchPending;
  String connSwitchedTo(String nodeName);
  String connError(String error);

  // ── Private DNS warning ────────────────────────────────────────────────────
  String privateDnsWarningHostname(String specifier);
  String get privateDnsWarningAuto;

  // ── Connectivity section ───────────────────────────────────────────────────
  String get connectivityTitle;
  String get connectivitySubtitle;
  String get btnRecheck;
  String lastChecked(String time);
  String get hintClickRecheck;

  // ── Connectivity pane labels ───────────────────────────────────────────────
  String get sectionExitIp;
  String get categoryDirect;
  String get directSubtitle;
  String get categoryVpnExit;
  String get vpnExitSubtitle;
  String get sectionAccessCheck;
  String get categoryDirectPath;
  String get categoryVpnProxy;
  String get categoryAiVpnProxy;
  String get ipResolved;
  String get ipFailed;
  String get siteStatusOk;
  String get siteStatusError;

  // ── Browser DNS section ────────────────────────────────────────────────────
  String get browserDnsTitle;
  String get browserDnsSubtitle;
  String get hintClickRecheckBrowserDns;
  String get browserDnsPathLabel;
  String get statusHealthy;
  String get statusRisk;

  // ── Browser DNS check item names ───────────────────────────────────────────
  String get checkPrivateDns;
  String get checkMihomoDns;
  String get checkSystemDns;
  String get checkProxyChain;

  // ── Browser DNS check details ──────────────────────────────────────────────
  String get privateDnsOff;
  String privateDnsOnHostname(String specifier);
  String privateDnsOn(String mode);
  String get mihomoDnsNoRecord;
  String get systemDnsNoAddr;
  String proxyChainSuccess(int ms);

  // ── Browser DNS summaries ──────────────────────────────────────────────────
  String get summaryPrivateDnsOn;
  String get summaryMihomoDnsFailed;
  String get summarySystemDnsFailed;
  String get summaryProxyChainFailed;
  String get summaryAllOk;
  String browserDnsFailed(String error);

  // ── Node switch section ────────────────────────────────────────────────────
  String get nodeSwitchTitle;
  String nodeSwitchSubtitle(String group);
  String get hintNoNodes;
  String get linkViewAllNodes;

  // ── Proxies tab ────────────────────────────────────────────────────────────
  String get proxiesTitle;
  String nodesCount(int count);
  String get noNodesYet;
  String get noNodesHint;

  // ── Subscriptions tab ──────────────────────────────────────────────────────
  String get subscriptionsTitle;
  String get subscriptionUrlLabel;
  String get btnFetching;
  String get btnImport;
  String importedNodes(int count, String name);
  String fetchFailed(int code);
  String get pasteNodesLabel;
  String get pasteNodesDesc;
  String get btnParsing;
  String get btnImportAndGenerate;
  String get noValidNodes;
  String importedFromPaste(int count);
  String parseFailedMsg(Object error);
  String get savedSubscriptions;
  String get activeLabel;
  String get btnSwitch;
  String nodesCountSub(int count);
  String get subNameDialogTitle;
  String get subNameHint;
  String get btnCancel;
  String get btnSave;
  String get deleteSubTitle;
  String deleteSubContent(String name);
  String get btnDelete;

  // ── Settings ───────────────────────────────────────────────────────────────
  String get settingsTitle;
  String get tileLogsTitle;
  String get tileLogsSubtitle;
  String get tileUpdatesTitle;
  String get tileUpdatesSubtitle;
  String get tileAboutTitle;
  String get tileAboutSubtitle;
  String get tileLanguageTitle;
  String get tileLanguageSubtitle;

  // ── Logs ───────────────────────────────────────────────────────────────────
  String get logsTitle;
  String get tooltipCopyAll;
  String get tooltipAutoScrollOn;
  String get tooltipAutoScrollOff;
  String get tooltipClear;
  String get logsCopied;
  String get noLogsYet;
  String noFilterLogs(String filter);

  // ── About ──────────────────────────────────────────────────────────────────
  String get aboutTitle;
  String get sectionApplication;
  String get sectionUpdate;
  String get sectionRuntime;
  String get sectionMemory;
  String get rowVersion;
  String get rowNodesLoaded;
  String get rowDeviceAbi;
  String get rowVpn;
  String get rowMihomo;
  String get rowAppPss;
  String get rowAvailable;
  String get rowRunning;
  String get rowStopped;
  String rowMihomoRunning(int pid);
  String get checkingUpdates;
  String get updateStatus;
  String get updateCouldNotCheck;
  String get updateUpToDate;
  String get rowLatest;
  String downloadVersion(String tag);
  String get btnRecheck2;
  String get pullDownToRefresh;

  // ── Update sheet ───────────────────────────────────────────────────────────
  String get updateSheetTitle;
  String get updateChecking;
  String get updateCouldNotCheckLong;
  String get updateUpToDateTitle;
  String updateVersionLabel(String tag);
  String get updateAvailableTitle;
  String downloadBtn(String tag);
  String get allReleases;

  // ── Site descriptions ──────────────────────────────────────────────────────
  String get siteTaobaoName;
  String get siteTaobaoDesc;
  String get siteNeteaseName;
  String get siteNeteaseDesc;
  String get siteGitHubDesc;
  String get siteGoogleDesc;
  String get siteOpenAIDesc;
  String get siteClaudeDesc;

  // ── Language names ─────────────────────────────────────────────────────────
  String get langEnglish;
  String get langChinese;
}

// ─── Delegate ─────────────────────────────────────────────────────────────────

class _AppLocalizationsDelegate
    extends LocalizationsDelegate<AppLocalizations> {
  const _AppLocalizationsDelegate();

  @override
  bool isSupported(Locale locale) =>
      ['en', 'zh'].contains(locale.languageCode);

  @override
  Future<AppLocalizations> load(Locale locale) async {
    if (locale.languageCode == 'zh') return _AppLocalizationsZh();
    return _AppLocalizationsEn();
  }

  @override
  bool shouldReload(_AppLocalizationsDelegate old) => false;
}

// ─── English ──────────────────────────────────────────────────────────────────

class _AppLocalizationsEn extends AppLocalizations {
  @override String get navHome => 'Home';
  @override String get navProxies => 'Routes';
  @override String get navSubscriptions => 'Subscriptions';
  @override String get navSettings => 'Settings';

  @override String get statusActive => 'ACTIVE';
  @override String get statusIdle => 'IDLE';

  @override String get vpnRunning => 'Passage Active';
  @override String get vpnIdle => 'Passage Off';
  @override String get btnConnect => 'Connect';
  @override String get btnDisconnect => 'Disconnect';
  @override String get btnMoreNodes => 'More Nodes';

  @override String get connTapToConnect => 'Tap to connect';
  @override String get connGrantPermission => 'Grant network permission, then tap again';
  @override String get connConnected => 'Connected';
  @override String get connNodeSelectedTapConnect => 'Node selected, tap connect';
  @override String get connNodeSwitchPending => 'Node switch pending, retry after core is ready';
  @override String connSwitchedTo(String nodeName) => 'Switched to $nodeName';
  @override String connError(String error) => 'Error: $error';

  @override String privateDnsWarningHostname(String specifier) =>
      'Private DNS strict hostname mode ($specifier) detected. '
      'Android may not intercept DNS — browsers may report DNS_PROBE_FINISHED_BAD_CONFIG. '
      'Disable Private DNS in system settings and retry.';
  @override String get privateDnsWarningAuto =>
      'Private DNS auto mode detected. '
      'Android may not intercept DNS — browsers may report DNS_PROBE_FINISHED_BAD_CONFIG. '
      'Disable Private DNS in system settings and retry.';

  @override String get connectivityTitle => 'Connectivity';
  @override String get connectivitySubtitle => 'Auto-runs after switching nodes';
  @override String get btnRecheck => 'Recheck';
  @override String lastChecked(String time) => 'Last checked: $time';
  @override String get hintClickRecheck => 'Tap "Recheck" to start connectivity check';

  @override String get sectionExitIp => 'Exit IP';
  @override String get categoryDirect => 'Direct';
  @override String get directSubtitle => 'Direct path, exits via router';
  @override String get categoryVpnExit => 'Passage Exit';
  @override String get vpnExitSubtitle => 'Via ClashForge passage';
  @override String get sectionAccessCheck => 'Access Check';
  @override String get categoryDirectPath => 'Direct Path';
  @override String get categoryVpnProxy => 'Passage Route';
  @override String get categoryAiVpnProxy => 'AI · Passage';
  @override String get ipResolved => 'resolved';
  @override String get ipFailed => 'failed';
  @override String get siteStatusOk => 'OK';
  @override String get siteStatusError => 'FAIL';

  @override String get browserDnsTitle => 'Browser DNS Diagnostics';
  @override String get browserDnsSubtitle => 'Pinpoint "connectivity OK but browser can\'t open domain"';
  @override String get hintClickRecheckBrowserDns => 'Tap "Recheck" to run browser DNS diagnostics';
  @override String get browserDnsPathLabel => 'Browser DNS Path';
  @override String get statusHealthy => 'Healthy';
  @override String get statusRisk => 'Risk';

  @override String get checkPrivateDns => 'System Private DNS';
  @override String get checkMihomoDns => 'Mihomo DNS';
  @override String get checkSystemDns => 'System DNS';
  @override String get checkProxyChain => 'Outbound Chain';

  @override String get privateDnsOff => 'Off';
  @override String privateDnsOnHostname(String specifier) => 'On (strict hostname: $specifier)';
  @override String privateDnsOn(String mode) => 'On ($mode)';
  @override String get mihomoDnsNoRecord => 'No records returned (may prevent browser from resolving domains)';
  @override String get systemDnsNoAddr => 'No addresses from lookup';
  @override String proxyChainSuccess(int ms) => 'Outbound check to gstatic succeeded (${ms}ms)';

  @override String get summaryPrivateDnsOn =>
      'Private DNS is enabled. Browsers may report DNS_PROBE_FINISHED_BAD_CONFIG. '
      'Disable Private DNS in system settings.';
  @override String get summaryMihomoDnsFailed =>
      'Mihomo DNS failed to resolve the target domain. '
      'Check routing nodes, upstream DNS, and anti-poisoning settings.';
  @override String get summarySystemDnsFailed =>
      'System DNS resolution failed. '
      'If the proxy side works, check system DNS / router DNS.';
  @override String get summaryProxyChainFailed =>
      'Outbound route test failed. DNS may be OK but the exit node is unreachable.';
  @override String get summaryAllOk =>
      'Browser DNS path is healthy. If browsers still fail, try switching networks.';
  @override String browserDnsFailed(String error) => 'Diagnostics failed: $error';

  @override String get nodeSwitchTitle => 'Node Switch';
  @override String nodeSwitchSubtitle(String group) => 'Current group: $group';
  @override String get hintNoNodes => 'No nodes — import a subscription in the Subscriptions tab';
  @override String get linkViewAllNodes => 'View all nodes  →';

  @override String get proxiesTitle => 'Routes';
  @override String nodesCount(int count) => '$count nodes';
  @override String get noNodesYet => 'No nodes yet';
  @override String get noNodesHint => 'Add a subscription in the\nSubscriptions tab';

  @override String get subscriptionsTitle => 'Subscriptions';
  @override String get subscriptionUrlLabel => 'SUBSCRIPTION URL';
  @override String get btnFetching => 'Fetching…';
  @override String get btnImport => 'Import';
  @override String importedNodes(int count, String name) => 'Imported $count nodes as "$name"';
  @override String fetchFailed(int code) => 'Fetch failed: HTTP $code';
  @override String get pasteNodesLabel => 'PASTE NODE TEXT';
  @override String get pasteNodesDesc =>
      'Supports ss:// vmess:// trojan:// vless:// links or Clash YAML. '
      'Auto-applies Loyalsoldier rules to generate a full config.';
  @override String get btnParsing => 'Parsing…';
  @override String get btnImportAndGenerate => 'Import & Generate Config';
  @override String get noValidNodes =>
      'No valid nodes found. Check format '
      '(supports ss:// vmess:// trojan:// vless:// and Clash YAML)';
  @override String importedFromPaste(int count) =>
      'Imported $count nodes. Config generated with Loyalsoldier rules. Passage ready to activate.';
  @override String parseFailedMsg(Object error) => 'Parse error: $error';
  @override String get savedSubscriptions => 'SAVED SUBSCRIPTIONS';
  @override String get activeLabel => 'Active';
  @override String get btnSwitch => 'Switch';
  @override String nodesCountSub(int count) => '$count nodes';
  @override String get subNameDialogTitle => 'Name this subscription';
  @override String get subNameHint => 'e.g. Work Profile';
  @override String get btnCancel => 'Cancel';
  @override String get btnSave => 'Save';
  @override String get deleteSubTitle => 'Delete Subscription';
  @override String deleteSubContent(String name) => 'Delete "$name"?\nThis cannot be undone.';
  @override String get btnDelete => 'Delete';

  @override String get settingsTitle => 'Settings';
  @override String get tileLogsTitle => 'Logs';
  @override String get tileLogsSubtitle => 'Runtime events, passage & core output';
  @override String get tileUpdatesTitle => 'Check for Updates';
  @override String get tileUpdatesSubtitle => 'See if a newer version is available';
  @override String get tileAboutTitle => 'About';
  @override String get tileAboutSubtitle => 'App version, runtime status, memory';
  @override String get tileLanguageTitle => 'Language';
  @override String get tileLanguageSubtitle => 'App display language';

  @override String get logsTitle => 'Logs';
  @override String get tooltipCopyAll => 'Copy all';
  @override String get tooltipAutoScrollOn => 'Auto-scroll on';
  @override String get tooltipAutoScrollOff => 'Auto-scroll off';
  @override String get tooltipClear => 'Clear';
  @override String get logsCopied => 'Logs copied to clipboard';
  @override String get noLogsYet => 'No logs yet.';
  @override String noFilterLogs(String filter) => 'No $filter logs.';

  @override String get aboutTitle => 'About';
  @override String get sectionApplication => 'APPLICATION';
  @override String get sectionUpdate => 'UPDATE';
  @override String get sectionRuntime => 'RUNTIME';
  @override String get sectionMemory => 'MEMORY';
  @override String get rowVersion => 'Version';
  @override String get rowNodesLoaded => 'Nodes loaded';
  @override String get rowDeviceAbi => 'Device ABI';
  @override String get rowVpn => 'Passage';
  @override String get rowMihomo => 'Mihomo';
  @override String get rowAppPss => 'App (PSS)';
  @override String get rowAvailable => 'Available';
  @override String get rowRunning => 'Running';
  @override String get rowStopped => 'Stopped';
  @override String rowMihomoRunning(int pid) => 'Running (PID $pid)';
  @override String get checkingUpdates => 'Checking for updates…';
  @override String get updateStatus => 'Status';
  @override String get updateCouldNotCheck => 'Could not check';
  @override String get updateUpToDate => 'Up to date  ✓';
  @override String get rowLatest => 'Latest';
  @override String downloadVersion(String tag) => 'Download $tag';
  @override String get btnRecheck2 => 'Re-check';
  @override String get pullDownToRefresh => 'Pull down to refresh';

  @override String get updateSheetTitle => 'Check for Updates';
  @override String get updateChecking => 'Checking…';
  @override String get updateCouldNotCheckLong =>
      'Could not check for updates.\nVerify internet connection.';
  @override String get updateUpToDateTitle => 'You are up to date';
  @override String updateVersionLabel(String tag) => 'Version: $tag';
  @override String get updateAvailableTitle => 'Update available';
  @override String downloadBtn(String tag) => 'Download $tag';
  @override String get allReleases => 'All releases →';

  @override String get siteTaobaoName => 'Taobao';
  @override String get siteTaobaoDesc => 'Verify direct connectivity to domestic e-commerce';
  @override String get siteNeteaseName => 'NetEase Music';
  @override String get siteNeteaseDesc => 'Verify latency to domestic content sites';
  @override String get siteGitHubDesc => 'Verify routed access to international dev sites';
  @override String get siteGoogleDesc => 'Verify Google search is reachable via passage';
  @override String get siteOpenAIDesc => 'Verify ChatGPT is reachable via passage';
  @override String get siteClaudeDesc => 'Verify Claude AI is reachable via passage';

  @override String get langEnglish => 'English';
  @override String get langChinese => '中文';
}

// ─── Chinese ──────────────────────────────────────────────────────────────────

class _AppLocalizationsZh extends AppLocalizations {
  @override String get navHome => '主页';
  @override String get navProxies => '线路';
  @override String get navSubscriptions => '订阅';
  @override String get navSettings => '设置';

  @override String get statusActive => '已连接';
  @override String get statusIdle => '未连接';

  @override String get vpnRunning => '畅行已开启';
  @override String get vpnIdle => '畅行未开启';
  @override String get btnConnect => '连接';
  @override String get btnDisconnect => '断开';
  @override String get btnMoreNodes => '更多节点';

  @override String get connTapToConnect => '轻触以连接';
  @override String get connGrantPermission => '授予网络权限后再次轻触';
  @override String get connConnected => '已连接';
  @override String get connNodeSelectedTapConnect => '节点已选定，轻触连接';
  @override String get connNodeSwitchPending => '节点切换待生效，等待核心就绪后重试';
  @override String connSwitchedTo(String nodeName) => '已切换至 $nodeName';
  @override String connError(String error) => '错误：$error';

  @override String privateDnsWarningHostname(String specifier) =>
      '检测到系统 Private DNS 严格主机名模式（$specifier），Android 可能无法接管 DNS 请求，'
      '浏览器可能报 DNS_PROBE_FINISHED_BAD_CONFIG。请先在系统设置中关闭 Private DNS 后重试。';
  @override String get privateDnsWarningAuto =>
      '检测到系统 Private DNS 自动模式，Android 可能无法接管 DNS 请求，'
      '浏览器可能报 DNS_PROBE_FINISHED_BAD_CONFIG。请先在系统设置中关闭 Private DNS 后重试。';

  @override String get connectivityTitle => '连通性检测';
  @override String get connectivitySubtitle => '切换节点后自动重新执行连通性检测';
  @override String get btnRecheck => '重测';
  @override String lastChecked(String time) => '最近检测: $time';
  @override String get hintClickRecheck => '点击"重测"开始连通性检查';

  @override String get sectionExitIp => '出口 IP';
  @override String get categoryDirect => '直连';
  @override String get directSubtitle => '直连路径，经路由器直出';
  @override String get categoryVpnExit => '畅行出口';
  @override String get vpnExitSubtitle => '经 ClashForge 畅行';
  @override String get sectionAccessCheck => '访问检查';
  @override String get categoryDirectPath => '直连路径';
  @override String get categoryVpnProxy => '畅行线路';
  @override String get categoryAiVpnProxy => 'AI · 畅行线路';
  @override String get ipResolved => '已解析';
  @override String get ipFailed => '未能获取';
  @override String get siteStatusOk => '正常';
  @override String get siteStatusError => '异常';

  @override String get browserDnsTitle => '浏览器 DNS 专项检测';
  @override String get browserDnsSubtitle => '定位"连通性通过但浏览器打不开域名"';
  @override String get hintClickRecheckBrowserDns => '点击"重测"开始浏览器 DNS 专项排查';
  @override String get browserDnsPathLabel => '浏览器 DNS 路径';
  @override String get statusHealthy => '正常';
  @override String get statusRisk => '风险';

  @override String get checkPrivateDns => '系统 Private DNS';
  @override String get checkMihomoDns => 'Mihomo DNS 解析';
  @override String get checkSystemDns => '系统 DNS 解析';
  @override String get checkProxyChain => '出口链路';

  @override String get privateDnsOff => '关闭';
  @override String privateDnsOnHostname(String specifier) => '开启（严格主机名：$specifier）';
  @override String privateDnsOn(String mode) => '开启（$mode）';
  @override String get mihomoDnsNoRecord => '无返回记录（可能导致浏览器域名无法打开）';
  @override String get systemDnsNoAddr => 'lookup 无可用地址';
  @override String proxyChainSuccess(int ms) => '经出口访问 gstatic 成功（${ms}ms）';

  @override String get summaryPrivateDnsOn =>
      '检测到 Private DNS 已开启。该状态下浏览器可能出现 DNS_PROBE_FINISHED_BAD_CONFIG，'
      '建议先关闭系统 Private DNS 再重试。';
  @override String get summaryMihomoDnsFailed =>
      'Mihomo DNS 当前未能解析目标域名，建议检查节点、上游 DNS 和 DNS 防污染设置。';
  @override String get summarySystemDnsFailed =>
      '系统 DNS 解析异常。若连通性页面"代理侧"正常，优先排查系统 DNS / 路由器 DNS。';
  @override String get summaryProxyChainFailed =>
      '出口链路探测失败。DNS 可能正常，但出站线路不可达。';
  @override String get summaryAllOk =>
      '浏览器 DNS 链路整体正常。若浏览器仍报错，请切换网络后再测一次。';
  @override String browserDnsFailed(String error) => '专项检测失败: $error';

  @override String get nodeSwitchTitle => '节点切换';
  @override String nodeSwitchSubtitle(String group) => '当前组：$group';
  @override String get hintNoNodes => '暂无节点，请先在"订阅"页面导入订阅';
  @override String get linkViewAllNodes => '查看完整节点列表  →';

  @override String get proxiesTitle => '线路';
  @override String nodesCount(int count) => '$count 个节点';
  @override String get noNodesYet => '暂无节点';
  @override String get noNodesHint => '请在"订阅"页面\n添加订阅';

  @override String get subscriptionsTitle => '订阅';
  @override String get subscriptionUrlLabel => '订阅链接';
  @override String get btnFetching => '获取中…';
  @override String get btnImport => '导入';
  @override String importedNodes(int count, String name) => '已导入 $count 个节点，订阅名称"$name"';
  @override String fetchFailed(int code) => '获取失败：HTTP $code';
  @override String get pasteNodesLabel => '粘贴节点文本';
  @override String get pasteNodesDesc =>
      '支持 ss:// vmess:// trojan:// vless:// 链接或 Clash YAML，'
      '自动套用 Loyalsoldier 规则生成完整配置';
  @override String get btnParsing => '解析中…';
  @override String get btnImportAndGenerate => '导入并生成配置';
  @override String get noValidNodes =>
      '未识别到有效节点，请检查格式（支持 ss:// vmess:// trojan:// vless:// 及 Clash YAML）';
  @override String importedFromPaste(int count) =>
      '已导入 $count 个节点，配置已生成（使用 Loyalsoldier 规则），可直接开启畅行';
  @override String parseFailedMsg(Object error) => '解析失败: $error';
  @override String get savedSubscriptions => '已保存的订阅';
  @override String get activeLabel => '使用中';
  @override String get btnSwitch => '切换';
  @override String nodesCountSub(int count) => '$count 个节点';
  @override String get subNameDialogTitle => '为订阅命名';
  @override String get subNameHint => '例：工作线路';
  @override String get btnCancel => '取消';
  @override String get btnSave => '保存';
  @override String get deleteSubTitle => '删除订阅';
  @override String deleteSubContent(String name) => '确认删除 "$name"？\n该操作不可撤销。';
  @override String get btnDelete => '删除';

  @override String get settingsTitle => '设置';
  @override String get tileLogsTitle => '日志';
  @override String get tileLogsSubtitle => '运行时事件、服务及核心输出';
  @override String get tileUpdatesTitle => '检查更新';
  @override String get tileUpdatesSubtitle => '查看是否有新版本可用';
  @override String get tileAboutTitle => '关于';
  @override String get tileAboutSubtitle => '应用版本、运行状态、内存用量';
  @override String get tileLanguageTitle => '语言';
  @override String get tileLanguageSubtitle => '应用显示语言';

  @override String get logsTitle => '日志';
  @override String get tooltipCopyAll => '全部复制';
  @override String get tooltipAutoScrollOn => '自动滚动：开';
  @override String get tooltipAutoScrollOff => '自动滚动：关';
  @override String get tooltipClear => '清空';
  @override String get logsCopied => '日志已复制到剪贴板';
  @override String get noLogsYet => '暂无日志。';
  @override String noFilterLogs(String filter) => '无 $filter 级别日志。';

  @override String get aboutTitle => '关于';
  @override String get sectionApplication => '应用信息';
  @override String get sectionUpdate => '更新';
  @override String get sectionRuntime => '运行状态';
  @override String get sectionMemory => '内存';
  @override String get rowVersion => '版本';
  @override String get rowNodesLoaded => '已加载节点';
  @override String get rowDeviceAbi => '设备 ABI';
  @override String get rowVpn => '畅行';
  @override String get rowMihomo => 'Mihomo';
  @override String get rowAppPss => '应用 (PSS)';
  @override String get rowAvailable => '可用内存';
  @override String get rowRunning => '运行中';
  @override String get rowStopped => '已停止';
  @override String rowMihomoRunning(int pid) => '运行中 (PID $pid)';
  @override String get checkingUpdates => '正在检查更新…';
  @override String get updateStatus => '状态';
  @override String get updateCouldNotCheck => '检查失败';
  @override String get updateUpToDate => '已是最新  ✓';
  @override String get rowLatest => '最新版本';
  @override String downloadVersion(String tag) => '下载 $tag';
  @override String get btnRecheck2 => '重新检查';
  @override String get pullDownToRefresh => '下拉刷新';

  @override String get updateSheetTitle => '检查更新';
  @override String get updateChecking => '检查中…';
  @override String get updateCouldNotCheckLong => '无法检查更新。\n请确认网络连接。';
  @override String get updateUpToDateTitle => '已是最新版本';
  @override String updateVersionLabel(String tag) => '版本：$tag';
  @override String get updateAvailableTitle => '发现新版本';
  @override String downloadBtn(String tag) => '下载 $tag';
  @override String get allReleases => '全部版本 →';

  @override String get siteTaobaoName => '淘宝';
  @override String get siteTaobaoDesc => '验证国内主要电商平台直连可达性';
  @override String get siteNeteaseName => '网易云音乐';
  @override String get siteNeteaseDesc => '验证国内常见内容站点延迟';
  @override String get siteGitHubDesc => '验证国际开发站点的访问效果';
  @override String get siteGoogleDesc => '验证 Google 搜索是否可正常访问';
  @override String get siteOpenAIDesc => '验证 ChatGPT 是否可正常访问';
  @override String get siteClaudeDesc => '验证 Claude AI 是否可正常访问';

  @override String get langEnglish => 'English';
  @override String get langChinese => '中文';
}
