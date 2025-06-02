package main

import (
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/isletnet/service"
	"github.com/isletnet/uptp/logging"
)

func serviceControl(ctrlComm string, exeAbsPath string, args []string) error {
	svcConfig := &service.Config{
		Name:        ProductName,
		DisplayName: ProductName,
		Description: ProductName,
		Executable:  exeAbsPath,
		Arguments:   args,
	}

	s, e := service.New(nil, svcConfig)
	if e != nil {
		return e
	}
	e = service.Control(s, ctrlComm)
	if e != nil {
		return e
	}
	return nil
}

func daemonStart() error {
	binPath, _ := os.Executable()
	svcConfig := &service.Config{
		Name:        ProductName,
		DisplayName: ProductName,
		Description: ProductName,
		Executable:  binPath,
	}
	var args []string
	for i, arg := range os.Args {
		if arg == "-d" {
			args = append(os.Args[0:i], os.Args[i+1:]...)
			break
		}
	}
	logging.Debug("worker start params %v", args)
	d := &daemon{
		binPath: binPath,
		args:    args,
	}

	s, e := service.New(d, svcConfig)
	if e != nil {
		return e
	}

	return s.Run()
}

type daemon struct {
	binPath string
	args    []string
	proc    *os.Process
	stopped bool
	mtx     sync.Mutex
}

func (d *daemon) Start(s service.Service) error {
	logging.Info("service start")
	go d.run()
	return nil
}
func (d *daemon) Stop(s service.Service) error {
	logging.Info("service stop")
	d.stopped = true

	d.mtx.Lock()
	if d.proc != nil {
		logging.Info("kill worker")
		d.proc.Kill()
	}
	d.mtx.Unlock()
	logging.Info("worker stopped")

	if service.Interactive() {
		logging.Info("stop daemon")
		os.Exit(0)
	}
	return nil
}

func (d *daemon) run() {
	for {
		if d.stopped {
			return
		}
		logging.Info("start worker")
		err := d.startWorker()
		if err != nil {
			logging.Error("start worker error: %s", err)
			return
		}
		logging.Info("worker stopped")
		time.Sleep(time.Second)
	}
}

func (d *daemon) startWorker() error {
	crashLog := filepath.Join("log", "stderr.log")
	crashLogBak := filepath.Join("log", "stderr.log.0")
	s, _ := os.Stat(crashLog)
	if s != nil && s.Size() > 0 {
		os.Remove(crashLogBak)
		os.Rename(crashLog, crashLogBak)
	}
	f, err := os.Create(filepath.Join(crashLog))
	if err != nil {
		return err
	}
	execSpec := &os.ProcAttr{
		Env:   append(os.Environ(), "GOTRACEBACK=crash"),
		Files: []*os.File{os.Stdin, os.Stdout, f},
	}

	d.mtx.Lock()
	p, err := os.StartProcess(d.binPath, d.args, execSpec)
	if err != nil {
		d.mtx.Unlock()
		return err
	}
	d.proc = p
	d.mtx.Unlock()
	d.proc.Wait()

	d.mtx.Lock()
	d.proc = nil
	d.mtx.Unlock()

	f.Close()
	return nil
}
