package logger

import (
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/lesismal/nbio/logging"
)

// logger implements Logger and is used in arpc by default.
type logger struct {
	mtx        sync.RWMutex
	lg         *log.Logger
	logFile    *os.File
	lineEnding string
	level      int
	maxLogSize int64
	pid        int
	logDir     string
	mode       int
}

const (
	LogFileAndConsole = iota
	LogFile
	LogConsole
)

func NewLogger(path string, filePrefix string, level int, maxLogSize int64, mode int) *logger {
	var logdir string
	var lg *log.Logger
	var lf *os.File
	if level != LogConsole {
		if path == "" {
			logdir = "log/"
		} else {
			logdir = path + "/log/"
		}
		os.MkdirAll(logdir, 0744)
		logFilePath := logdir + filePrefix + ".log"
		f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			log.Fatal(err)
		}
		os.Chmod(logFilePath, 0644)
		lg = log.New(f, "", log.LstdFlags)
		lf = f
	}

	var le string
	if runtime.GOOS == "windows" {
		le = "\r\n"
	} else {
		le = "\n"
	}

	pLog := &logger{
		lg:         lg,
		logFile:    lf,
		lineEnding: le,
		level:      level,
		maxLogSize: maxLogSize,
		pid:        os.Getpid(),
		logDir:     logdir,
		mode:       mode,
	}
	if mode != LogConsole {
		go pLog.checkFileLoop()
	}
	return pLog
}

// SetLevel sets logs priority.
func (l *logger) SetLevel(lvl int) {
	l.mtx.Lock()
	defer l.mtx.Unlock()
	l.level = lvl
}

// Debug uses fmt.Printf to log a message at LevelDebug.
func (l *logger) Debug(format string, v ...interface{}) {
	l.mtx.RLock()
	if logging.LevelDebug >= l.level {
		pid := []interface{}{l.pid}
		params := append(pid, v...)
		if l.mode == LogFile || l.mode == LogFileAndConsole {
			l.lg.Printf("%d [DBG] "+format+l.lineEnding, params...)
		}
		if l.mode == LogConsole || l.mode == LogFileAndConsole {
			log.Printf("%d [DBG] "+format+l.lineEnding, params...)
		}
	}
	l.mtx.RUnlock()
}

// Info uses fmt.Printf to log a message at LevelInfo.
func (l *logger) Info(format string, v ...interface{}) {
	l.mtx.RLock()
	if logging.LevelInfo >= l.level {
		pid := []interface{}{l.pid}
		params := append(pid, v...)
		if l.mode == LogFile || l.mode == LogFileAndConsole {
			l.lg.Printf("%d [INF] "+format+l.lineEnding, params...)
		}
		if l.mode == LogConsole || l.mode == LogFileAndConsole {
			log.Printf("%d [INF] "+format+l.lineEnding, params...)
		}
	}
	l.mtx.RUnlock()
}

// Warn uses fmt.Printf to log a message at LevelWarn.
func (l *logger) Warn(format string, v ...interface{}) {
	l.mtx.RLock()
	if logging.LevelWarn >= l.level {
		pid := []interface{}{l.pid}
		params := append(pid, v...)
		if l.mode == LogFile || l.mode == LogFileAndConsole {
			l.lg.Printf("%d [WRN] "+format+l.lineEnding, params...)
		}
		if l.mode == LogConsole || l.mode == LogFileAndConsole {
			log.Printf("%d [WRN] "+format+l.lineEnding, params...)
		}
	}
	l.mtx.RUnlock()
}

// Error uses fmt.Printf to log a message at LevelError.
func (l *logger) Error(format string, v ...interface{}) {
	l.mtx.RLock()
	if logging.LevelError >= l.level {
		pid := []interface{}{l.pid}
		params := append(pid, v...)
		if l.mode == LogFile || l.mode == LogFileAndConsole {
			l.lg.Printf("%d [ERR] "+format+l.lineEnding, params...)
		}
		if l.mode == LogConsole || l.mode == LogFileAndConsole {
			log.Printf("%d [ERR] "+format+l.lineEnding, params...)
		}
	}
	l.mtx.RUnlock()
}

func (l *logger) checkFileLoop() {
	if l.maxLogSize <= 0 {
		return
	}
	ticker := time.NewTicker(time.Minute)
	for {
		<-ticker.C
		l.checkFile()
	}
}

func (l *logger) checkFile() {
	l.mtx.Lock()
	defer l.mtx.Unlock()

	f, e := l.logFile.Stat()
	if e != nil {
		return
	}
	if f.Size() <= l.maxLogSize {
		return
	}
	l.logFile.Close()
	fname := f.Name()
	backupPath := l.logDir + fname + ".0"
	os.Remove(backupPath)
	os.Rename(l.logDir+fname, backupPath)
	newFile, e := os.OpenFile(l.logDir+fname, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if e == nil {
		l.lg.SetOutput(newFile)
		l.logFile = newFile
	}
}
