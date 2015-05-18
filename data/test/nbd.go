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
	"runtime"
	"time"

	"github.com/vtolstov/cloudbootstrap/internal/code.google.com/p/biogo.bam/bgzf"

	"github.com/vtolstov/go-nbd"
)

var debug_mode bool = true

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
	if debug_mode {
		fmt.Printf("http ReadAt off: %d len: %d\n", offset, len(b))
	}

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
	req.Header.Add("Range", fmt.Sprintf("bytes=%d-%d", r.start, r.start+int64(len(b))))

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

func main() {
	httpTransport := &http.Transport{
		Dial:            (&net.Dialer{DualStack: true}).Dial,
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 5 * time.Minute}
	src := "http://85.143.208.10/vps/centos-6-x86_64"
	u, _ := url.Parse(src)
	host := "85.143.208.10"
	port := "80"
	ndev, err := NewDevice(httpClient, net.JoinHostPort(host, port), host, u, src)
	if err != nil {
		fmt.Printf("http err: %s\n", err)
		os.Exit(1)
	}
	defer ndev.Close()

	n := nbd.Create(ndev, ndev.size)
	dev_src, err := n.Connect()
	if err != nil {
		fmt.Printf("http err: %s\n", err)
		os.Exit(1)
	}

	if debug_mode {
		log.Printf("nbd device %s ready\n", dev_src)
	}
	go n.Handle()
	if debug_mode {
		fmt.Printf("handle nbd io\n")
	}

	select {}
}
