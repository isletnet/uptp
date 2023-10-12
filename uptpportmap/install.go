package main

import (
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/isletnet/uptp/logging"
)

func install() {
	gLog := NewLogger("", "", logging.LevelDebug, 0, LogConsole)
	err := os.MkdirAll(defaultInstallPath, 0775)

	if err != nil {
		gLog.Error("MkdirAll %s error:%s", defaultInstallPath, err)
		os.Exit(1)
	}
	err = os.Chdir(defaultInstallPath)
	if err != nil {
		gLog.Error("cd error: %s", err)
		os.Exit(1)
	}

	uninstall()
	targetPath := filepath.Join(defaultInstallPath, defaultBinName)
	// copy files

	binPath, _ := os.Executable()
	src, errFiles := os.Open(binPath)
	if errFiles != nil {
		gLog.Error("os.OpenFile %s error:%s", os.Args[0], errFiles)
		os.Exit(1)
	}

	dst, errFiles := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0775)
	if errFiles != nil {
		gLog.Error("os.OpenFile %s error:%s", targetPath, errFiles)
		os.Exit(1)
	}

	_, errFiles = io.Copy(dst, src)
	if errFiles != nil {
		gLog.Error("io.Copy error:%s", errFiles)
		os.Exit(1)
	}
	src.Close()
	dst.Close()

	// install system service
	gLog.Info("targetPath: %s", targetPath)
	err = serviceControl("install", targetPath, []string{"-d"})
	if err != nil {
		gLog.Error("install system service error: ", err)
		os.Exit(1)
	}
	gLog.Info("install system service ok.")
	time.Sleep(time.Second * 2)
	err = serviceControl("start", targetPath, []string{"-d"})
	if err != nil {
		gLog.Error("start %s service error:", ProductName, err)
		os.Exit(1)
	} else {
		gLog.Info("start %s service ok.", ProductName)
	}
}

func uninstall() {
	gLog := NewLogger("", "", logging.LevelDebug, 0, LogConsole)
	defer gLog.Info("uninstall end")
	err := serviceControl("stop", "", nil)
	if err != nil { // service maybe not install
		gLog.Error("stop service fail: %s", err)
	}
	err = serviceControl("uninstall", "", nil)
	if err != nil {
		gLog.Error("uninstall system service error:", err)
		return
	} else {
		gLog.Info("uninstall system service ok.")
	}
	binPath := filepath.Join(defaultInstallPath, defaultBinName)
	os.Remove(binPath + "0")
	os.Remove(binPath)
}
