package main

import (
	//	"compress/gzip"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/cheggaaa/pb"
	gzip "github.com/klauspost/pgzip"
	"github.com/vtolstov/go-ioctl"
)

func copyImage(img string, dev string, fetchaddrs []string) (err error) {
	httpTransport := &http.Transport{
		Dial:            (&net.Dialer{DualStack: true}).Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 5 * time.Minute}

	var host string
	var port string
	var src string

	for _, fetchaddr := range fetchaddrs {
		src = fmt.Sprintf("%s/%s", fetchaddr, img)
		u, err := url.Parse(src)
		if err != nil {
			if debug {
				fmt.Printf("url err: %s", err)
			}
			continue
		}

		if !strings.HasPrefix(u.Host, "[") && strings.Index(u.Host, ":") > 0 {
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

		if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
			host = host[1 : len(host)-1]
		}
		addrs, err := net.LookupIP(host)
		if err != nil {
			addrs = []net.IP{net.ParseIP(host)}
			err = nil
		}

		for _, addr := range addrs {
			if ipv4 && addr.To4() == nil {
				continue
			}
			if ipv6 && addr.To4() != nil {
				continue
			}

			req, _ := http.NewRequest("HEAD", src, nil)
			req.URL = u
			req.URL.Host = net.JoinHostPort(addr.String(), port)
			req.Host = host

			res, err := httpClient.Do(req)
			if err != nil {
				if debug {
					fmt.Printf("http err: %s\n", err)
				}
				continue
			}
			i, _ := strconv.Atoi(res.Header.Get("Content-Length"))
			bar := pb.New(i)
			bar.ShowSpeed = true
			bar.ShowTimeLeft = true
			bar.ShowPercent = true
			bar.SetRefreshRate(time.Second)
			bar.SetWidth(80)
			bar.SetMaxWidth(80)
			bar.SetUnits(pb.U_BYTES)
			bar.Start()
			defer bar.Finish()

			req.Method = "GET"
			res, err = httpClient.Do(req)
			if err != nil {
				if debug {
					fmt.Printf("http err: %s\n", err)
				}
				continue
			}
			defer res.Body.Close()

			fw, err := os.OpenFile(dev, os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer fw.Close()

			pr, pw := io.Pipe()
			mw := io.MultiWriter(pw, bar)
			go func() error {
				_, err := io.Copy(mw, res.Body)
				if err != nil {
					fmt.Printf("copy error: %s\n", err)
					return err
				}

				defer pw.Close()
				return nil
				//			defer pr.Close()
			}()

			gr, err := gzip.NewReader(pr)
			if err != nil {
				fmt.Printf("gz error: %s\n", err)
				return err
			}
			defer gr.Close()

			_, err = io.Copy(fw, gr)
			if err != nil {
				fmt.Printf("copy error: %s\n", err)
				return err
			}

			return nil
		}
	}
	return nil
}

func blkpart(dst string) error {
	w, err := os.OpenFile(dst, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer w.Close()
	return ioctl.BlkRRPart(w.Fd())
}
