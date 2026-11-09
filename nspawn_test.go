package main

import (
	"os"
	"path/filepath"
	"testing"
)

// cleanBase lays out the identity files of a base built as the README says
func cleanBase(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	writeTree(t, dir, map[string]string{"etc/machine-id": "", "etc/ssh/ssh_config": ""})
	return dir
}

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func assertBaseRejected(t *testing.T, files map[string]string) {
	t.Helper()
	dir := cleanBase(t)
	writeTree(t, dir, files)
	if err := checkBaseIdentity(openRoot(t, dir)); err == nil {
		t.Fatal("accepted")
	}
}

func TestCleanBaseIsAccepted(t *testing.T) {
	if err := checkBaseIdentity(openRoot(t, cleanBase(t))); err != nil {
		t.Fatal(err)
	}
}

func TestBaseWithoutAMachineIDIsAccepted(t *testing.T) {
	for name, content := range map[string]string{"empty": "", "uninitialized": "uninitialized\n"} {
		t.Run(name, func(t *testing.T) {
			dir := cleanBase(t)
			writeTree(t, dir, map[string]string{"etc/machine-id": content})
			if err := checkBaseIdentity(openRoot(t, dir)); err != nil {
				t.Fatal(err)
			}
		})
	}
	t.Run("missing", func(t *testing.T) {
		dir := cleanBase(t)
		if err := os.Remove(filepath.Join(dir, "etc/machine-id")); err != nil {
			t.Fatal(err)
		}
		if err := checkBaseIdentity(openRoot(t, dir)); err != nil {
			t.Fatal(err)
		}
	})
}

func TestBaseSharingAnIdentityIsRejected(t *testing.T) {
	cases := map[string]map[string]string{
		"machine ID":       {"etc/machine-id": "73cb569738634dd882ede2ba9e734538\n"},
		"D-Bus machine ID": {"var/lib/dbus/machine-id": "73cb569738634dd882ede2ba9e734538\n"},
		"hostname":         {"etc/hostname": "trixie\n"},
		"SSH host key":     {"etc/ssh/ssh_host_ed25519_key": "key"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) { assertBaseRejected(t, files) })
	}
}
