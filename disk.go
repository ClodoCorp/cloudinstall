package main

import (
	"bufio"
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

	"github.com/biogo/hts/bgzf"
	"github.com/cheggaaa/pb"
	pgzip "github.com/klauspost/pgzip"
	compress "github.com/vtolstov/packer-post-processor-compress/compress"
	ranger "github.com/vtolstov/ranger"
	"gopkg.in/yaml.v2"
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
	var bar *pb.ProgressBar
	//	var n int64

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
				req.Method = "GET"
				res, err = httpClient.Do(req)
				if err == nil && res.StatusCode == 200 && res.Body != nil {
					rd := bufio.NewReader(res.Body)
				lines:
					for {
						line, err := rd.ReadString('\n')
						if err != nil {
							break lines
						}
						parts := strings.Fields(line)
						if parts[1] == img {
							checksum = parts[0]
						}
					}
					res.Body.Close()
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
			}
			meta := make(compress.Metadata, 0)
			req.Method = "GET"
			req.URL = mu
			res, err = httpClient.Do(req)
			if err == nil && res.StatusCode == 200 && res.Body != nil {
				metaBody, _ := ioutil.ReadAll(res.Body)
				res.Body.Close()
				if err = yaml.Unmarshal(metaBody, &meta); err != nil {
					fmt.Printf("metadata err %s\n", err.Error())
				}
			} else {
				if debug && err != nil {
					fmt.Printf("meta: %s\n", err.Error())
					time.Sleep(20 * time.Second)
				}
			}

			if debug {
				fmt.Printf("meta %+v\n", meta)
				time.Sleep(5 * time.Second)
			}

			if len(meta) > 0 {
				if m, ok := meta[img]; ok {
					if m.OrigSize != 0 {
						bar = pb.New64(m.OrigSize)
						bar.ShowSpeed = true
						bar.ShowTimeLeft = true
						bar.ShowPercent = true
						bar.SetRefreshRate(time.Second)
						bar.SetWidth(80)
						bar.SetMaxWidth(80)
						bar.SetUnits(pb.U_BYTES)
						bar.Start()
						defer bar.Finish()
					}
				}
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
			rf := &ranger.HTTPRanger{URL: u}
			if err = rf.Initialize(0); err != nil {
				fmt.Printf("rf err %s\n", err.Error())
				continue
			}

			rs := res.Body
			defer res.Body.Close()

			fw, err := os.OpenFile(dev, os.O_WRONLY, 0600)
			if err != nil {
				fmt.Printf("open err: %s\n", err)
				time.Sleep(10 * time.Second)
				return err
			}
			//TODO: check for error
			defer fw.Close()

			comptype := ""
			if len(meta) > 0 {
				comptype = meta[img].CompType
			}

			pr, pw := io.Pipe()
			var cmw io.Writer
			if checksum != "" {
				cmw = io.MultiWriter(pw, h)
			} else {
				cmw = io.MultiWriter(pw)
			}

			go func() {
				//TODO: chck for error
				_, err := io.Copy(cmw, rs)
				if err != nil {
					fmt.Printf("copy err: %s\n", err)
					time.Sleep(10 * time.Second)
				}
				err = pw.Close()
				if err != nil {
					fmt.Printf("close err: %s\n", err)
					time.Sleep(10 * time.Second)
				}
			}()

			switch comptype {
			case "bgzf":
				gr, err = bgzf.NewReader(pr, runtime.NumCPU())
			case "pgzip":
				gr, err = pgzip.NewReader(pr)
			case "gzip":
				gr, err = gzip.NewReader(pr)
			default:
				if gr, err = pgzip.NewReader(pr); err != nil {
					if gr, err = bgzf.NewReader(pr, runtime.NumCPU()); err != nil {
						if gr, err = gzip.NewReader(pr); err != nil {
							fmt.Printf("gz error: %s\n", err)
							return err
						}
					}
				}
			}

			if err != nil {
				return err
			}

			defer gr.Close()
			writers := []io.Writer{fw}

			if len(meta) > 0 {
				if m, ok := meta[img]; ok && m.OrigSize != 0 {
					writers = append(writers, bar)
				}
			}

			mw = io.MultiWriter(writers...)
			io.Copy(mw, gr)

			if checksum != "" {
				if checksum != fmt.Sprintf("%x", h.Sum(nil)) {
					err = fmt.Errorf("checksum mismatch %s != %s", checksum, fmt.Sprintf("%x", h.Sum(nil)))
					if debug {
						fmt.Printf("%s\n", err.Error())
						time.Sleep(10 * time.Second)
					}
					return err
				} else {
					fmt.Printf("checksum ok %s == %s\n", checksum, fmt.Sprintf("%x", h.Sum(nil)))
				}
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
