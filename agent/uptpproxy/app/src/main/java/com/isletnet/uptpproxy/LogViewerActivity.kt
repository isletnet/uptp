package com.isletnet.uptpproxy

import android.os.Bundle
import android.widget.TextView
import androidx.appcompat.app.AppCompatActivity
import java.io.File
import java.io.RandomAccessFile
import java.util.LinkedList

class LogViewerActivity : AppCompatActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(R.layout.log_viewer)
        
        val logText = findViewById<TextView>(R.id.log_text)
        val logFileName = intent.getStringExtra("log_file") ?: "libp2p.log"
        val logFile = File(filesDir, "log/"+logFileName)
        
        supportActionBar?.title = "查看日志: $logFileName"
        
        if (logFile.exists()) {
            try {
                val last100Lines = getLastLines(logFile, 100)
                logText.text = last100Lines
            } catch (e: Exception) {
                logText.text = "读取日志失败: ${e.message}"
            }
        } else {
            logText.text = "日志文件不存在: $logFileName"
        }
    }

    private fun getLastLines(file: File, lineCount: Int): String {
        val lines = LinkedList<String>()
        RandomAccessFile(file, "r").use { raf ->
            var pos = file.length() - 1
            var newLineCount = 0
            
            while (pos >= 0 && newLineCount < lineCount) {
                raf.seek(pos)
                if (raf.readByte().toInt().toChar() == '\n') {
                    if (pos < file.length() - 1) {
                        lines.addFirst(raf.readLine())
                        newLineCount++
                    }
                }
                pos--
            }
            
            // Add the first line if not empty
            if (newLineCount < lineCount) {
                raf.seek(0)
                lines.addFirst(raf.readLine())
            }
        }
        return lines.joinToString("\n")
    }
}