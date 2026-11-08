package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	hostNetworkDir = "/etc/systemd/network"
	// guestNetworkDir is relative to the container rootfs.
	guestNetworkDir = "etc/systemd/network"
	// 50, not 80: networkd uses the first matching file in lexical order,
	// and the shipped 80-container-ve.network would run a DHCP server on ve-*.
	filePrefix = "50-decnspawn-"
	// [0-9], not *: with *, the pattern for "web" would also match the
	// files of a container named "web-1".
	guestNetworkPattern = filePrefix + "eth[0-9].network"
)

func bridgeNetdevFile(bridge string) string {
	return filePrefix + bridge + ".netdev"
}

func bridgeNetworkFile(bridge string) string {
	return filePrefix + bridge + ".network"
}

func vethName(container string, index int) string {
	return fmt.Sprintf("ve-%s-%d", container, index)
}

func vethNetworkFile(container string, index int) string {
	return filePrefix + vethName(container, index) + ".network"
}

func vethNetworkPattern(container string) string {
	return filePrefix + "ve-" + container + "-[0-9].network"
}

func guestNetworkFile(index int) string {
	return fmt.Sprintf("%seth%d.network", filePrefix, index)
}

func renderBridgeNetdev(network Network) string {
	return fmt.Sprintf("[NetDev]\nName=%s\nKind=bridge\n", network.Name)
}

func renderBridgeNetwork(network Network) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Match]\nName=%s\n\n[Network]\n", network.Name)
	if network.Address != "" {
		fmt.Fprintf(&b, "Address=%s\n", network.Address)
	}
	if network.Masquerade {
		b.WriteString("IPMasquerade=ipv4\n")
	}
	b.WriteString("ConfigureWithoutCarrier=yes\n")
	return b.String()
}

func renderVethNetwork(container string, index int, bridge string) string {
	return fmt.Sprintf("[Match]\nName=%s\n\n[Network]\nBridge=%s\n", vethName(container, index), bridge)
}

func renderGuestNetwork(index int, attachment Attachment) string {
	var b strings.Builder
	fmt.Fprintf(&b, "[Match]\nName=eth%d\n\n[Network]\n", index)
	if attachment.IPv4Address != "" {
		fmt.Fprintf(&b, "Address=%s\n", attachment.IPv4Address)
	}
	if attachment.IPv4Gateway != "" {
		fmt.Fprintf(&b, "Gateway=%s\n", attachment.IPv4Gateway)
	}
	return b.String()
}

func bridgeFiles(network Network) map[string]string {
	return map[string]string{
		bridgeNetdevFile(network.Name):  renderBridgeNetdev(network),
		bridgeNetworkFile(network.Name): renderBridgeNetwork(network),
	}
}

func vethFiles(container Container) map[string]string {
	files := make(map[string]string)
	for i, attachment := range container.Networks {
		files[vethNetworkFile(container.Name, i)] = renderVethNetwork(container.Name, i, attachment.Bridge)
	}
	return files
}

func guestFiles(container Container) map[string]string {
	files := make(map[string]string)
	for i, attachment := range container.Networks {
		files[guestNetworkFile(i)] = renderGuestNetwork(i, attachment)
	}
	return files
}

func referencesBridge(content, bridge string) bool {
	for line := range strings.SplitSeq(content, "\n") {
		if strings.TrimSpace(line) == "Bridge="+bridge {
			return true
		}
	}
	return false
}

func checkBridgeOwnership(network Network) error {
	for name, want := range bridgeFiles(network) {
		if err := checkFileContent(filepath.Join(hostNetworkDir, name), want); err != nil {
			return err
		}
	}
	generated, err := pathExists(filepath.Join(hostNetworkDir, bridgeNetdevFile(network.Name)))
	if err != nil {
		return err
	}
	if generated {
		return nil
	}
	exists, err := interfaceExists(network.Name)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("interface %s exists but is not managed by decnspawn", network.Name)
	}
	return nil
}

// Differing content is an error, not overwritten: a bridge can be shared by
// several YAML files, and the last create would silently win.
func checkFileContent(path, want string) error {
	got, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if string(got) != want {
		return fmt.Errorf("%s differs from the YAML; destroy the containers using it or remove it by hand", path)
	}
	return nil
}

func writeBridge(network Network) error {
	return writeHostFiles("", bridgeFiles(network))
}

func writeVethNetworks(container Container) error {
	return writeHostFiles(vethNetworkPattern(container.Name), vethFiles(container))
}

func removeVethNetworks(container Container) error {
	return writeHostFiles(vethNetworkPattern(container.Name), nil)
}

// os.Root, not a joined path: the rootfs is writable from inside the
// container, and a symlink placed there must not redirect writes to the host.
func writeGuestNetworks(container Container) error {
	root, err := os.OpenRoot(rootfsPath(container.Name))
	if err != nil {
		return err
	}
	defer root.Close()
	return writeFiles(root, guestNetworkDir, guestNetworkPattern, guestFiles(container))
}

func writeHostFiles(stalePattern string, files map[string]string) error {
	if err := os.MkdirAll(hostNetworkDir, 0o755); err != nil {
		return err
	}
	root, err := os.OpenRoot(hostNetworkDir)
	if err != nil {
		return err
	}
	defer root.Close()
	return writeFiles(root, ".", stalePattern, files)
}

// writeFiles also removes stale files, not a separate step: destroy calls it
// with no files, so it removes exactly what create would have written.
func writeFiles(root *os.Root, dir, stalePattern string, files map[string]string) error {
	if err := root.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for name, content := range files {
		if err := root.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			return err
		}
	}
	if stalePattern == "" {
		return nil
	}
	stale, err := fs.Glob(root.FS(), filepath.Join(dir, stalePattern))
	if err != nil {
		return err
	}
	for _, path := range stale {
		if _, keep := files[filepath.Base(path)]; keep {
			continue
		}
		if err := root.Remove(path); err != nil {
			return err
		}
	}
	return nil
}

// bridgeInUse reports whether any remaining host veth .network, possibly
// generated from another YAML file, still attaches to the bridge
func bridgeInUse(bridge string) (bool, error) {
	paths, err := filepath.Glob(filepath.Join(hostNetworkDir, filePrefix+"ve-*.network"))
	if err != nil {
		return false, err
	}
	for _, path := range paths {
		content, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		if referencesBridge(string(content), bridge) {
			return true, nil
		}
	}
	return false, nil
}

func removeBridgeFiles(network Network) error {
	for name := range bridgeFiles(network) {
		if err := removeIfExists(filepath.Join(hostNetworkDir, name)); err != nil {
			return err
		}
	}
	return nil
}

// deleteBridge removes the interface itself: networkd leaves a bridge in
// place after its .netdev is gone and networkd is reloaded, and does not
// recreate it from a .netdev that is still there.
func deleteBridge(bridge string) (bool, error) {
	exists, err := interfaceExists(bridge)
	if err != nil || !exists {
		return false, err
	}
	return true, execute("networkctl", "delete", "--", bridge)
}

func interfaceExists(name string) (bool, error) {
	return pathExists(filepath.Join("/sys/class/net", name))
}

func reloadNetworkd() error {
	return execute("networkctl", "reload")
}
