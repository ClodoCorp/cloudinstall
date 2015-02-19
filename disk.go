package main

// #cgo pkg-config: ext2fs com_err
// #include <ext2fs/ext2fs.h>
// #include <stdlib.h>
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

type httpReaderSeekerCloser struct {
	fmt string

	client *http.Client
	req    *http.Request

	fs C.ext2_filsys

	start   int64
	blksize int64
	length  int64

	hostport string
	host     string
	url      *url.URL
	src      string
}

func httpReaderSeekerCloserNew(client *http.Client, hostport string, host string, u *url.URL, src string) (*httpReaderSeekerCloser, error) {

	req, _ := http.NewRequest("HEAD", src, nil)
	req.URL = u
	req.URL.Host = hostport
	req.Host = host

	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	h := &httpReaderSeekerCloser{hostport: hostport, host: host, url: u, src: src, length: res.ContentLength, client: client, blksize: 5372903424}
	return h, nil
}

func (h *httpReaderSeekerCloser) ReadAt(b []byte, offset int64) (n int, err error) {
	req, _ := http.NewRequest("GET", h.src, nil)
	req.URL = h.url
	req.URL.Host = h.hostport
	req.Host = h.host
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", offset, len(b)))

	if debug_mode {
		fmt.Printf("http ReadAt %+v\n", req)
	}

	res, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	hr := httpReader(

	bgzfr, err := bgzf.NewReader(res.Body, runtime.NumCPU())
	if err != nil {
		if debug_mode {
			fmt.Printf("bgzfr err: %s\n", err.Error())
		}
		return 0, err
	}
	defer bgzfr.Close()

	n, err = bgzfr.Read(b)
	if err != nil {
		return n, err
	}
	h.start += int64(n)
	return n, nil

	return 0, nil
}

func (h *httpReaderSeekerCloser) WriteAt(b []byte, offset int64) (int, error) {
	if debug_mode {
		fmt.Printf("http WriteAt %+v %d\n", b, offset)
	}
	return 0, nil
}

func (h *httpReaderSeekerCloser) Seek(offset int64, whence int) (int64, error) {
	if debug_mode {
		fmt.Printf("http Seek %d %d\n", offset, whence)
	}
	switch whence {
	case os.SEEK_SET:
		h.start = offset
	case os.SEEK_CUR:
		h.start += offset
	case os.SEEK_END:
		h.start = h.length - offset
	default:
		return 0, fmt.Errorf("Seek not implemented for whence %d\n", whence)
	}
	return h.start, nil
}

func (h *httpReaderSeekerCloser) Close() error {
	if debug_mode {
		fmt.Printf("http Close\n")
	}
	C.ext2fs_free(h.fs)
	return nil
}

func (h *httpReaderSeekerCloser) Read(b []byte) (n int, err error) {
	req, _ := http.NewRequest("GET", h.src, nil)
	req.URL = h.url
	req.URL.Host = h.hostport
	req.Host = h.host
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", h.start, len(b)))

	if debug_mode {
		fmt.Printf("http Read %+v\n", req)
	}

	res, err := h.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()

	bgzfr, err := bgzf.NewReader(res.Body, runtime.NumCPU())
	if err != nil {
		return 0, err
	}
	defer bgzfr.Close()

	n, err = bgzfr.Read(b)
	if err != nil {
		return n, err
	}
	h.start += int64(n)
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

			hr, err := httpReaderSeekerCloserNew(httpClient, net.JoinHostPort(addr.String(), port), host, u, src)
			if err != nil {
				if debug_mode {
					fmt.Printf("http err: %s\n", err)
					time.Sleep(600 * time.Second)
				}
				continue
			}
			return hr.copyFs(dev_dst)
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

func (h *httpReaderSeekerCloser) copyFs(dev_dst string) error {

	fw, err := os.OpenFile(dev_dst, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer fw.Close()

	n := nbd.Create(h, h.blksize) //need heuristic
	dev_src, err := n.Connect()
	if err != nil {
		return err
	}

	log.Printf("nbd device %s ready\n", dev_src)

	go n.Handle()

	fr, err := os.OpenFile(dev_src, os.O_RDONLY, 0600)
	if err != nil {
		return err
	}
	defer fr.Close()

	blkstr := C.CString(dev_src)

	cmd := exec.Command("/bin/busybox", "fdisk", "-l", "-u", "/dev/nbd0")
	err = cmd.Run()
	if err != nil {
		fmt.Printf("failed to get nbd status: %s\n", err.Error())
	}

	if debug_mode {
		fmt.Printf("open ext2 superblock\n")
	}
	ret := C.ext2fs_open(blkstr, C.EXT2_FLAG_FORCE, 0, 0, C.unix_io_manager, &h.fs)
	if ret != 0 {
		if debug_mode {
			fmt.Printf("open ext2 superblock fail %d\n", ret)
		}
		return fmt.Errorf("ext2 error: %d\n", ret)
	}
	C.free(unsafe.Pointer(blkstr))

	if debug_mode {
		fmt.Printf("handle nbd io\n")
	}

	return nil
}
