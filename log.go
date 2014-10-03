package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func log(s string) error {
	var host string
	var port string

	_, metadataUrl, err = cmdlineVar("cloud-config-url")

	u, _ := url.Parse(metadataUrl)
	if strings.Index(u.Host, ":") > 0 {
		host, port, _ = net.SplitHostPort(u.Host)
	} else {
		host = u.Host
	}
	if port == "" {
		if u.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}

	addrs, err := net.LookupIP(host)
	if err != nil {
		return err
	}

	for _, addr := range addrs {
		if ipv4 && addr.To4() == nil {
			continue
		}
		if ipv6 && addr.To4() != nil {
			continue
		}

		req, _ := http.NewRequest("GET", metadataUrl+"&action=log&message="+s, nil)
		req.URL = u
		req.URL.Host = net.JoinHostPort(addr.String(), port)
		req.Host = host

		res, err = httpClient.Do(req)
		if err != nil {
			if debug {
				fmt.Printf("http %s", err)
				time.Sleep(10 * time.Second)
			}
		}
	}
}
