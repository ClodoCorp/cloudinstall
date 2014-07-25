package main

import (
	"compress/gzip"
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"time"
)

func copyImage(src string, dst string) (err error) {
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 5 * time.Minute}

	res, err := httpClient.Get(src)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	w, err := os.OpenFile(dst, os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer w.Close()

	r, err := gzip.NewReader(res.Body)
	if err != nil {
		return err
	}
	defer r.Close()

	_, err = io.Copy(w, r)
	if err != nil {
		return err
	}
	return nil
}
