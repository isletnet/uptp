package com.isletnet.uptpproxy

import android.content.pm.ApplicationInfo
import android.content.pm.PackageManager
import android.view.LayoutInflater
import android.view.View
import android.view.ViewGroup
import android.widget.CheckBox
import android.widget.ImageView
import android.widget.TextView
import androidx.recyclerview.widget.RecyclerView
import java.util.Locale

class AppListAdapter(
    private var apps: List<ApplicationInfo>,
    private val pm: PackageManager
) : RecyclerView.Adapter<AppListAdapter.AppViewHolder>() {

    private val collator = java.text.Collator.getInstance(Locale.CHINA)
    
    private var filteredApps = apps.sortedWith(compareBy(
        { !isChineseApp(it.loadLabel(pm).toString()) }, // 中文应用在前
        { collator.getCollationKey(it.loadLabel(pm).toString()) } // 按拼音排序
    )).toMutableList()

    private fun isChineseApp(name: String): Boolean {
        return name.any {
            Character.UnicodeBlock.of(it) == Character.UnicodeBlock.CJK_UNIFIED_IDEOGRAPHS ||
            Character.UnicodeBlock.of(it) == Character.UnicodeBlock.CJK_COMPATIBILITY_IDEOGRAPHS
        }
    }
    private val selectedApps = mutableSetOf<String>()

    class AppViewHolder(view: View) : RecyclerView.ViewHolder(view) {
        val icon: ImageView = view.findViewById(R.id.appIcon)
        val name: TextView = view.findViewById(R.id.appName)
        val packageName: TextView = view.findViewById(R.id.packageName)
        val checkBox: CheckBox = view.findViewById(R.id.appCheckBox)
    }

    override fun onCreateViewHolder(parent: ViewGroup, viewType: Int): AppViewHolder {
        val view = LayoutInflater.from(parent.context)
            .inflate(R.layout.item_app, parent, false)
        return AppViewHolder(view)
    }

    override fun onBindViewHolder(holder: AppViewHolder, position: Int) {
        val app = filteredApps[position]
        holder.icon.setImageDrawable(app.loadIcon(pm))
        holder.name.text = app.loadLabel(pm)
        holder.packageName.text = app.packageName
        holder.checkBox.isChecked = selectedApps.contains(app.packageName)

        holder.checkBox.setOnClickListener {
            val isChecked = holder.checkBox.isChecked
            if (isChecked) {
                selectedApps.add(app.packageName)
            } else {
                selectedApps.remove(app.packageName)
            }
            it.isClickable = false // 阻止事件冒泡
        }

        holder.itemView.setOnClickListener {
            holder.checkBox.isChecked = !holder.checkBox.isChecked
            if (holder.checkBox.isChecked) {
                selectedApps.add(app.packageName)
            } else {
                selectedApps.remove(app.packageName)
            }
        }
    }

    override fun getItemCount() = filteredApps.size

    fun getSelectedApps(): List<String> = selectedApps.toList()
    
    fun setSelectedApps(apps: Set<String>) {
        selectedApps.clear()
        selectedApps.addAll(apps)
        notifyDataSetChanged()
    }

    val filter = object : android.widget.Filter() {
        override fun performFiltering(constraint: CharSequence?): FilterResults {
            val results = FilterResults()
            val filteredList = mutableListOf<ApplicationInfo>()

            if (constraint.isNullOrEmpty()) {
                filteredList.addAll(apps)
            } else {
                val filterPattern = constraint.toString().lowercase(Locale.getDefault())
                apps.forEach {
                    if (it.loadLabel(pm).toString().lowercase(Locale.getDefault())
                        .contains(filterPattern)) {
                        filteredList.add(it)
                    }
                }
            }

            results.values = filteredList
            results.count = filteredList.size
            return results
        }

        @Suppress("UNCHECKED_CAST")
        override fun publishResults(constraint: CharSequence?, results: FilterResults?) {
            filteredApps.clear()
            results?.values?.let { filteredApps.addAll(it as List<ApplicationInfo>) }
            notifyDataSetChanged()
        }
    }
}