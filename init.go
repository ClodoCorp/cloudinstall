package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
	"syscall"
)

var (
	cmdline []string
	debug   = false
)

func init() {
	var err error

	if _, err := os.Stat("/proc"); err != nil {
		err = os.Mkdir("/proc", 0755)
		if err != nil {
			fmt.Printf("failed to create /proc: %s\n", err.Error())
		}
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
	debug = cmdlineBool("debug")

	if _, err := os.Stat("/sys"); err != nil {
		err = os.Mkdir("/sys", 0755)
		if err != nil {
			fmt.Printf("failed to create /sys: %s\n", err.Error())
		}
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

	if _, err := os.Stat("/dev/pts"); err != nil {
		err = os.Mkdir("/dev/pts", 0755)
		if err != nil {
			fmt.Printf("failed to create /dev/pts: %s\n", err.Error())
		}
	}
	err = syscall.Mount("devpts", "/dev/pts", "devpts", 0, "gid=5,mode=620")
	if err != nil {
		fmt.Printf("failed to mount /dev/pts: %s\n", err.Error())
		os.Exit(1)
	}

	if _, err := os.Stat("/mnt"); err != nil {
		err = os.Mkdir("/mnt", 0755)
		if err != nil {
			fmt.Printf("failed to create /mnt: %s\n", err.Error())
		}
	}

	/*
		cmd := exec.Command("/bin/busybox", "--help", "|", "/bin/busybox", "grep", "'Currently defined functions:'", "-A300", "|", "/bin/busybox", "grep", "-v", "'Currently defined functions:'", "|", "/bin/busybox", "tr", ",", "'\n'", "|", "/bin/busybox", "xargs", "-n1", "/bin/busybox", "ln", "-s", "busybox")
		cmd.Dir = "/bin"
		err = cmd.Run()
		if err != nil {
			fmt.Printf(err.Error())
		}
	*/

	runtime.GOMAXPROCS(runtime.NumCPU())

	// set deadline scheduler
	ioutil.WriteFile("/sys/block/sda/queue/scheduler", []byte("deadline\n"), os.FileMode(0644))
}
