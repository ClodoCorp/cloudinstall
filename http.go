package main

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"gopkg.in/yaml.v1"
)

func getDataSource() (dataSource DataSource, err error) {
	httpTransport := &http.Transport{
		Dial:            (&net.Dialer{DualStack: true}).Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 10 * time.Second}

	var res *http.Response
	var urlDataSource string
	var buffer []byte

	_, urlDataSource, err = cmdlineVar("cloud-config-url")
	if err != nil {
		return
	}
	var host string
	var port string
	u, _ := url.Parse(urlDataSource)
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
		return dataSource, err
	}

	for _, addr := range addrs {
		if ipv4 && addr.To4() == nil {
			continue
		}
		if ipv6 && addr.To4() != nil {
			continue
		}

		req, _ := http.NewRequest("GET", urlDataSource, nil)
		req.URL = u
		req.URL.Host = net.JoinHostPort(addr.String(), port)
		req.Host = host

		res, err = httpClient.Do(req)
		if err != nil {
			if debug {
				fmt.Printf("http: %s", err)
				time.Sleep(10 * time.Second)
			}
			continue
		}
		defer res.Body.Close()

		buffer, err = ioutil.ReadAll(res.Body)
		if err != nil {
			continue
		}
		err = yaml.Unmarshal(buffer, &dataSource)
		if err != nil {
			continue
		}
		return dataSource, nil
	}

	return
}

func getCloudConfig(dataSource DataSource) (cloudConfig CloudConfig, err error) {
	httpTransport := &http.Transport{
		Dial:            (&net.Dialer{DualStack: true}).Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	var metadataUrl string
	var metadataUrls []string
	httpClient := &http.Client{Transport: httpTransport, Timeout: 10 * time.Second}
	var res *http.Response
	var buffer []byte

	if dataSource.Datasource.Ec2.MetadataUrls == nil {
		_, metadataUrl, err = cmdlineVar("cloud-config-url")
		metadataUrls = append(metadataUrls, metadataUrl)
	} else {
		metadataUrls = dataSource.Datasource.Ec2.MetadataUrls
	}

	for _, metadataUrl := range metadataUrls {
		var host string
		var port string
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
			return cloudConfig, err
		}

		for _, addr := range addrs {
			if ipv4 && addr.To4() == nil {
				continue
			}
			if ipv6 && addr.To4() != nil {
				continue
			}

			req, _ := http.NewRequest("GET", metadataUrl, nil)
			req.URL = u
			req.URL.Host = net.JoinHostPort(addr.String(), port)
			req.Host = host

			res, err = httpClient.Do(req)
			if err != nil {
				if debug {
					fmt.Printf("http %s", err)
					time.Sleep(10 * time.Second)
				}
				continue
			}
			defer res.Body.Close()

			buffer, err = ioutil.ReadAll(res.Body)
			if err != nil {
				continue
			}
			err = yaml.Unmarshal(buffer, &cloudConfig)
			if err != nil {
				if debug {
					fmt.Printf("http %s", err)
					time.Sleep(10 * time.Second)
				}
				continue
			}
		}
		return cloudConfig, nil
	}
	return cloudConfig, fmt.Errorf("failed to get cloud-config")
}
