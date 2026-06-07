//go:build !windows
// +build !windows

package main

import "os/exec"

func hideCommandWindow(cmd *exec.Cmd) {
}
