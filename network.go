package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"strings"
	"time"

	"github.com/d2g/dhcp4"
	"github.com/d2g/dhcp4client"
	"github.com/vishvananda/netlink"
)

var ipv4 bool = false
var ipv6 bool = false

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

	if cmdline_mode == "auto4" || cmdline_mode == "dhcp4" || cmdline_mode == "auto" {
		err4 = networkAuto4(cmdline_ifaces)
		if err4 == nil {
			ipv4 = true
		}
	}
	if debug && err4 != nil {
		fmt.Printf("ipv4 error: %s\n", err4.Error())
	}

	if cmdline_mode == "auto6" || cmdline_mode == "dhcp6" || cmdline_mode == "auto" {
		err6 = networkAuto6(cmdline_ifaces)
		if err6 == nil {
			ipv6 = true
		}
	}
	if debug && err6 != nil {
		fmt.Printf("ipv6 error: %s\n", err6.Error())
	}

	if debug {
		ifaces, err := net.Interfaces()
		exit_fail(err)
		for _, iface := range ifaces {
			fmt.Printf("iface: %+v\n", iface)
			addrs, _ := iface.Addrs()
			fmt.Printf("iface addrs: %+v\n", addrs)
		}
		time.Sleep(10 * time.Second)
	}

	if err4 != nil && err6 != nil {
		return fmt.Errorf(err4.Error() + err6.Error())
	}
	return
}

func networkIfacesUp(ifaces []string) (err error) {
	for _, ifname := range ifaces {
		link, err := netlink.LinkByName(ifname)
		exit_fail(err)

		exit_fail(netlink.LinkSetUp(link))
	}
	time.Sleep(2 * time.Second)
	return nil
}

func networkAuto6(ifaces []string) error {

	exit_fail(networkIfacesUp(ifaces))
	exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver 2001:4860:4860::8888\nnameserver 2001:4860:4860::8844\n")), 0644))

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
				exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver 2001:4860:4860::8888\nnameserver 2001:4860:4860::8844\n")), 0644))
			}
		}
	}
	return nil
}

func flushAddr(ifaces []string, family string) (err error) {
	for _, ifname := range ifaces {
		iface, err := net.InterfaceByName(ifname)
		exit_fail(err)

		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			if debug {
				fmt.Printf("present addr: %s\n", addr.String())
			}
			ip, ipnet, err := net.ParseCIDR(addr.String())
			if err != nil {
				fmt.Printf("parsecidr error: %s\n", err.Error())
			}
			if family == "ipv4" && ip.To4() == nil {
				if debug {
					fmt.Printf("family ipv4 skip %s\n", ip)
				}
				continue
			}
			if family == "ipv6" && ip.To4() != nil {
				if debug {
					fmt.Printf("family ipv6 skip %s\n", ip)
				}
				continue
			}

			if debug {
				fmt.Printf("try to remove addr %s\n", addr.String())
			}

			link, err := netlink.LinkByName(ifname)
			if err != nil {
				fmt.Printf("link err: %s\n", err.Error())
			}

			naddr := &netlink.Addr{}
			naddr.IP = ipnet.IP
			naddr.Mask = ipnet.Mask
			err = netlink.AddrDel(link, naddr)
			if err != nil {
				fmt.Printf("ipdel err: %s\n", err.Error())
			}
		}
	}
	return nil
}

func networkAuto4(ifaces []string) (err error) {
	err = fmt.Errorf("failed to configure ipv4")

	exit_fail(networkIfacesUp(ifaces))
	exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver 8.8.8.8\nnameserver 8.8.4.4\n")), 0644))

	for _, ifname := range ifaces {
		iface, err := net.InterfaceByName(ifname)
		exit_fail(err)
		link, err := netlink.LinkByName(ifname)
		exit_fail(err)

		if iface.Flags&net.FlagLoopback == 0 {
			flushAddr(ifaces, "ipv4")
			if debug {
				fmt.Printf("try to get link %+v\n", link)
			}

			routes, err := netlink.RouteList(link, netlink.FAMILY_V4)
			exit_fail(err)

			if debug {
				fmt.Printf("try to get routes %+v\n", routes)
			}

			if routes != nil && len(routes) > 0 {
				for _, route := range routes {
					if debug {
						fmt.Printf("try to remove route %s\n", route)
					}
					netlink.RouteDel(&route)
				}
			}

			if debug {
				fmt.Printf("add default route to 0.0.0.0\n")
			}
			exit_fail(netlink.RouteAdd(&netlink.Route{LinkIndex: link.Attrs().Index, Dst: &net.IPNet{}, Src: net.ParseIP("0.0.0.0")}))

			if debug {
				fmt.Printf("send dhcp4 request\n")
			}

			client := dhcp4client.Client{}
			client.IgnoreServers = []net.IP{}
			client.MACAddress = iface.HardwareAddr
			client.Timeout = (10 * time.Second)
			exit_fail(client.Connect())
			defer client.Close()
			ok, packet, err := client.Request()
			if !ok || err != nil {
				return fmt.Errorf("can't do dhcp request")
			}
			opts := packet.ParseOptions()

			ipnet := net.IPNet{
				IP:   packet.YIAddr(),
				Mask: net.IPMask(opts[dhcp4.OptionSubnetMask]),
			}
			addr, err := netlink.ParseAddr(ipnet.String())
			exit_fail(err)

			exit_fail(netlink.AddrAdd(link, addr))

			gw := net.IPv4(opts[3][0], opts[3][1], opts[3][2], opts[3][3])
			r := &netlink.Route{
				LinkIndex: link.Attrs().Index,
				Dst:       &net.IPNet{},
				Gw:        gw,
			}
			if debug {
				fmt.Printf("del default route to 0.0.0.0\n")
			}
			exit_fail(netlink.RouteDel(&netlink.Route{LinkIndex: link.Attrs().Index, Src: net.ParseIP("0.0.0.0"), Dst: &net.IPNet{}}))

			if debug {
				fmt.Printf("set default route to %s\n", gw.String())
			}
			exit_fail(netlink.RouteAdd(r))

			//			ns := net.IPv4(opts[dhcp4.OptionDomainNameServer][0], opts[dhcp4.OptionDomainNameServer][1], opts[dhcp4.OptionDomainNameServer][2], opts[dhcp4.OptionDomainNameServer][3])
			//			exit_fail(ioutil.WriteFile("/etc/resolv.conf", []byte(fmt.Sprintf("nameserver %s\n", ns)), 0644))
		}
	}
	return nil
}
