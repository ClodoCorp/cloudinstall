package main

import (
	"bytes"
	"fmt"

	"os/exec"
	"syscall"
)

func main() {

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	var err error
	var c *exec.Cmd
	err = configNetwork()
	exit_fail(err)

	dataSource, err := getDataSource()
	exit_fail(err)

	cloudConfig, err := getCloudConfig(dataSource)
	exit_fail(err)

	for _, srv := range cloudConfig.Bootstrap.Fetch {
		src := fmt.Sprintf("%s/%s-%s-%s", srv, cloudConfig.Bootstrap.Name, cloudConfig.Bootstrap.Version, cloudConfig.Bootstrap.Arch)
		dst := "/dev/sda"

		err = copyImage(src, dst)
		if err != nil {
			continue
		}
	}

	exit_fail(blkpart("/dev/sda"))

	chroot := &syscall.SysProcAttr{Chroot: "/mnt"}

	stdin := new(bytes.Buffer)
	stdin.Write([]byte("o\nn\np\n1\n2048\n\nw\n"))
	c = exec.Command("/bin/busybox", "fdisk", "-u", "/dev/sda")
	c.Dir = "/"
	c.Stdin = stdin
	_, err = c.CombinedOutput()
	exit_fail(err)
	stdin.Reset()

	exit_fail(blkpart("/dev/sda"))

	exit_fail(mount("/dev/sda1", "/mnt", "ext4", syscall.MS_RELATIME, "data=writeback,barrier=0"))

	exit_fail(mount("devtmpfs", "/mnt/dev", "devtmpfs", 0, "mode=0755"))

	exit_fail(mount("proc", "/mnt/proc", "proc", 0, ""))

	exit_fail(mount("sys", "/mnt/sys", "sysfs", 0, ""))

	resize2fs, err := lookupPathChroot("resize2fs", "/mnt", []string{"/sbin", "/usr/sbin"})
	exit_fail(err)

	c = exec.Command(resize2fs, "/dev/sda1")
	c.Dir = "/"
	c.SysProcAttr = chroot
	_, err = c.CombinedOutput()
	exit_fail(err)

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

	exit_fail(unmount("/mnt/dev", syscall.MNT_DETACH))

	exit_fail(unmount("/mnt", syscall.MNT_DETACH))

	sync()

	reboot()
}
