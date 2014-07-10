package main

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
	"unsafe"

	netlink "./netlink"
	"github.com/d2g/dhcp4"
	"github.com/d2g/dhcp4client"
	"gopkg.in/yaml.v1"
)

type Config struct {
	Users []struct {
		Name   string
		Passwd string
	}
	Bootstrap struct {
		Name    string
		Arch    string
		Fetch   []string
		Version string
	}
}

func ioctl(fd uintptr, name int, data unsafe.Pointer) error {
	_, _, err := syscall.RawSyscall(syscall.SYS_IOCTL, fd, uintptr(name), uintptr(data))
	if err != 0 {
		return errors.New("failed to run ioctl")
	}
	return nil
}

type ProgressReader struct {
	io.Reader
	total int64 // Total # of bytes transferred
}

func (r *ProgressReader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if err == nil {
		r.total += int64(n)
		fmt.Printf("Read %d bytes from %d", n, r.total)
	}
	return n, err
}

func network() {
	var cmdline_iface string = "all"
	var cmdline_mode string

	cmdline, err := ioutil.ReadFile("/proc/cmdline")
	if err != nil {
		return
	}

	for _, token := range strings.Split(string(cmdline), " ") {
		parts := strings.SplitN(token, "=", 2)

		key := parts[0]
		key = strings.Replace(key, "_", "-", -1)

		if key != "ip" {
			continue
		}

		if len(parts) != 2 {
			fmt.Printf("Found ip in /proc/cmdline with no value, ignoring.")
			continue
		}

		conf := strings.TrimSpace(parts[1])
		if strings.Index(conf, ":") > 0 {
			ifparts := strings.SplitN(conf, ":", 2)
			cmdline_iface = ifparts[0]
			cmdline_mode = ifparts[1]
		} else {
			cmdline_mode = conf
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("failed to list interfaces: %s\n", err.Error())
		os.Exit(1)
	}

	for _, iface := range ifaces {
		if cmdline_iface != "all" && iface.Name != cmdline_iface {
			continue
		}
		err := netlink.NetworkLinkUp(&iface)
		if err != nil {
			fmt.Printf("failed to bring up interface %s: %s", iface, err)
		} else {
			fmt.Printf("bring up interface: %s\n", iface.Name)
		}
	}
	fmt.Printf("hold some time to get interfaces up\n")
	time.Sleep(5 * time.Second)

	ifaces, err = net.Interfaces()
	if err != nil {
		fmt.Printf("failed to list interfaces: %s\n", err.Error())
		os.Exit(1)
	}

	fmt.Printf("iface: %s mode: %s\n", cmdline_iface, cmdline_mode)

	if cmdline_mode == "auto6" || cmdline_mode == "dhcp6" {
		for _, iface := range ifaces {
			if cmdline_iface != "all" && iface.Name != cmdline_iface {
				continue
			}

			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				a := strings.Split(addr.String(), "/")[0]
				ip := net.ParseIP(a)
				if ip == nil {
					continue
				}

				if ip.To4() != nil {
					if strings.HasPrefix(a, "fe80") {
						continue
					}
					routes, err := netlink.NetworkGetRoutes()
					if err == nil {
						for _, route := range routes {
							if route.Default {
								err = ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver %s\n", route.IP)), 0644)
								if err != nil {
									fmt.Printf("failed to write resolv.conf")
								}
								return
							}
						}
					}
				}
			}
		}
	}

	if cmdline_mode == "auto4" || cmdline_mode == "dhcp4" {
		for _, iface := range ifaces {
			if cmdline_iface != "all" && iface.Name != cmdline_iface {
				continue
			}
			if iface.Flags&net.FlagLoopback == 0 {
				err := netlink.AddDefaultGw("0.0.0.0", iface.Name)
				if err != nil {
					fmt.Printf("failed to add route to interface %s: %s", iface, err)
				}

				client := dhcp4client.Client{}
				client.IgnoreServers = []net.IP{}
				client.MACAddress = iface.HardwareAddr
				client.Timeout = (10 * time.Second)
				err = client.Connect()
				if err != nil {
					fmt.Printf("failed to listen dhcp %s", err)
				}

				ok, packet, err := client.Request()
				if !ok || err != nil {
					fmt.Printf("request failed %s\n", err)
					os.Exit(1)
				}
				opts := packet.ParseOptions()
				fmt.Printf("dhcp options: %+v\n", opts)
				err = netlink.NetworkLinkAddIp(&iface, packet.YIAddr(), &net.IPNet{
					IP:   packet.YIAddr(),
					Mask: net.IPMask(opts[dhcp4.OptionSubnetMask]),
				})
				if err != nil {
					fmt.Printf("failed to add ip to interface %s: %s", iface, err)
				}
				gw := net.IPv4(opts[3][0], opts[3][1], opts[3][2], opts[3][3])
				err = netlink.AddDefaultGw(fmt.Sprintf("%s", gw), iface.Name)
				if err != nil {
					fmt.Printf("failed to add route to interface %s: %s", iface, err)
				}
				ns := net.IPv4(opts[dhcp4.OptionDomainNameServer][0], opts[dhcp4.OptionDomainNameServer][1], opts[dhcp4.OptionDomainNameServer][2], opts[dhcp4.OptionDomainNameServer][3])
				err = ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver %s\n", ns)), 0644)
				if err != nil {
					fmt.Printf("failed to write resolv.conf")
				}
				//	}(iface)

			}
		}
	}

	fmt.Printf("configured interfaces\n")
	ifaces, err = net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			addrs, _ := iface.Addrs()
			for _, addr := range addrs {
				fmt.Printf("iface %s addr: %s\n", iface.Name, addr)
			}
		}
	}
	time.Sleep(5 * time.Second)
}

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
	err = os.Mkdir("/sys", 0755)
	if err != nil {
		fmt.Printf("failed to create /sys: %s\n", err.Error())
	}
	err = syscall.Mount("sys", "/sys", "sysfs", 0, "")
	if err != nil {
		fmt.Printf("failed to mount /sys: %s\n", err.Error())
		os.Exit(1)
	}
	//	err = os.Mkdir("/dev", 0755)
	//	if err != nil {
	//		fmt.Printf("failed to create /dev: %s\n", err.Error())
	//	}
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

func blkpart(dev string) error {
	BLKRRPART := _IO(0x12, 95)

	w, err := os.OpenFile(dev, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer w.Close()

	var r int
	// TODO: firstly send BLKPG, if it fails send BLKRRPART
	err = ioctl(uintptr(w.Fd()), BLKRRPART, unsafe.Pointer(&r))
	if err != nil {
		return err
	}
	return nil
}

func main() {

	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in f", r)
		}
	}()

	var err error
	var res *http.Response
	var conf Config
	var c *exec.Cmd

	network()

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{Transport: tr}

	authurl := "https://panel.mighost.ru/api.robots/"
	v := url.Values{}
	v.Set("auth_login", "bot_cc")
	v.Set("auth_password", "3jJf20m02B")
	v.Set("action", "get_root_password")
	v.Set("yaml", "1")
	//	v.Set("vps", "484")
	if uuid, err := ioutil.ReadFile("/sys/class/dmi/id/product_uuid"); err == nil {
		v.Set("uuid", fmt.Sprintf("%s", uuid))
	}

	for {
		fmt.Printf("post auth data\n")
		res, err = client.PostForm(authurl, v)
		if err != nil {
			fmt.Printf("failed to get data: %s\n", err.Error())
			continue
		}

		fmt.Printf("get install data\n")
		buf, err := ioutil.ReadAll(res.Body)
		err = yaml.Unmarshal(buf, &conf)
		if err != nil {
			fmt.Printf("failed to get data: %s\n", err.Error())
			continue
		}

		fetch := fmt.Sprintf("%s/%s-%s-%s", conf.Bootstrap.Fetch[0], conf.Bootstrap.Name, conf.Bootstrap.Version, conf.Bootstrap.Arch)
		fmt.Printf("install from: %s\n", fetch)
		res, err = client.Get(fetch)
		if err != nil {
			fmt.Printf("failed to get data: %s\n", err.Error())
			continue
		}
		defer res.Body.Close()
		fmt.Printf("open target dev\n")
		w, err := os.OpenFile("/dev/sda", os.O_WRONLY, 0600)
		if err != nil {
			fmt.Printf("failed to get data: %s\n", err.Error())
			os.Exit(1)
		}

		fmt.Printf("create gzip reader\n")
		r, err := gzip.NewReader(res.Body)
		if err != nil {
			fmt.Printf("failed to get data: %s\n", err.Error())
			w.Close()
			continue
		}
		defer r.Close()
		fmt.Printf("copy data\n")
		_, err = io.Copy(w, r)
		if err != nil {
			fmt.Printf("failed to install: %s\n", err.Error())
			r.Close()
			w.Close()
			time.Sleep(10 * time.Minute)
			//			os.Exit(1)
		}
		w.Close()
		fmt.Printf("install complete\n")

		break
	}

	fmt.Printf("update partition table\n")
	if blkpart("/dev/sda") != nil {
		fmt.Printf("failed to update partition table: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	fmt.Printf("mount dev to mnt\n")
	err = syscall.Mount("/dev/sda1", "/mnt", "ext4", syscall.MS_RELATIME, "data=writeback,barrier=0")
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	fmt.Printf("mount devtmpfs\n")
	err = syscall.Mount("devtmpfs", "/mnt/dev", "devtmpfs", 0, "mode=0755")
	if err != nil {
		fmt.Printf("failed to mount /mnt/dev: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	attr := &syscall.SysProcAttr{Chroot: "/mnt"}
	var buf []byte

	fmt.Printf("run fdisk\n")
	stdin := new(bytes.Buffer)
	stdin.Write([]byte("o\nn\np\n1\n2048\n\nw\n"))
	//for _, p := range []string{"/sbin", "/usr/sbin"} {
	c = exec.Command("/bin/busybox", "fdisk", "-u", "/dev/sda")
	c.Dir = "/"
	c.Stdin = stdin
	//		c.SysProcAttr = attr
	buf, err = c.CombinedOutput()
	fmt.Printf("output: %s\n", buf)
	stdin.Reset()
	//}
	fmt.Printf("unmount /mnt/dev\n")
	err = syscall.Unmount("/mnt/dev", syscall.MNT_DETACH)
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}
	fmt.Printf("unmount /mnt\n")
	err = syscall.Unmount("/mnt", syscall.MNT_DETACH)
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	fmt.Printf("update partition table\n")
	if blkpart("/dev/sda") != nil {
		fmt.Printf("failed to update partition table: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	fmt.Printf("mount dev to /mnt\n")
	err = syscall.Mount("/dev/sda1", "/mnt", "ext4", syscall.MS_RELATIME, "data=writeback,barrier=0")
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	fmt.Printf("mount devtmpfs\n")
	err = syscall.Mount("devtmpfs", "/mnt/dev", "devtmpfs", 0, "mode=0755")
	if err != nil {
		fmt.Printf("failed to mount /mnt/dev: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	err = syscall.Mount("proc", "/mnt/proc", "proc", 0, "")
	if err != nil {
		fmt.Printf("failed to mount /mnt/proc: %s\n", err.Error())
		os.Exit(1)
	}
	err = syscall.Mount("sys", "/mnt/sys", "sysfs", 0, "")
	if err != nil {
		fmt.Printf("failed to mount /sys: %s\n", err.Error())
		os.Exit(1)
	}

	for _, p := range []string{"/sbin", "/usr/sbin"} {
		/*
			fmt.Printf("fsck dev\n")
			c = exec.Command(p+"/fsck -fy", "/dev/sda1")
			c.Dir = "/"
			c.SysProcAttr = attr
			buf, err = c.CombinedOutput()
			fmt.Printf("err : %s output: %s\n", err, buf)
		*/
		fmt.Printf("resize2fs dev\n")
		c = exec.Command(p+"/resize2fs", "/dev/sda1")
		c.Dir = "/"
		c.SysProcAttr = attr
		buf, err = c.CombinedOutput()
		fmt.Printf("err : %s output: %s\n", err, buf)

		fmt.Printf("set root password\n")
		stdin.Write([]byte("root:" + conf.Users[0].Passwd))
		c = exec.Command(p+"/chpasswd", "-e")
		c.Dir = "/"
		c.Stdin = stdin
		c.SysProcAttr = attr
		buf, err = c.CombinedOutput()
		fmt.Printf("err: %s output: %s\n", err, buf)
		stdin.Reset()
	}

	fmt.Printf("unmount /mnt/dev\n")
	err = syscall.Unmount("/mnt/dev", syscall.MNT_DETACH)
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}
	fmt.Printf("unmount /mnt\n")
	err = syscall.Unmount("/mnt", syscall.MNT_DETACH)
	if err != nil {
		fmt.Printf("failed to mount: %s\n", err.Error())
		time.Sleep(10 * time.Minute)
		os.Exit(1)
	}

	syscall.Sync()
	//	syscall.Reboot(syscall.LINUX_REBOOT_CMD_RESTART2)
	syscall.Reboot(syscall.LINUX_REBOOT_CMD_POWER_OFF)
	stdin.Reset()
}
