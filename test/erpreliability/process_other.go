//go:build !windows

package main

import (
	"net"
	"os/exec"
)

func hideProcess(cmd *exec.Cmd)           {}
func startMonitor(c *privateChild) func() { panic(fail("Windows native monitor required")) }
func validateLocalRedis(addr string, db int) {
	h, _, e := net.SplitHostPort(addr)
	must(e)
	if db != 13 || (h != "127.0.0.1" && h != "localhost" && h != "::1") {
		panic(fail("local Redis DB13 required"))
	}
}
