# First Configuration

The configuration goal is to make mihomo start reliably while keeping subscriptions, rules, DNS and transparent proxy takeover controlled.

## Recommended Order

1. Prepare a Clash-compatible YAML file or subscription URL.
2. Import it in the Web UI setup wizard.
3. Save and activate the configuration.
4. Start the mihomo core.
5. Verify node connectivity and egress IP.
6. Enable transparent proxying or DNS takeover only after verification.

## Configuration Sources

| Source | Use case | Recommendation |
| --- | --- | --- |
| Uploaded YAML | Migrate an existing Clash config | Best for first validation |
| Pasted YAML | Manual debugging | Watch indentation and syntax |
| Subscription URL | Long-term node maintenance | Keep at least one backup source |
| YAML Overrides | Override generated config | Keep overrides minimal |

::: warning Protect secrets
Subscription URLs, tokens, SSH key paths and diagnostic reports may contain sensitive data. Redact before sharing.
:::

## Network Settings

| Setting | Initial recommendation | Notes |
| --- | --- | --- |
| Transparent mode | `none` or manual | Avoid taking over the whole LAN too early |
| Firewall backend | `auto` | Let ClashForge choose nftables or iptables |
| Apply on start | Disabled | Enable only after stable validation |
| LAN bypass | Enabled | Keep router management reachable |
| Mainland China IP bypass | As needed | Depends on your rule strategy |

## DNS Settings

Open DNS takeover gradually:

1. Start the mihomo core only.
2. Verify nodes, rules and egress IP.
3. Enable mihomo DNS.
4. Enable dnsmasq cooperation or entry takeover.

## Completion Criteria

1. Web UI opens.
2. At least one node is usable.
3. The mihomo core starts.
4. Status page or API reports healthy state.
5. Browser-side and router-side connectivity probes match expectations.
