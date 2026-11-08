package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	machinesDir = "/var/lib/machines"
	nspawnDir   = "/etc/systemd/nspawn"
	// managedMarker must be the first line of every generated .nspawn:
	// every subcommand refuses to touch a container without it.
	managedMarker = "# Managed by decnspawn"
)

func rootfsPath(name string) string {
	return filepath.Join(machinesDir, name)
}

func nspawnPath(name string) string {
	return filepath.Join(nspawnDir, name+".nspawn")
}

// renderNspawn attaches every interface with VirtualEthernetExtra=, not the
// primary one with Bridge=: one way of wiring keeps a single code path.
// NotifyReady=yes, not the default: without it systemctl start returns
// before the container has booted, and its bus is not yet reachable.
// Private=yes although VirtualEthernetExtra= implies it: a container
// without networks would otherwise share the host's interfaces.
func renderNspawn(container Container) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n[Exec]\nBoot=yes\nNotifyReady=yes\n\n[Network]\nPrivate=yes\nVirtualEthernet=no\n", managedMarker)
	for i := range container.Networks {
		fmt.Fprintf(&b, "VirtualEthernetExtra=%s:eth%d\n", vethName(container.Name, i), i)
	}
	return b.String()
}

func isManaged(name string) (bool, error) {
	data, err := os.ReadFile(nspawnPath(name))
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	first, _, _ := strings.Cut(string(data), "\n")
	return first == managedMarker, nil
}

func checkNotForeign(name string) error {
	exists, err := pathExists(rootfsPath(name))
	if err != nil || !exists {
		return err
	}
	managed, err := isManaged(name)
	if err != nil {
		return err
	}
	if !managed {
		return fmt.Errorf("%s exists but is not managed by decnspawn", rootfsPath(name))
	}
	return nil
}

func checkManaged(name string) error {
	managed, err := isManaged(name)
	if err != nil {
		return err
	}
	if !managed {
		return fmt.Errorf("container %s is not managed by decnspawn; run create first", name)
	}
	return checkRootfs(name)
}

func checkBase(base string) error {
	if err := requireOSRelease(base, "base "+base+" not found"); err != nil {
		return err
	}
	root, err := os.OpenRoot(rootfsPath(base))
	if err != nil {
		return err
	}
	defer root.Close()
	if err := checkBaseIdentity(root); err != nil {
		return fmt.Errorf("base %s: %w", base, err)
	}
	return nil
}

// checkBaseIdentity refuses a base whose clones would share an identity:
// they boot fine, and the duplicates surface only later.
func checkBaseIdentity(root *os.Root) error {
	id, err := root.ReadFile("etc/machine-id")
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if len(strings.TrimSpace(string(id))) > 0 {
		return fmt.Errorf("etc/machine-id is not empty; every clone would share it")
	}
	// Only a regular file: a symlink to /etc/machine-id carries no ID of its own.
	info, err := root.Lstat("var/lib/dbus/machine-id")
	if err == nil && info.Mode().IsRegular() {
		return fmt.Errorf("var/lib/dbus/machine-id exists; systemd copies it into the machine ID of every clone")
	}
	if _, err := root.Lstat("etc/hostname"); err == nil {
		return fmt.Errorf("etc/hostname exists; every clone would take it instead of its container name")
	}
	keys, err := fs.Glob(root.FS(), "etc/ssh/ssh_host_*_key")
	if err != nil {
		return err
	}
	if len(keys) > 0 {
		return fmt.Errorf("SSH host keys exist in etc/ssh; every clone would share them")
	}
	return nil
}

// checkRootfs catches a clone that was interrupted half way
func checkRootfs(name string) error {
	return requireOSRelease(name, "rootfs of "+name+" is incomplete")
}

func requireOSRelease(name, problem string) error {
	path := filepath.Join(rootfsPath(name), "usr/lib/os-release")
	exists, err := pathExists(path)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%s: %s does not exist", problem, path)
	}
	return nil
}

func writeNspawn(container Container) error {
	if err := os.MkdirAll(nspawnDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(nspawnPath(container.Name), []byte(renderNspawn(container)), 0o644)
}

// cloneRootfs relies on machinectl clone keeping an existing .nspawn of the
// new name, so a .nspawn shipped with the base cannot replace ours.
func cloneRootfs(container Container) (bool, error) {
	exists, err := pathExists(rootfsPath(container.Name))
	if err != nil || exists {
		return false, err
	}
	return true, execute("machinectl", "clone", "--", container.Base, container.Name)
}

func removeRootfs(name string) error {
	exists, err := pathExists(rootfsPath(name))
	if err != nil || !exists {
		return err
	}
	return execute("machinectl", "remove", "--", name)
}

// machinectl remove usually deletes the .nspawn too; this covers the rest.
func removeNspawn(name string) error {
	return removeIfExists(nspawnPath(name))
}

func startContainer(name string) error {
	return execute("systemctl", "start", "systemd-nspawn@"+name)
}

// stopContainer uses systemctl stop, not machinectl poweroff: poweroff
// returns before the container is down, and remove would then fail as busy.
func stopContainer(name string) error {
	return execute("systemctl", "stop", "systemd-nspawn@"+name)
}

func execute(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}
