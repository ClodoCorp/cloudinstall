package main

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func lookupPathChroot(prog string, chroot string, dirs []string) (string, error) {
	err := fmt.Errorf("failed to get path for %s", prog)
	for _, dir := range dirs {
		path := filepath.Join(chroot, dir, prog)
		_, err = os.Stat(path)
		if err == nil {
			return filepath.Join(dir, prog), nil
		}
		continue
	}
	return "", err
}

func mount(source string, target string, fstype string, flags uintptr, data string) (err error) {
	return syscall.Mount(source, target, fstype, flags, data)
}

func unmount(target string, flags int) (err error) {
	return syscall.Unmount(target, flags)
}

func reboot() error {
	return syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
}

func sync() {
	syscall.Sync()
}

func fatalf(format string, v ...interface{}) {
	//	fmt.Printf(format, v...)
	fmt.Fprintf(os.Stdout, format, v...)
	os.Exit(1)
}

func exit_fail(err error) {
	if err != nil {
		log(fmt.Sprintf("%s", err))
		fatalf("%s", err)
	}
}
