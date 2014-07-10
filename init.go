package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"syscall"
)

var (
	cmdline []string
)

func init() {
	var err error
	err = os.Mkdir("/proc", 0755)
	if err != nil {
		fmt.Printf("failed to create /proc: %s\n", err.Error())
	}
	err = syscall.Mount("proc", "/proc", "proc", 0, "")
	if err != nil {
		fmt.Printf("failed to mount /proc: %s\n", err.Error())
		os.Exit(1)
	}

	buf, err := ioutil.ReadFile("/proc/cmdline")
	if err != nil {
		fmt.Printf("failed to read /proc/cmdline: %s\n", err)
		os.Exit(1)
	}
	cmdline = strings.Split(strings.TrimSpace(string(buf)), " ")

	err = os.Mkdir("/sys", 0755)
	if err != nil {
		fmt.Printf("failed to create /sys: %s\n", err.Error())
	}
	err = syscall.Mount("sys", "/sys", "sysfs", 0, "")
	if err != nil {
		fmt.Printf("failed to mount /sys: %s\n", err.Error())
		os.Exit(1)
	}
	if _, err = os.Stat("/dev"); err != nil {
		err = os.Mkdir("/dev", 0755)
		if err != nil {
			fmt.Printf("failed to create /dev: %s\n", err.Error())
			os.Exit(1)
		}
	}
	err = syscall.Mount("devtmpfs", "/dev", "devtmpfs", 0, "mode=0755")
	if err != nil {
		fmt.Printf("failed to mount /dev: %s\n", err.Error())
		os.Exit(1)
	}
	err = os.Mkdir("/dev/pts", 0755)
	if err != nil {
		fmt.Printf("failed to create /dev/pts: %s\n", err.Error())
	}
	err = syscall.Mount("devpts", "/dev/pts", "devpts", 0, "gid=5,mode=620")
	if err != nil {
		fmt.Printf("failed to mount /dev/pts: %s\n", err.Error())
		os.Exit(1)
	}
	err = os.Mkdir("/mnt", 0755)
	if err != nil {
		fmt.Printf("failed to create /mnt: %s\n", err.Error())
	}
}
