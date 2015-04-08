package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"os/exec"
	"strings"
	"syscall"
)

func main() {
	var err error

	/*
		for {
			_, err := os.Stat("/dev/tty")
			if err == nil {
				break
			}
		}

		err = termbox.Init()
		if err != nil {
			fmt.Printf("failed to init terminal: %s\n", err)
			os.Exit(1)
		}
		defer termbox.Close()

		err = termbox.Clear(termbox.ColorWhite, termbox.ColorBlack)
		if err != nil {
			fmt.Printf("failed to clear terminal: %s\n", err)
		}
		termbox.Flush()
	*/

	//	fmt.Print("\033[2J")

	dst := "/dev/sda"

	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("Recovered in %+v\n", r)
		}
	}()

	var c *exec.Cmd
	var cloudConfig CloudConfig
	var stdout io.ReadCloser
	stdin := new(bytes.Buffer)
	chroot := &syscall.SysProcAttr{Chroot: "/mnt"}
	var fstype string

Network:
	for {
		err = configNetwork()
		exit_fail(err)

		if debug {
			fmt.Printf("get CloudConfig\n")
		}
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

	src := fmt.Sprintf("%s-%s-%s", cloudConfig.Bootstrap.Name, cloudConfig.Bootstrap.Version, cloudConfig.Bootstrap.Arch)
	fmt.Printf("install image %s\n", src)
	err = copyImage(src, dst, cloudConfig.Bootstrap.Fetch)
	if err != nil {
		logError(fmt.Sprintf("copy image err: %s\n", err))
		if debug {
			fmt.Printf("copy image err: %s\n", err)
			time.Sleep(10 * time.Second)
		}
		goto Network
	}

	ok, val := cmdlineVar("cloudinit")
	if !ok || val == "false" {
		exit_fail(blkpart(dst))

		parts, err := filepath.Glob("/dev/sda?")
		exit_fail(err)

		var ostype string = "linux"
		if strings.Contains(cloudConfig.Bootstrap.Name, "bsd") {
			ostype = "bsd"
		}

		var partstart string = "2048"
		if len(parts) == 1 {
			c = exec.Command("/bin/busybox", "fdisk", "-lu", dst)
			c.Dir = "/"
			stdout, err = c.StdoutPipe()
			if err != nil {
				goto fail
			}
			r := bufio.NewReader(stdout)

			if err = c.Start(); err != nil {
				goto fail
			}

			for {
				line, err := r.ReadString('\n')
				if err != nil {
					break
				}

				if strings.HasPrefix(line, dst) {
					ps := strings.Fields(line) // /dev/sda1      *      4096   251658239   125827072  83 Linux
					if ps[1] == "*" {
						partstart = ps[2]
					} else {
						partstart = ps[1]
					}
				}
			}

			if err = c.Wait(); err != nil || partstart == "" {
				goto fail
			}

			switch ostype {
			case "linux":
				stdin.Write([]byte("o\nn\np\n1\n" + partstart + "\n\na\n1\nw\n"))
				c = exec.Command("/bin/busybox", "fdisk", "-u", dst)
				c.Dir = "/"
				c.Stdin = stdin
				_, err = c.CombinedOutput()
				exit_fail(err)
				stdin.Reset()
				exit_fail(blkpart(dst))
			}

			switch ostype {
			case "linux":
				for _, fs := range []string{"ext4", "btrfs"} {
					err = mount("/dev/sda1", "/mnt", fs, syscall.MS_RELATIME, "data=writeback,discard,barrier=0")
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

					if debug {
						fmt.Printf("set root password\n")
					}

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
		}
	}
	sync()

	logComplete("install success")
	reboot()
	return

fail:
	logFatal("install fail")
	time.Sleep(50 * time.Minute)
	reboot()
	return
}
