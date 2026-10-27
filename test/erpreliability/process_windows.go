//go:build windows

package main

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"
)

func hideProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
}
func validateLocalRedis(addr string, db int) {
	h, _, e := net.SplitHostPort(addr)
	must(e)
	if db != 13 || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		panic(fail("local Redis DB13 required"))
	}
}
func startMonitor(c *privateChild) func() {
	root := filepath.Join(relRoot, "native")
	must(os.Mkdir(root, 0700))
	absolute, e := filepath.Abs(root)
	must(e)
	must(save(relRoot+"/monitor-location.json", map[string]string{"path": absolute}))
	exe, e := os.Executable()
	must(e)
	script, e := filepath.Abs("scripts/erp-reliability-monitor.ps1")
	must(e)
	cmd := exec.Command("powershell.exe", "-NoProfile", "-File", script, "-Evidence", absolute, "-ChildID", fmt.Sprint(c.info.PID), "-RunnerID", fmt.Sprint(os.Getpid()), "-Binary", exe)
	hideProcess(cmd)
	stderr, e := os.Create(root + "/stderr.log")
	must(e)
	cmd.Stderr = stderr
	must(cmd.Start())
	stopped := false
	return func() {
		if stopped {
			return
		}
		stopped = true
		_ = save(root+"/stop-monitor.json", map[string]bool{"stop": true})
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()
		select {
		case e := <-done:
			must(e)
		case <-time.After(5 * time.Second):
			_ = cmd.Process.Kill()
			<-done
			panic(fail("owned monitor failed to stop"))
		}
		must(stderr.Close())
	}
}
