package main

type User struct {
	Name   string `yaml:"name"`
	Passwd string `yaml:"passwd"`
}

type Bootstrap struct {
	Name    string   `yaml:"name"`
	Arch    string   `yaml:"arch"`
	Fetch   []string `yaml:"fetch"`
	Version string   `yaml:"version"`
}

type CloudConfig struct {
	Users     []User    `yaml:"users,omitempty"`
	Bootstrap Bootstrap `yaml:"bootstrap,omitempty"`
}

type Ec2 struct {
	Timeout      int      `yaml:"timeout,omitempty"`
	MaxWait      int      `yaml:"max_wait,omitempty"`
	MetadataUrls []string `yaml:"metadata_urls,omitempty"`
}

type DataSource struct {
	Datasource struct {
		Ec2 Ec2 `yaml:"Ec2,omitempty"`
	} `yaml:"datasource"`
}
