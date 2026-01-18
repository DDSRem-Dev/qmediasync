//go:build windows
// +build windows

package database

import "syscall"

func setHideWindow(attr *syscall.SysProcAttr) {
	attr.HideWindow = true
}

func getSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow: true,
	}
}
