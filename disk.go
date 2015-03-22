package main

// #cgo pkg-config: ext2fs com_err
// #include <ext2fs/ext2fs.h>
// #include <stdlib.h>
// #include <et/com_err.h>
import "C"

import (
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
	"unsafe"

	"code.google.com/p/biogo.bam/bgzf"

	"github.com/vtolstov/go-ioctl"
	"github.com/vtolstov/go-nbd"
)

type DeviceReader struct {
	cr   *bgzf.Reader
	fs   C.ext2_filsys
	size int64
	hr   *HTTPReader
}

type HTTPReader struct {
	c *http.Client

	start  int64
	length int64

	hostport string
	host     string
	url      *url.URL
	src      string
}

func NewDevice(c *http.Client, hostport string, host string, u *url.URL, src string) (*DeviceReader, error) {
	req, _ := http.NewRequest("HEAD", src, nil)
	req.URL = u
	req.URL.Host = hostport
	req.Host = host

	res, err := c.Do(req)
	if err != nil {
		return nil, err
	}

	hr := &HTTPReader{
		hostport: hostport,
		host:     host,
		url:      u,
		src:      src,
		length:   res.ContentLength,
		c:        c,
	}

	r, err := bgzf.NewReader(hr, runtime.NumCPU())
	if err != nil {
		return nil, err
	}

	dev := &DeviceReader{size: 5372903424, cr: r, hr: hr}
	return dev, nil
}

func (r *DeviceReader) ReadAt(b []byte, offset int64) (n int, err error) {
	off := bgzf.Offset{
		File:  offset,
		Block: 0,
	}
	err = r.cr.Seek(off)
	if err != nil {
		return 0, err
	}

	return r.cr.Read(b)
}

func (r *DeviceReader) WriteAt(b []byte, offset int64) (int, error) {
	if debug_mode {
		fmt.Printf("http WriteAt %+v %d\n", b, offset)
	}
	return 0, nil
}

func (r *DeviceReader) Close() error {
	C.ext2fs_free(r.fs)
	return r.cr.Close()
}

func (r *HTTPReader) Seek(offset int64, whence int) (int64, error) {
	if debug_mode {
		fmt.Printf("http Seek %d %d\n", offset, whence)
	}
	switch whence {
	case os.SEEK_SET:
		r.start = offset
	case os.SEEK_CUR:
		r.start += offset
	case os.SEEK_END:
		r.start = r.length - offset
	default:
		return 0, fmt.Errorf("Seek not implemented for whence %d\n", whence)
	}
	return r.start, nil
}

func (r *HTTPReader) Read(b []byte) (n int, err error) {
	req, _ := http.NewRequest("GET", r.src, nil)
	req.URL = r.url
	req.URL.Host = r.hostport
	req.Host = r.host
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", r.start, len(b)))

	if debug_mode {
		fmt.Printf("http Read %+v\n", req)
	}

	res, err := r.c.Do(req)
	if err != nil {
		if debug_mode {
			fmt.Printf("http Read err %s\n", err.Error())
		}
		return 0, err
	}
	defer res.Body.Close()

	n, err = res.Body.Read(b)
	if err != nil {
		return n, err
	}

	r.start += int64(n)
	return n, nil
}

func copyImage(img string, dev_dst string, fetchaddrs []string) (err error) {
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
			if debug_mode {
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

			ndev, err := NewDevice(httpClient, net.JoinHostPort(host, port), host, u, src)
			if err != nil {
				return err
			}
			defer ndev.Close()

			n := nbd.Create(ndev, ndev.size)
			dev_src, err := n.Connect()
			if err != nil {
				if debug_mode {
					fmt.Printf("http err: %s\n", err)
					time.Sleep(600 * time.Second)
				}
				continue
			}

			if debug_mode {
				log.Printf("nbd device %s ready\n", dev_src)
			}
			go n.Handle()
			if debug_mode {
				fmt.Printf("handle nbd io\n")
			}

			err = copyFs(ndev, dev_dst, dev_src)
			if err != nil {
				if debug_mode {
					fmt.Printf("http err: %s\n", err)
					time.Sleep(600 * time.Second)
				}
				continue
			} else {
				return nil
			}
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

func copyFs(ndev *DeviceReader, dev_dst, dev_src string) error {
	fw, err := os.OpenFile(dev_dst, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fw.Close()

	fr, err := os.OpenFile(dev_src, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	defer fr.Close()

	blkstr := C.CString(dev_src)

	cmd := exec.Command("/bin/busybox", "fdisk", "-l", "-u", "/dev/nbd0")
	buf, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Printf("failed to get nbd status: %s\n", err.Error())
	} else {
		fmt.Printf("fdisk output is :%s\n", buf)
	}

	ret := C.ext2fs_open(blkstr, C.EXT2_FLAG_FORCE, 0, 0, C.unix_io_manager, &ndev.fs)
	if ret != 0 {
		if debug_mode {
			C.com_err("ext2fs_open", ret, "%s", blkstr)
		}
		return fmt.Errorf("ext2 error: %d\n", ret)
	}
	C.free(unsafe.Pointer(blkstr))

	return nil
}
