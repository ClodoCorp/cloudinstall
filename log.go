package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func logError(s string) error {
	return httplog("error", s)
}

func logFatal(s string) error {
	return httplog("fatal", s)
}

func logComplete(s string) error {
	return httplog("complete", s)
}

func httplog(t, s string) error {
	httpTransport := &http.Transport{
		Dial:            (&net.Dialer{DualStack: true}).Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 10 * time.Second}

	var host string
	var port string

	_, metadataUrl, err := cmdlineVar("cloud-config-url")

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

		logurl := ""
		switch t {
		case "error":
			logurl = metadataUrl + "&action=log&flag=install_error&message=" + url.QueryEscape(s)
		case "fatal":
			logurl = metadataUrl + "&action=log&flag=install_fatal&message=" + url.QueryEscape(s)
		case "complete":
			logurl = metadataUrl + "&action=log&flag=install_complete&message=" + url.QueryEscape(s)
		default:
			return fmt.Errorf("unknown log level %s", t)
		}

		req, err := http.NewRequest("GET", logurl, nil)
		if err != nil {
			if debug_mode {
				fmt.Printf("http %s\n", err)
				time.Sleep(10 * time.Second)
			}
		}
		req.URL, err = url.Parse(logurl)
		if err != nil {
			if debug_mode {
				fmt.Printf("http %s\n", err)
				time.Sleep(10 * time.Second)
			}
		}
		req.URL.Host = net.JoinHostPort(addr.String(), port)
		req.Host = host

		res, err := httpClient.Do(req)
		if err != nil {
			if debug_mode {
				fmt.Printf("http err %s\n", err)
				time.Sleep(10 * time.Second)
			}
		}
		if res != nil && res.Body != nil {
			defer res.Body.Close()
		}
	}
	return nil
}
