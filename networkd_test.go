package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func openRoot(t *testing.T, dir string) *os.Root {
	t.Helper()
	root, err := os.OpenRoot(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { root.Close() })
	return root
}

func listDir(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	return names
}

func writeVethFiles(t *testing.T, root *os.Root, container Container) {
	t.Helper()
	if err := writeFiles(root, ".", vethNetworkPattern(container.Name), vethFiles(container)); err != nil {
		t.Fatal(err)
	}
}

func TestVethNetworkSortsBeforeShippedContainerVe(t *testing.T) {
	if name := vethNetworkFile("zzzzzzzzzz", 9); name >= "80-container-ve.network" {
		t.Fatalf("%s does not sort before 80-container-ve.network", name)
	}
}

func TestVethPatternMatchesOnlyItsOwnContainer(t *testing.T) {
	pattern := vethNetworkPattern("web")
	if ok, _ := filepath.Match(pattern, vethNetworkFile("web", 9)); !ok {
		t.Fatal("pattern misses the container's own file")
	}
	if ok, _ := filepath.Match(pattern, vethNetworkFile("web-1", 0)); ok {
		t.Fatal("pattern for web matches a file of web-1")
	}
}

func TestReferencesBridgeMatchesWholeName(t *testing.T) {
	content := renderVethNetwork("web1", 0, "br01")
	if referencesBridge(content, "br0") {
		t.Fatal("br01 counted as a reference to br0")
	}
	if !referencesBridge(content, "br01") {
		t.Fatal("reference to br01 missed")
	}
}

func TestRenderedNspawnIsManaged(t *testing.T) {
	first, _, _ := strings.Cut(renderNspawn(validConfig().Containers[0]), "\n")
	if first != managedMarker {
		t.Fatalf("first line is %q, not the marker", first)
	}
}

func TestWriteFilesRemovesDroppedNetworks(t *testing.T) {
	dir := t.TempDir()
	root := openRoot(t, dir)
	web := Container{Name: "web", Networks: attachments(2)}
	other := Container{Name: "web-1", Networks: attachments(1)}

	writeVethFiles(t, root, other)
	writeVethFiles(t, root, web)
	web.Networks = attachments(1)
	writeVethFiles(t, root, web)

	want := []string{vethNetworkFile("web", 0), vethNetworkFile("web-1", 0)}
	if got := listDir(t, dir); !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWriteFilesDoesNotFollowSymlinkOutOfRootfs(t *testing.T) {
	rootfs := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(rootfs, "etc")); err != nil {
		t.Fatal(err)
	}

	err := writeFiles(openRoot(t, rootfs), guestNetworkDir, guestNetworkPattern, guestFiles(validConfig().Containers[0]))
	if err == nil {
		t.Fatal("wrote through a symlink leaving the rootfs")
	}
	if got := listDir(t, outside); len(got) != 0 {
		t.Fatalf("files written outside the rootfs: %v", got)
	}
}
