package com.clashforge.mobile.clashforge_mobile

import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.VpnService
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import androidx.annotation.RequiresApi

@RequiresApi(Build.VERSION_CODES.N)
class ClashForgeTileService : TileService() {

    private val stateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context, intent: Intent) {
            if (intent.action == ClashVpnService.ACTION_VPN_STATE_CHANGED) {
                updateTile()
            }
        }
    }

    override fun onStartListening() {
        super.onStartListening()
        val filter = IntentFilter(ClashVpnService.ACTION_VPN_STATE_CHANGED)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            registerReceiver(stateReceiver, filter, RECEIVER_NOT_EXPORTED)
        } else {
            @Suppress("UnspecifiedRegisterReceiverFlag")
            registerReceiver(stateReceiver, filter)
        }
        updateTile()
    }

    override fun onStopListening() {
        super.onStopListening()
        try { unregisterReceiver(stateReceiver) } catch (_: Exception) {}
    }

    override fun onClick() {
        super.onClick()
        if (ClashVpnService.vpnRunning) {
            startService(Intent(this, ClashVpnService::class.java).apply {
                action = ClashVpnService.ACTION_STOP
            })
        } else {
            // If VPN permission not yet granted, collapse tile and open the app.
            // The app's connect flow calls VpnService.prepare() and shows the dialog.
            val vpnPrepareIntent = VpnService.prepare(this)
            if (vpnPrepareIntent == null) {
                // Permission already granted — start directly.
                startService(Intent(this, ClashVpnService::class.java).apply {
                    action = ClashVpnService.ACTION_START
                })
            } else {
                // Need user confirmation — launch app to handle prepare() dialog.
                val launchIntent = Intent(this, MainActivity::class.java).apply {
                    flags = Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_CLEAR_TOP
                }
                if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
                    startActivityAndCollapse(
                        PendingIntent.getActivity(
                            this, 0, launchIntent,
                            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE
                        )
                    )
                } else {
                    @Suppress("DEPRECATION")
                    startActivityAndCollapse(launchIntent)
                }
            }
        }
        updateTile()
    }

    private fun updateTile() {
        val tile = qsTile ?: return
        val running = ClashVpnService.vpnRunning
        tile.state = if (running) Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
        tile.label = "ClashForge"
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            tile.subtitle = if (running) "已连接" else "未连接"
        }
        tile.updateTile()
    }
}
