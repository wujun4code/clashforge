#!/bin/sh

# 1. Disable auto-respawn so procd won't restart the service after we kill it
/etc/init.d/clashforge disable 2>/dev/null || true

# 2. Ask procd to stop gracefully
/etc/init.d/clashforge stop 2>/dev/null || true

# 3. Give procd up to 5 s to stop; force-kill what remains
i=0
while [ $i -lt 5 ]; do
  pgrep -f '/usr/bin/clashforge' >/dev/null 2>&1 || break
  sleep 1
  i=$((i+1))
done
kill -9 $(pgrep -f '/usr/bin/clashforge' 2>/dev/null) 2>/dev/null || true
kill -9 $(pgrep -f 'mihomo-clashforge'   2>/dev/null) 2>/dev/null || true

# 4. Remove procd service instance so it won't respawn
ubus call service delete '{"name":"clashforge"}' 2>/dev/null || true

# 5. Clean up pid files
rm -f /var/run/metaclash/metaclash.pid /var/run/metaclash/mihomo.pid
