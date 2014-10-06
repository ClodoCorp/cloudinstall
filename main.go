package main

import (
	"bytes"
	"fmt"
	"path/filepath"
	"time"

	"os/exec"
	"syscall"
)

func main() {
	dst := "/dev/sda"

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	var err error
	var c *exec.Cmd
	var cloudConfig CloudConfig

Network:
	for {
		err = configNetwork()
		exit_fail(err)

		/*
			fmt.Printf("get DataSource\n")
			dataSource, err := getDataSource()
			if err != nil {
				if debug {
					fmt.Printf("get DataSource err: %s\n", err)
				}
				continue
			}
		*/
		fmt.Printf("get CloudConfig\n")
		cloudConfig, err = getCloudConfig(DataSource{})
		if err != nil {
			logError(fmt.Sprintf("get CloudConfig err: %s\n", err))
			if debug {
				fmt.Printf("get CloudConfig err: %s\n", err)
				time.Sleep(10 * time.Second)
			}
			continue
		}
		break Network
	}

Disk:
	for _, srv := range cloudConfig.Bootstrap.Fetch {
		src := fmt.Sprintf("%s/%s-%s-%s", srv, cloudConfig.Bootstrap.Name, cloudConfig.Bootstrap.Version, cloudConfig.Bootstrap.Arch)
		fmt.Printf("copy image %s %s\n", src, dst)
		err = copyImage(src, dst)
		if err != nil {
			logError(fmt.Sprintf("copy image err: %s\n", err))
			if debug {
				fmt.Printf("copy image err: %s\n", err)
				time.Sleep(10 * time.Second)
			}
			continue
		} else {
			break Disk
		}
	}
	if err != nil {
		goto Network
	}

	exit_fail(blkpart(dst))

	parts, err := filepath.Glob("/dev/sda?")
	exit_fail(err)

	if len(parts) == 1 {

		chroot := &syscall.SysProcAttr{Chroot: "/mnt"}

		stdin := new(bytes.Buffer)
		stdin.Write([]byte("o\nn\np\n1\n2048\n\nw\n"))
		c = exec.Command("/bin/busybox", "fdisk", "-u", dst)
		c.Dir = "/"
		c.Stdin = stdin
		_, err = c.CombinedOutput()
		exit_fail(err)
		stdin.Reset()

		exit_fail(blkpart(dst))

		var fstype string
		for _, fs := range []string{"ext4", "btrfs"} {
			err = mount("/dev/sda1", "/mnt", fs, syscall.MS_RELATIME, "data=writeback,barrier=0")
			if err != nil {
				continue
			}
			fstype = fs
		}
		if fstype == "" {
			exit_fail(fmt.Errorf("failed to determine fstype"))
		}
		exit_fail(mount("devtmpfs", "/mnt/dev", "devtmpfs", 0, "mode=0755"))

		exit_fail(mount("proc", "/mnt/proc", "proc", 0, ""))

		exit_fail(mount("sys", "/mnt/sys", "sysfs", 0, ""))

		switch fstype {
		case "ext3", "ext4":
			resize2fs, err := lookupPathChroot("resize2fs", "/mnt", []string{"/sbin", "/usr/sbin"})
			exit_fail(err)

			c = exec.Command(resize2fs, "/dev/sda1")
			c.Dir = "/"
			c.SysProcAttr = chroot
			_, err = c.CombinedOutput()
			exit_fail(err)
		case "btrfs":
			btrfs, err := lookupPathChroot("btrfs", "/mnt", []string{"/sbin", "/usr/sbin"})
			exit_fail(err)
			c = exec.Command(btrfs, "filesystem", "resize", "max", "/")
			c.Dir = "/"
			c.SysProcAttr = chroot
			_, err = c.CombinedOutput()
			exit_fail(err)
		}

		for _, user := range cloudConfig.Users {
			chpasswd, err := lookupPathChroot("chpasswd", "/mnt", []string{"/sbin", "/usr/sbin"})
			exit_fail(err)

			fmt.Printf("set root password\n")
			stdin.Write([]byte(user.Name + ":" + user.Passwd))
			c = exec.Command(chpasswd, "-e")
			c.Dir = "/"
			c.Stdin = stdin
			c.SysProcAttr = chroot
			_, err = c.CombinedOutput()
			exit_fail(err)
			stdin.Reset()
		}
		/*
			w, err := os.OpenFile("/mnt/.autorelabel", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
			if err == nil {
				w.Close()
			}
		*/
		exit_fail(unmount("/mnt/dev", syscall.MNT_DETACH))

		exit_fail(unmount("/mnt", syscall.MNT_DETACH))

	}
	sync()

	logComplete("install success")
	reboot()
}
