package com.clashforge.mobile.clashforge_mobile

/** Android adapters return false only for an uninstalled package; other failures propagate. */
internal object VpnAppRouting {
    fun apply(
        mode: String,
        packages: Set<String>,
        self: String,
        allow: (String) -> Boolean,
        disallow: (String) -> Boolean,
    ): Int {
        val selected = packages.filter { it.isNotBlank() && it != self }.sorted()
        if (mode == "allowlist") {
            val applied = selected.count { allow(it) }
            check(applied > 0) { "VPN allowlist has no installed applications" }
            return applied
        }
        check(disallow(self)) { "Cannot exclude the VPN application" }
        return if (mode == "bypass") selected.count { disallow(it) } else 0
    }
}
