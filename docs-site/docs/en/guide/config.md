# First Configuration

ClashForge generates the final mihomo runtime config from multiple layers: imported YAML, subscriptions, managed nodes, DNS/network settings, per-device rules and optional YAML overrides.

## Recommended Order

1. Import one source that is likely to work.
2. Start the mihomo core without rushing into full LAN takeover.
3. Verify node latency and egress IP.
4. Configure per-device routing if needed.
5. Enable transparent proxying and DNS takeover gradually.

## Configuration Sources

| Source | Use case | How to use |
| --- | --- | --- |
| Clash YAML | Migrate an existing client config | Upload or paste in Setup/Configuration |
| Subscription URL | Long-term node maintenance | Add URL, name, User-Agent and update interval |
| Cloudflare Worker node | Low-cost lightweight egress | Deploy from Nodes, then export or publish |
| VPS/SSH node | Stable dedicated egress | Deploy GOST through SSH workflow |
| YAML Overrides | Advanced mihomo fields | Edit only when you understand the generated config |

## Airport Subscriptions

**What it does**
Fetch and parse Clash/SS/Trojan/VLESS/VMess subscriptions.

**Problem solved**
You do not need to maintain subscription URLs on every device.

**How to use**
Open Configuration, add the subscription URL, set a name and optional User-Agent, save and update.

**Value**
Node count, update time and source status become visible and centrally managed.

## Existing Clash YAML

**What it does**
Imports a known-good Clash/mihomo YAML file.

**Problem solved**
Migration is easier when you can start from a config that already works elsewhere.

**How to use**
Upload or paste YAML, save it, preview runtime config and start the core.

**Value**
You can validate the router environment before introducing advanced routing.

## Worker Nodes

**What it does**
Deploys VLESS-over-WebSocket Cloudflare Worker nodes and exports Clash-compatible config.

**Problem solved**
Provides a low-cost backup or lightweight work exit without owning a VPS.

**How to use**
Open Nodes, provide Cloudflare token/account/zone details, deploy and export.

**Value**
Worker nodes can be routed, exported or included in published subscriptions.

## VPS/SSH Nodes

**What it does**
Connects to your VPS over SSH, deploys GOST, binds Cloudflare DNS, issues TLS certificates and probes availability.

**Problem solved**
Critical workflows often need a stable dedicated egress.

**How to use**
Open Nodes, add a VPS/SSH node, authorize the ClashForge public key, verify SSH and run full deployment.

**Value**
Business devices can use a predictable exit while entertainment traffic stays elsewhere.

## Per-device Routing

**What it does**
Matches source IP/CIDR groups and injects managed rules into the generated config.

**Problem solved**
Different devices need different exits.

**How to use**
Bind DHCP leases, create device groups, add IPs/CIDRs, choose policy overrides and restart the config.

**Value**
Users do not need to understand proxy clients; routing is enforced at the router.

## DNS and Transparent Proxy

Start conservative:

| Setting | Initial recommendation |
| --- | --- |
| Transparent mode | Keep disabled until nodes are verified |
| Firewall backend | Use the UI default or auto mode |
| DNS takeover | Enable after core and proxy routing work |
| LAN bypass | Keep enabled |
| Apply on start | Enable only after stable validation |

## GeoData and Rule Sets

Use GeoData to update `GeoIP.dat` and `GeoSite.dat`. Use Configuration rule-set views to inspect and sync rule-providers. Search domain/IP matches when routing behavior is unexpected.

## Overrides

YAML Overrides have high priority and can break startup if invalid. Change one thing at a time, preview runtime config and verify one device after each change.
