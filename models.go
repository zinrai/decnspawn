package main

type Config struct {
	Networks   []Network   `yaml:"networks"`
	Containers []Container `yaml:"containers"`
}

type Network struct {
	Name       string `yaml:"name"`
	Address    string `yaml:"address"`
	Masquerade bool   `yaml:"masquerade"`
}

type Container struct {
	Name     string       `yaml:"name"`
	Base     string       `yaml:"base"`
	Networks []Attachment `yaml:"networks"`
}

type Attachment struct {
	Bridge      string `yaml:"bridge"`
	IPv4Address string `yaml:"ipv4_address"`
	IPv4Gateway string `yaml:"ipv4_gateway"`
}
