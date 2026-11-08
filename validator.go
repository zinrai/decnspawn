package main

import (
	"fmt"
	"net/netip"
	"regexp"
)

const (
	// maxIfnameLen is the kernel's interface name limit (IFNAMSIZ - 1).
	maxIfnameLen = 15
	// maxAttachments keeps the index in ve-<name>-<n> to one digit.
	maxAttachments = 10
	// maxContainerNameLen leaves room for "ve-", "-" and a one-digit index.
	maxContainerNameLen = maxIfnameLen - len("ve--0")
)

var (
	// Hostname labels, not whatever machinectl accepts: clone takes names
	// such as web_1 or web-, and only start then fails on them. The pattern
	// also keeps '/' out of the paths built from the name.
	containerNamePattern = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*$`)
	// No '/': the name is part of a file path. No ':' or whitespace:
	// networkd ignores such a .netdev without failing the reload.
	bridgeNamePattern = regexp.MustCompile(`^[^/:\s]+$`)
)

// Validate checks the whole configuration without touching the host
func Validate(config Config) error {
	bridges, err := validateNetworks(config.Networks)
	if err != nil {
		return err
	}
	return validateContainers(config.Containers, bridges)
}

func validateNetworks(networks []Network) (map[string]bool, error) {
	bridges := make(map[string]bool)
	for _, network := range networks {
		if err := validateNetwork(network); err != nil {
			return nil, err
		}
		if bridges[network.Name] {
			return nil, fmt.Errorf("duplicate network name: %s", network.Name)
		}
		bridges[network.Name] = true
	}
	return bridges, nil
}

func validateNetwork(network Network) error {
	// "." and "..": networkd ignores them like the characters above.
	if !bridgeNamePattern.MatchString(network.Name) || network.Name == "." || network.Name == ".." {
		return fmt.Errorf("network name %q is not a valid interface name", network.Name)
	}
	if len(network.Name) > maxIfnameLen {
		return fmt.Errorf("network name %q is longer than %d characters", network.Name, maxIfnameLen)
	}
	if network.Address != "" {
		return checkIPv4Prefix(fmt.Sprintf("networks[%s].address", network.Name), network.Address)
	}
	return nil
}

func validateContainers(containers []Container, bridges map[string]bool) error {
	bases := make(map[string]bool)
	for _, container := range containers {
		bases[container.Base] = true
	}

	names := make(map[string]bool)
	for _, container := range containers {
		if err := validateContainer(container, bridges); err != nil {
			return err
		}
		// A container named after a base would let destroy remove the base.
		if bases[container.Name] {
			return fmt.Errorf("container name %q is also used as a base", container.Name)
		}
		if names[container.Name] {
			return fmt.Errorf("duplicate container name: %s", container.Name)
		}
		names[container.Name] = true
	}
	return nil
}

func validateContainer(container Container, bridges map[string]bool) error {
	if !containerNamePattern.MatchString(container.Name) {
		return fmt.Errorf("container name %q must be a hostname: letters, digits, inner '-' and '.'", container.Name)
	}
	if len(container.Name) > maxContainerNameLen {
		return fmt.Errorf("container name %q is longer than %d characters: host interface ve-<name>-<n> must fit in %d", container.Name, maxContainerNameLen, maxIfnameLen)
	}
	if len(container.Networks) > maxAttachments {
		return fmt.Errorf("container %s has more than %d networks", container.Name, maxAttachments)
	}
	for i, attachment := range container.Networks {
		if err := validateAttachment(fmt.Sprintf("containers[%s].networks[%d]", container.Name, i), attachment, bridges); err != nil {
			return err
		}
	}
	return nil
}

func validateAttachment(path string, attachment Attachment, bridges map[string]bool) error {
	if !bridges[attachment.Bridge] {
		return fmt.Errorf("%s.bridge: %q is not defined in networks", path, attachment.Bridge)
	}
	if attachment.IPv4Address != "" {
		if err := checkIPv4Prefix(path+".ipv4_address", attachment.IPv4Address); err != nil {
			return err
		}
	}
	if attachment.IPv4Gateway != "" {
		return checkIPv4Addr(path+".ipv4_gateway", attachment.IPv4Gateway)
	}
	return nil
}

func checkIPv4Prefix(path, value string) error {
	prefix, err := netip.ParsePrefix(value)
	if err != nil || !prefix.Addr().Is4() {
		return fmt.Errorf("%s: %q is not an IPv4 address in CIDR notation", path, value)
	}
	return nil
}

func checkIPv4Addr(path, value string) error {
	addr, err := netip.ParseAddr(value)
	if err != nil || !addr.Is4() {
		return fmt.Errorf("%s: %q is not an IPv4 address", path, value)
	}
	return nil
}
