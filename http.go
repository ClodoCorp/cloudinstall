package main

import (
	"crypto/tls"
	"io/ioutil"
	"net/http"
	"time"

	"gopkg.in/yaml.v1"
)

func getDataSource() (dataSource DataSource, err error) {
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 15 * time.Second}

	var res *http.Response
	var urlDataSource string
	var buffer []byte

	_, urlDataSource, err = cmdlineVar("cloud-config-url")
	if err != nil {
		return
	}

	res, err = httpClient.Get(urlDataSource)
	if err != nil {
		return
	}
	defer res.Body.Close()

	buffer, err = ioutil.ReadAll(res.Body)
	if err != nil {
		return
	}
	err = yaml.Unmarshal(buffer, &dataSource)
	if err != nil {
		return
	}

	return
}

func getCloudConfig(dataSource DataSource) (cloudConfig CloudConfig, err error) {
	httpTransport := &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	httpClient := &http.Client{Transport: httpTransport, Timeout: 60 * time.Second}
	var res *http.Response
	var buffer []byte

	for _, metadataUrl := range dataSource.Datasource.Ec2.MetadataUrls {
		res, err = httpClient.Get(metadataUrl + "&action=install")
		if err != nil {
			continue
		}
		defer res.Body.Close()

		buffer, err = ioutil.ReadAll(res.Body)
		if err != nil {
			continue
		}
		err = yaml.Unmarshal(buffer, &cloudConfig)
		if err != nil {
			continue
		}
		return cloudConfig, nil
	}
	return
}
