package main

import (
	"compress/gzip"
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
	"strings"
	"time"

	"github.com/vtolstov/cloudbootstrap/internal/github.com/biogo/hts/bgzf"
	"github.com/vtolstov/cloudbootstrap/internal/github.com/cheggaaa/pb"
	pgzip "github.com/vtolstov/cloudbootstrap/internal/github.com/klauspost/pgzip"
	compress "github.com/vtolstov/cloudbootstrap/internal/github.com/vtolstov/packer-post-processor-compress/compress"
	ranger "github.com/vtolstov/cloudbootstrap/internal/github.com/vtolstov/ranger"
	"github.com/vtolstov/cloudbootstrap/internal/gopkg.in/yaml.v2"
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
	var bar io.Writer = ioutil.Discard

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

			for _, ct := range []string{"md5", "sha1", "sha244", "sha256", "sha384", "sha512"} {
				csumurl := fmt.Sprintf("%s/%s.%ssums", fetchaddr, img, ct)
				cu, err := url.Parse(csumurl)
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
			metaurl := fmt.Sprintf("%s/%s.metadata", fetchaddr, img)
			mu, err := url.Parse(metaurl)
			if err != nil {
				if debug {
					fmt.Printf("url err: %s", err)
					time.Sleep(5 * time.Second)
				}
				continue
			}

			meta := compress.Metadata{}
			req.URL = mu
			res, err = httpClient.Do(req)
			if err == nil && res.StatusCode == 200 {
				metaBody, _ := ioutil.ReadAll(res.Body)
				res.Body.Close()
				yaml.Unmarshal(metaBody, &meta)
			}

			if meta[img].OrigSize != 0 {
				b := pb.New(int(meta[img].OrigSize))
				b.ShowSpeed = true
				b.ShowTimeLeft = true
				b.ShowPercent = true
				b.SetRefreshRate(time.Second)
				b.SetWidth(80)
				b.SetMaxWidth(80)
				b.SetUnits(pb.U_BYTES)
				b.Start()
				defer b.Finish()
				bar = io.Writer(b)
			}

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
			rs, err := ranger.NewReader(&ranger.HTTPRanger{URL: u})
			//defer rs.Close()

			fw, err := os.OpenFile(dev, os.O_WRONLY, 0600)
			if err != nil {
				return err
			}
			defer fw.Close()

			switch meta[img].CompType {
			case "bgzf":
				gr, err = bgzf.NewReader(rs, runtime.NumCPU())
			case "pgzip":
				gr, err = pgzip.NewReader(rs)
			case "gzip":
				gr, err = gzip.NewReader(rs)
			default:
				gr, err = bgzf.NewReader(rs, runtime.NumCPU())
				if err != nil {
					if gr, err = pgzip.NewReader(rs); err != nil {
						if gr, err = gzip.NewReader(rs); err != nil {
							fmt.Printf("gz error: %s\n", err)
							return err
						}
					}
				} else {
					if ok, err := bgzf.HasEOF(rs); !ok || err != nil {
						if gr, err = pgzip.NewReader(rs); err != nil {
							if gr, err = gzip.NewReader(rs); err != nil {
								fmt.Printf("gz error: %s\n", err)
								return err
							}
						}
					}
				}
			}

			if err != nil {
				return err
			}

			defer gr.Close()

			if checksum != "" {
				mw = io.MultiWriter(fw, bar, h)
			} else {
				mw = io.MultiWriter(fw, bar)
			}

			_, err = io.Copy(mw, gr)
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
