package main

import (
	"strings"
	"testing"
)

func validConfig() Config {
	return Config{
		Networks: []Network{{Name: "br0", Address: "192.168.100.1/24", Masquerade: true}},
		Containers: []Container{{
			Name: "web1",
			Base: "trixie",
			Networks: []Attachment{{
				Bridge:      "br0",
				IPv4Address: "192.168.100.10/24",
				IPv4Gateway: "192.168.100.1",
			}},
		}},
	}
}

func attachments(n int) []Attachment {
	list := make([]Attachment, n)
	for i := range list {
		list[i] = Attachment{Bridge: "br0"}
	}
	return list
}

// renameBridge renames the bridge together with its reference, so that only
// the name itself can be what is rejected
func renameBridge(c *Config, name string) {
	c.Networks[0].Name = name
	c.Containers[0].Networks[0].Bridge = name
}

func assertRejected(t *testing.T, mutate func(*Config)) {
	t.Helper()
	config := validConfig()
	mutate(&config)
	if err := Validate(config); err == nil {
		t.Fatal("accepted")
	}
}

func TestExampleIsValid(t *testing.T) {
	if _, err := ParseFile("examples/containers.yaml"); err != nil {
		t.Fatal(err)
	}
}

func TestLongestAllowedNameFitsInterfaceLimit(t *testing.T) {
	config := validConfig()
	config.Containers[0].Name = strings.Repeat("a", maxContainerNameLen)
	config.Containers[0].Networks = attachments(maxAttachments)
	if err := Validate(config); err != nil {
		t.Fatal(err)
	}
	if name := vethName(config.Containers[0].Name, maxAttachments-1); len(name) > maxIfnameLen {
		t.Fatalf("%s is longer than %d characters", name, maxIfnameLen)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"container name longer than 10 characters": func(c *Config) { c.Containers[0].Name = "abcdefghijk" },
		"container named after a base":             func(c *Config) { c.Containers = append(c.Containers, Container{Name: "trixie", Base: "bookworm"}) },
		"bridge not defined in networks":           func(c *Config) { c.Containers[0].Networks[0].Bridge = "br1" },
		"more than 10 networks":                    func(c *Config) { c.Containers[0].Networks = attachments(maxAttachments + 1) },
		"address without prefix length":            func(c *Config) { c.Containers[0].Networks[0].IPv4Address = "192.168.100.10" },
		"slash in container name":                  func(c *Config) { c.Containers[0].Name = "../web1" },
		"network name networkd ignores":            func(c *Config) { renameBridge(c, "br 0") },
		"network name ..":                          func(c *Config) { renameBridge(c, "..") },
		"IPv6 address":                             func(c *Config) { c.Containers[0].Networks[0].IPv4Address = "fd00::10/64" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) { assertRejected(t, mutate) })
	}
}
