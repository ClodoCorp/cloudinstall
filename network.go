package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	netlink "./netlink"
	"github.com/d2g/dhcp4"
	"github.com/d2g/dhcp4client"
)

func configNetwork() (err error) {

	var cmdline_ifaces []string
	var cmdline_mode string

	_, values, err := cmdlineVar("ip")
	if err == nil {
		if strings.Index(values, ":") == 0 {
			cmdline_mode = values
		}
		if strings.Index(values, ":") > 0 {
			ifparts := strings.SplitN(values, ":", 2)
			cmdline_ifaces = append(cmdline_ifaces, ifparts[0])
			cmdline_mode = ifparts[1]
		}
	}

	if len(cmdline_ifaces) == 0 {
		ifaces, err := net.Interfaces()
		exit_fail(err)
		for _, iface := range ifaces {
			cmdline_ifaces = append(cmdline_ifaces, iface.Name)
		}
	}
	var err4, err6 error

	if cmdline_mode == "auto4" || cmdline_mode == "dhcp4" {
		err4 = networkAuto4(cmdline_ifaces)
	}
	if debug && err4 != nil {
		fmt.Printf("ipv4 error: %s\n", err4.Error())
	}

	if cmdline_mode == "auto6" || cmdline_mode == "dhcp6" {
		err6 = networkAuto6(cmdline_ifaces)
	}
	if debug && err6 != nil {
		fmt.Printf("ipv6 error: %s\n", err6.Error())
	}

	if err4 != nil && err6 != nil {
		return fmt.Errorf(err4.Error() + err6.Error())
	}
	return
}

func networkIfacesUp(ifaces []string) (err error) {
	for _, ifname := range ifaces {
		iface, err := net.InterfaceByName(ifname)
		exit_fail(err)

		exit_fail(netlink.NetworkLinkUp(iface))
	}
	time.Sleep(2 * time.Second)
	return nil
}

func networkAuto6(ifaces []string) (err error) {
	err = fmt.Errorf("failed to configure ipv6")

	for _, ifname := range ifaces {

		iface, err := net.InterfaceByName(ifname)
		exit_fail(err)

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
							//							exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver %s\n", route.IP)), 0644))
							exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver 2001:4860:4860::8888\nnameserver 2001:4860:4860::8844\n")), 0644))
							return err
						}
					}
				}
			}
		}
	}
	return err
}

func networkAuto4(ifaces []string) (err error) {
	err = fmt.Errorf("failed to configure ipv4")
	for _, ifname := range ifaces {

		iface, err := net.InterfaceByName(ifname)
		exit_fail(err)

		if iface.Flags&net.FlagLoopback == 0 {
			exit_fail(netlink.AddDefaultGw("0.0.0.0", ifname))

			client := dhcp4client.Client{}
			client.IgnoreServers = []net.IP{}
			client.MACAddress = iface.HardwareAddr
			client.Timeout = (10 * time.Second)
			exit_fail(client.Connect())

			ok, packet, err := client.Request()
			if !ok || err != nil {
				return fmt.Errorf("can't do dhcp request")
			}
			opts := packet.ParseOptions()
			exit_fail(netlink.NetworkLinkAddIp(iface, packet.YIAddr(), &net.IPNet{
				IP:   packet.YIAddr(),
				Mask: net.IPMask(opts[dhcp4.OptionSubnetMask]),
			}))

			gw := net.IPv4(opts[3][0], opts[3][1], opts[3][2], opts[3][3])
			exit_fail(netlink.AddDefaultGw(fmt.Sprintf("%s", gw), ifname))

			//			ns := net.IPv4(opts[dhcp4.OptionDomainNameServer][0], opts[dhcp4.OptionDomainNameServer][1], opts[dhcp4.OptionDomainNameServer][2], opts[dhcp4.OptionDomainNameServer][3])
			//			exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver %s\n", ns)), 0644))
			exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver 8.8.8.8\nnameserver 8.8.4.4\n")), 0644))
		}
	}
	return nil
}
