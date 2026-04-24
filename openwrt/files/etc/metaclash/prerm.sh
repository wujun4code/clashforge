#!/bin/sh

# 0. Backup user config so postinst can restore it after opkg removes the directory
cp /etc/metaclash/config.toml /tmp/clashforge-config.bak 2>/dev/null || true

# 1. Disable auto-respawn so procd won't restart after we kill
/etc/init.d/clashforge disable 2>/dev/null || true

# 2. Graceful stop via procd
/etc/init.d/clashforge stop 2>/dev/null || true

# 3. Kill by pidfile (covers procd-managed process)
PIDFILE=/var/run/metaclash/metaclash.pid
if [ -f "$PIDFILE" ]; then
  PID=$(cat "$PIDFILE" 2>/dev/null)
  [ -n "$PID" ] && kill -9 "$PID" 2>/dev/null || true
fi

# 4. Kill any remaining clashforge/mihomo processes by scanning /proc
# (covers manually-started processes that procd doesn't know about)
for pid in $(ls /proc 2>/dev/null | grep '^[0-9]'); do
  cmdline=$(cat /proc/$pid/cmdline 2>/dev/null | tr '\0' ' ')
  case "$cmdline" in
    */usr/bin/clashforge*)  kill -9 "$pid" 2>/dev/null || true ;;
    */usr/bin/mihomo-clashforge*) kill -9 "$pid" 2>/dev/null || true ;;
  esac
done

# 5. Remove procd service instance so it won't respawn
ubus call service delete '{"name":"clashforge"}' 2>/dev/null || true

# 6. Clean up pid files
rm -f /var/run/metaclash/metaclash.pid /var/run/metaclash/mihomo.pid
