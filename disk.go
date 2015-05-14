package main

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/tls"
	"fmt"
	"hash"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strconv"
	"strings"
	"time"

	bgzf "github.com/biogo/hts/bgzf"
	"github.com/cheggaaa/pb"
	pgzip "github.com/klauspost/pgzip"
	"github.com/vtolstov/go-ioctl"
)

func getHash(t string) hash.Hash {
	var h hash.Hash
	switch t {
	case "md5":
		h = md5.New()
	case "sha1":
		h = sha1.New()
	case "sha224":
		h = sha256.New224()
	case "sha256":
		h = sha256.New()
	case "sha384":
		h = sha512.New384()
	case "sha512":
		h = sha512.New()
	}
	return h
}

func copyImage(img string, dev string, fetchaddrs []string) (err error) {
	var gr io.ReadCloser
	var h hash.Hash
	var checksum string
	var mw io.Writer

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
			if err != nil || res.StatusCode != 200 {
				if debug {
					err = fmt.Errorf("failed to fetch image %s", req)
					fmt.Printf("http err: %s\n", err)
					time.Sleep(5 * time.Second)
				}
				continue
			}
			i, _ := strconv.Atoi(res.Header.Get("Content-Length"))

			for _, ct := range []string{"md5", "sha1", "sha244", "sha256", "sha384", "sha512"} {
				csum := fmt.Sprintf("%s/%s.%ssums", fetchaddr, img, ct)
				cu, err := url.Parse(csum)
				if err != nil {
					if debug {
						fmt.Printf("url err: %s", err)
						time.Sleep(5 * time.Second)
					}
					continue
				}
				req.URL = cu
				res, err = httpClient.Do(req)
				if err == nil && res.StatusCode == 200 {
					checksumBody, _ := ioutil.ReadAll(res.Body)
					res.Body.Close()
					checksum = strings.Fields(string(checksumBody))[0]
					h = getHash(ct)
				}
			}

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
			req.URL = u

			res, err = httpClient.Do(req)
			if err != nil || res.StatusCode != 200 {
				if debug {
					err = fmt.Errorf("failed to fetch image %s", req)
					fmt.Printf("http err: %s\n", err)
					time.Sleep(10 * time.Second)
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

			if checksum != "" {
				mw = io.MultiWriter(pw, bar, h)
			} else {
				mw = io.MultiWriter(pw, bar)
			}
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

			gr, err = bgzf.NewReader(pr, runtime.NumCPU())
			if err != nil {
				gr, err = pgzip.NewReader(pr)
				if err != nil {
					fmt.Printf("gz error: %s\n", err)
					return err
				}
			}

			defer gr.Close()

			_, err = io.Copy(fw, gr)
			if err != nil {
				fmt.Printf("copy error: %s\n", err)
				return err
			}

			if checksum != "" && checksum != fmt.Sprintf("%x", h.Sum(nil)) {
				err = fmt.Errorf("checksum mismatch %s != %s", checksum, fmt.Sprintf("%x", h.Sum(nil)))
				if debug {
					fmt.Printf("%s\n", err.Error())
					time.Sleep(10 * time.Second)
				}
				return err
			}

			return nil
		}
	}
	return fmt.Errorf("failed to fetch image %s", err.Error())
}

func blkpart(dst string) error {
	w, err := os.OpenFile(dst, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer w.Close()
	return ioctl.BlkRRPart(w.Fd())
}
