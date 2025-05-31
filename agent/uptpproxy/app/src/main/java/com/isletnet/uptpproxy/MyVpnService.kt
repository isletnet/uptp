package com.isletnet.uptpproxy

import agent.Agent
import android.R
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.ParcelFileDescriptor
import android.util.Log
import androidx.core.app.NotificationCompat
import java.io.IOException


class MyVpnService : VpnService() {
    companion object {
        const val ACTION_VPN_STATE_CHANGED = "com.isletnet.uptpproxy.VPN_STATE_CHANGED"
        const val EXTRA_IS_RUNNING = "is_running"
        const val PACKAGE_NAME = "com.isletnet.uptpproxy"
        
        var isRunning = false
            private set
    }

    private var vpnInterface: ParcelFileDescriptor? = null

    override fun onCreate() {
        super.onCreate()
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent != null && "DISCONNECT".equals(intent.getAction())) {
            stopTProxy(); // 处理停止请求
            return START_NOT_STICKY;
            }
        startTProxy(intent?.getIntExtra("selected_gateway_index", 0) ?: 0); // 启动 VPN 连接
        return START_STICKY; // 保持服务存活
    }
    
    
    override fun onDestroy() {
        super.onDestroy()
        stopTProxy()
    }

    private fun startTProxy(idx: Int) {
        val builder = Builder().apply {
            setSession("UptpProxy VPN")
            addAddress("10.8.0.2", 24)
            addRoute("0.0.0.0", 0)
            addDnsServer("8.8.8.8")
            addDnsServer("8.8.4.4")
            setMtu(1500)
            setBlocking(true)
            
            // 添加允许的应用
            val prefs = getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE)
            val allowedApps = prefs.getStringSet("allowed_apps", emptySet()) ?: emptySet()
            
            if (allowedApps.isEmpty()) {
                // 如果没有选择应用，默认禁止所有应用
                addDisallowedApplication(PACKAGE_NAME)
            } else {
                // 允许选择的应用(跳过自身应用)
                allowedApps.forEach { packageName ->
                    if (packageName != PACKAGE_NAME) {
                        try {
                            addAllowedApplication(packageName)
                        } catch (e: Exception) {
                            Log.e("VpnService", "Failed to allow app $packageName", e)
                        }
                    }
                }
            }
        }
        try {
            Log.d("VpnService", "Starting VPN service")
            vpnInterface = builder.establish()
            Log.d("VpnService", "VPN interface established, fd=${vpnInterface?.fd}")

            Agent.startTunProxy("fd://"+vpnInterface?.fd, idx.toLong())
            Log.d("VpnService", "Tun proxy started")
            
            isRunning = true
            Log.d("VpnService", "VPN is now running, sending broadcast")
            
            try {
                sendBroadcast(Intent(ACTION_VPN_STATE_CHANGED).apply {
                    setPackage(packageName)
                    putExtra(EXTRA_IS_RUNNING, true)
                })
                Log.d("VpnService", "VPN started broadcast sent")
            } catch (e: Exception) {
                Log.e("VpnService", "Failed to send broadcast", e)
            }
        } catch (e: Exception) {
            Log.e("VpnService", "Error start tun proxy", e)
            stopSelf()
        }
        startForeground(1, createNotification())
    }

    private fun stopTProxy() {
        Log.d("VpnService", "Stopping VPN service")
        try {
            if (vpnInterface != null) {
                Agent.stopTunProxy()
                Log.d("VpnService", "Tun proxy stopped")
                
                vpnInterface!!.close() // 关闭 VPN 连接
                Log.d("VpnService", "VPN interface closed")
                
                vpnInterface = null
                isRunning = false
                Log.d("VpnService", "VPN is now stopped, sending broadcast")
                
                try {
                    sendBroadcast(Intent(ACTION_VPN_STATE_CHANGED).apply {
                        setPackage(packageName)
                        putExtra(EXTRA_IS_RUNNING, false)
                    })
                    Log.d("VpnService", "VPN stopped broadcast sent")
                } catch (e: Exception) {
                    Log.e("VpnService", "Failed to send broadcast", e)
                }
            }
        } catch (e: IOException) {
            e.printStackTrace()
        }
        stopForeground(true) // 移除前台通知
        stopSelf() // 停止服务
    }
    private fun createNotification(): Notification {
        val channel = NotificationChannel(
            "vpn_channel", "VPN Service", NotificationManager.IMPORTANCE_LOW
        )
        val manager = getSystemService(NotificationManager::class.java)
        manager.createNotificationChannel(channel)

        return NotificationCompat.Builder(this, "vpn_channel")
            .setContentTitle("VPN 运行中")
//            .setSmallIcon(R.drawable.ic_vpn)
            .build()
    }

}