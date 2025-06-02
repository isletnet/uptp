package com.isletnet.uptpproxy

import agent.Agent
import android.Manifest
import android.content.pm.ApplicationInfo
import android.widget.EditText
import androidx.appcompat.app.AlertDialog
import org.json.JSONArray
import androidx.recyclerview.widget.RecyclerView
import androidx.recyclerview.widget.LinearLayoutManager
import android.text.Editable
import android.text.TextWatcher
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.content.pm.PackageManager
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.util.Log
import android.view.Menu
import android.view.MenuItem
import android.view.View
import android.widget.ArrayAdapter
import android.widget.Button
import android.widget.Spinner
import android.widget.TextView
import android.widget.Toast
import androidx.appcompat.app.AppCompatActivity
import androidx.core.content.ContextCompat
import android.widget.AdapterView
import androidx.core.content.edit

class MainActivity : AppCompatActivity() {
    
    override fun onCreateOptionsMenu(menu: Menu): Boolean {
        menuInflater.inflate(R.menu.main_menu, menu)
        return true
    }

    override fun onOptionsItemSelected(item: MenuItem): Boolean {
        val intent = Intent(this, LogViewerActivity::class.java)
        return when (item.itemId) {
            R.id.menu_add_gateway -> {
                showAddGatewayDialog()
                true
            }
            R.id.menu_view_core_logs -> {
                intent.putExtra("log_file", "uptp-agent.log")
                startActivity(intent)
                true
            }
            R.id.menu_view_libp2p_logs -> {
                intent.putExtra("log_file", "libp2p.log")
                startActivity(intent)
                true
            }
            R.id.menu_select_apps -> {
                showSelectAppsDialog()
                true
            }
            else -> super.onOptionsItemSelected(item)
        }
    }

    private fun showSelectAppsDialog() {
        val dialogView = layoutInflater.inflate(R.layout.dialog_select_apps, null)
        val appList = dialogView.findViewById<RecyclerView>(R.id.appList)
        val searchBox = dialogView.findViewById<EditText>(R.id.searchBox)
        val confirmButton = dialogView.findViewById<Button>(R.id.confirmButton)
        val cancelButton = dialogView.findViewById<Button>(R.id.cancelButton)
        
        // 获取所有应用列表
        val pm = packageManager
        val apps = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.R) {
            // Android 11+ 使用QUERY_ALL_PACKAGES权限
            pm.getInstalledApplications(PackageManager.MATCH_ALL)
        } else if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            pm.getInstalledApplications(PackageManager.GET_META_DATA or PackageManager.MATCH_UNINSTALLED_PACKAGES)
        } else {
            pm.getInstalledApplications(PackageManager.GET_META_DATA)
        }.filter {
            // 过滤系统应用和自身
            it.flags and ApplicationInfo.FLAG_SYSTEM == 0 &&
            it.packageName != packageName
        }.sortedBy { it.loadLabel(pm).toString() }
        
        // 创建并设置适配器
        val adapter = AppListAdapter(apps, packageManager)
        
        // 加载已保存的应用
        val prefs = getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE)
        val savedApps = prefs.getStringSet("allowed_apps", emptySet()) ?: emptySet()
        adapter.setSelectedApps(savedApps)
        
        // 设置RecyclerView
        appList.layoutManager = LinearLayoutManager(this)
        appList.adapter = adapter
        
        // 设置搜索功能
        searchBox.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, start: Int, count: Int, after: Int) {}

            override fun onTextChanged(s: CharSequence?, start: Int, before: Int, count: Int) {
                adapter.filter.filter(s?.toString())
            }

            override fun afterTextChanged(s: Editable?) {}
        })
        
        // 创建并显示对话框
        val dialog = AlertDialog.Builder(this, R.style.ThemeOverlay_AppCompat_Dark)
            .setTitle(R.string.select_apps_title)
            .setView(dialogView)
            .create()
            
        confirmButton.setOnClickListener {
            // 保存选中的应用
            val selectedApps = adapter.getSelectedApps()
            Log.d("MainActivity","Saving selected apps: $selectedApps")
            saveSelectedApps(selectedApps)
            dialog.dismiss()
        }
        
        cancelButton.setOnClickListener {
            dialog.dismiss()
        }
        
        dialog.show()
    }
    
    private fun saveSelectedApps(apps: List<String>) {
        val prefs = getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE)
        prefs.edit().putStringSet("allowed_apps", apps.toSet()).apply()
    }

    private fun showAddGatewayDialog() {
        val dialogView = layoutInflater.inflate(R.layout.dialog_add_gateway, null)
        val gatewayInput = dialogView.findViewById<EditText>(R.id.gateway_input)
        val tokenInput = dialogView.findViewById<EditText>(R.id.token_input)
        val dialog = AlertDialog.Builder(this, R.style.ThemeOverlay_AppCompat_Dark)
            .setTitle("添加网关")
            .setView(dialogView)
            .setPositiveButton("确定") { _, _ ->
                val gateway = gatewayInput.text.toString()
                val token = tokenInput.text.toString()
                if (gateway.isNotBlank()) {
                    try {
                        Agent.addProxyGateway(gateway, token)
                    } catch (e: Exception) {
                        showError("添加网关失败: ${e.message}")
                    }
                    startAgent()
                    Toast.makeText(this, "添加网关: $gateway", Toast.LENGTH_SHORT).show()
                }
            }
            .setNegativeButton("取消", null)
            .create()
        dialog.show()
    }
    private lateinit var vpnButton: Button
    private lateinit var statusText: TextView
    private lateinit var gatewaySpinner: Spinner
    private val vpnStateReceiver = object : BroadcastReceiver() {
        override fun onReceive(context: Context?, intent: Intent?) {
            Log.d("MainActivity", "Received VPN state broadcast")
            if (intent?.action == MyVpnService.ACTION_VPN_STATE_CHANGED) {
                val isRunning = intent.getBooleanExtra(MyVpnService.EXTRA_IS_RUNNING, false)
                Log.d("MainActivity", "VPN state changed: isRunning=$isRunning")
                updateButtonText()
            }
        }
    }
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        lastSelectedGatewayPosition = getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE)
            .getInt("last_selected_gateway_position", 0)
        setContentView(R.layout.activity_main)
        
        // 强制设置状态栏为黑色
        window.statusBarColor = resources.getColor(R.color.black)
        
        // 初始化UI组件
        vpnButton = findViewById<Button>(R.id.vpn_button)
        statusText = findViewById<TextView>(R.id.status_text)
        gatewaySpinner = findViewById<Spinner>(R.id.spinner)

        gatewaySpinner.onItemSelectedListener = object : android.widget.AdapterView.OnItemSelectedListener {
            override fun onItemSelected(parent: android.widget.AdapterView<*>, view: View, position: Int, id: Long) {
                lastSelectedGatewayPosition = position
                getSharedPreferences("vpn_prefs", Context.MODE_PRIVATE).edit {
                    putInt("last_selected_gateway_position", position)
                }
            }
            override fun onNothingSelected(parent: android.widget.AdapterView<*>) {}
        }

        updateButtonText()
        
        vpnButton.setOnClickListener {
            if (MyVpnService.isRunning) {
                stopVpnService()
            } else {
                if (gatewaySpinner.adapter == null || gatewaySpinner.adapter.count == 0) {
                    showError("请先获取网关列表")
                } else {
                    startVpnService()
                }
            }
        }
        
        registerReceiver(vpnStateReceiver, IntentFilter(MyVpnService.ACTION_VPN_STATE_CHANGED), Context.RECEIVER_NOT_EXPORTED)
        
        // 检查并请求网络权限
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.M) {
            val requiredPermissions = arrayOf(
                Manifest.permission.ACCESS_NETWORK_STATE,
                Manifest.permission.ACCESS_WIFI_STATE,
                Manifest.permission.CHANGE_NETWORK_STATE
            )
            
            val permissionsToRequest = requiredPermissions.filter {
                checkSelfPermission(it) != PackageManager.PERMISSION_GRANTED
            }.toTypedArray()
            
            if (permissionsToRequest.isNotEmpty()) {
                requestPermissions(permissionsToRequest, 100)
            } else {
                startAgent()
            }
        } else {
            startAgent()
        }
    }
    
    private fun startAgent() {
        try {
            if (!isNetworkAvailable()) {
                Log.e("MainActivity", "No network connection available")
                showError("网络不可用，请检查网络连接")
                return
            }
            Agent.start(filesDir.absolutePath,false)
            Log.d("MainActivity", "Agent started successfully")
        } catch (e: Exception) {
            Log.e("MainActivity", "Error in startAgent", e)
            showError("启动Agent失败: ${e.message}")
        }
        try{
            val gwListJson = Agent.getProxyGatewaysJson()
            Log.d("MainActivity", "Gateway list: $gwListJson")
            val jsonArray = JSONArray(gwListJson)
            val gatewayList = mutableListOf<String>()
            for (i in 0 until jsonArray.length()) {
                gatewayList.add(jsonArray.getString(i))
            }

            runOnUiThread {
                val adapter = ArrayAdapter(
                    this,
                    android.R.layout.simple_spinner_item,
                    gatewayList
                )
                adapter.setDropDownViewResource(android.R.layout.simple_spinner_dropdown_item)
                gatewaySpinner.adapter = adapter
                // 使用已设置的监听器
                if (gatewayList.isNotEmpty()) {
                    if (gatewayList.size > lastSelectedGatewayPosition) {
                        gatewaySpinner.setSelection(lastSelectedGatewayPosition)
                    } else{
                        gatewaySpinner.setSelection(0)
                    }
                }
            }
        }catch (e: Exception){
            Log.d("MainActivity","读取gateway列表失败: ${e.message}")
        }
    }

    private var lastSelectedGatewayPosition = 0
    
    private fun isNetworkAvailable(): Boolean {
        val connectivityManager = getSystemService(ConnectivityManager::class.java)
        val network = connectivityManager.activeNetwork ?: return false
        val capabilities = connectivityManager.getNetworkCapabilities(network) ?: return false
        return capabilities.hasCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
    }
    
    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == 100) {
            if (grantResults.all { it == PackageManager.PERMISSION_GRANTED }) {
                startAgent()
            } else {
                Log.e("MainActivity", "Required permissions not granted")
                showError("缺少必要权限，请授予权限")
            }
        }
    }
    
    private fun startVpnService() {
        statusText.text = ""
        try {
            Agent.testIPv6("http://ipv6.ddnspod.com/")
        }catch (e: Exception){
            showError("ipv6 检测失败: ${e.message}")
            return
        }
        try {
            startAgent()
            Agent.pingProxyGateway(gatewaySpinner.selectedItemPosition.toLong())
            val intent = VpnService.prepare(this)
            if (intent != null) {
                startActivityForResult(intent, 100)
            } else {
                startService(Intent(this, MyVpnService::class.java).apply {
                    putExtra("selected_gateway_index", gatewaySpinner.selectedItemPosition)
                })
            }
        } catch (e: Exception) {
            showError("连接网关失败: ${e.message}")
        }
    }
    
    override fun onActivityResult(request: Int, result: Int, data: Intent?) {
        super.onActivityResult(request, result, data)
        if (request == 100 && result == RESULT_OK) {
            startService(Intent(this, MyVpnService::class.java).apply {
                putExtra("selected_gateway_index", gatewaySpinner.selectedItemPosition)
            })
        }
    }
    
    private fun stopVpnService() {
        statusText.text = ""
        try {
            startService(Intent(this@MainActivity, MyVpnService::class.java).also { it.action = "DISCONNECT" })
        } catch (e: Exception) {
            showError("断开网关失败: ${e.message}")
        }
    }
    
    private fun updateButtonText() {
        if (MyVpnService.isRunning) {
            gatewaySpinner.isEnabled = false
            vpnButton.text = "断开网关"
            vpnButton.setBackgroundColor(ContextCompat.getColor(this, R.color.red_500))
//            statusText.text = "VPN运行中"
//            statusText.setTextColor(ContextCompat.getColor(this, R.color.purple_500))
        } else {
            gatewaySpinner.isEnabled = true
            vpnButton.text = "连接网关"
            vpnButton.setBackgroundColor(ContextCompat.getColor(this, R.color.purple_500))
//            statusText.text = "VPN已停止"
//            statusText.setTextColor(ContextCompat.getColor(this, R.color.red_500))
        }
    }
    
    private fun showError(message: String) {
        runOnUiThread {
            statusText.text = message
            statusText.setTextColor(ContextCompat.getColor(this, R.color.red_500))
            Toast.makeText(this, message, Toast.LENGTH_LONG).show()
        }
    }
    
}