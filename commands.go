package main

import "fmt"

func createAll(config Config) error {
	if err := checkCreate(config); err != nil {
		return err
	}
	for _, network := range config.Networks {
		if err := writeBridge(network); err != nil {
			return fmt.Errorf("network %s: %w", network.Name, err)
		}
	}
	for _, container := range config.Containers {
		cloned, err := createContainer(container)
		if err != nil {
			return fmt.Errorf("container %s: %w", container.Name, err)
		}
		if cloned {
			fmt.Printf("Container %s created\n", container.Name)
		} else {
			fmt.Printf("Container %s exists; generated files rewritten\n", container.Name)
		}
	}
	return reloadNetworkd()
}

// checkCreate inspects the host before anything is changed, so a create
// that would fail half way does not start at all
func checkCreate(config Config) error {
	for _, network := range config.Networks {
		if err := checkBridgeOwnership(network); err != nil {
			return err
		}
	}
	for _, container := range config.Containers {
		if err := checkBase(container.Base); err != nil {
			return err
		}
		if err := checkNotForeign(container.Name); err != nil {
			return err
		}
	}
	return nil
}

// createContainer writes the .nspawn first, not after the clone: it carries
// the marker, so a create interrupted later is resumed by the next one
// instead of being refused as a foreign directory.
func createContainer(container Container) (bool, error) {
	if err := writeNspawn(container); err != nil {
		return false, err
	}
	cloned, err := cloneRootfs(container)
	if err != nil {
		return false, err
	}
	if err := checkRootfs(container.Name); err != nil {
		return false, err
	}
	if err := writeGuestNetworks(container); err != nil {
		return false, err
	}
	return cloned, writeVethNetworks(container)
}

func startAll(config Config) error {
	for _, container := range config.Containers {
		if err := checkManaged(container.Name); err != nil {
			return err
		}
	}
	if err := checkBridgesUp(config.Networks); err != nil {
		return err
	}
	for _, container := range config.Containers {
		if err := startContainer(container.Name); err != nil {
			return err
		}
		fmt.Printf("Container %s started\n", container.Name)
	}
	return nil
}

func checkBridgesUp(networks []Network) error {
	for _, network := range networks {
		exists, err := interfaceExists(network.Name)
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("bridge %s does not exist; run create first", network.Name)
		}
	}
	return nil
}

func stopAll(config Config) error {
	for _, container := range config.Containers {
		if err := checkManaged(container.Name); err != nil {
			return err
		}
	}
	for _, container := range config.Containers {
		if err := stopContainer(container.Name); err != nil {
			return err
		}
		fmt.Printf("Container %s stopped\n", container.Name)
	}
	return nil
}

func destroyAll(config Config) error {
	for _, container := range config.Containers {
		if err := checkNotForeign(container.Name); err != nil {
			return err
		}
	}
	for _, container := range config.Containers {
		managed, err := destroyContainer(container)
		if err != nil {
			return fmt.Errorf("container %s: %w", container.Name, err)
		}
		if managed {
			fmt.Printf("Container %s destroyed\n", container.Name)
		} else {
			fmt.Printf("Container %s not found\n", container.Name)
		}
	}
	if err := removeUnusedBridges(config.Networks); err != nil {
		return err
	}
	return reloadNetworkd()
}

// destroyContainer reads the marker before machinectl remove, not after:
// remove deletes the .nspawn, and with it the proof of ownership. Without
// the marker only the rootfs-less leftovers remain, which are ours by prefix.
func destroyContainer(container Container) (bool, error) {
	managed, err := isManaged(container.Name)
	if err != nil {
		return false, err
	}
	if managed {
		if err := destroyManaged(container.Name); err != nil {
			return false, err
		}
	}
	return managed, removeVethNetworks(container)
}

func destroyManaged(name string) error {
	if err := stopContainer(name); err != nil {
		return err
	}
	if err := removeRootfs(name); err != nil {
		return err
	}
	return removeNspawn(name)
}

// removeUnusedBridges keeps a bridge that a container from another YAML
// file still attaches to. It deletes the interface before its files, not
// after the reload: networkd drops the masquerade entry of a bridge it still
// manages, and keeps it for good once the reload has made the bridge
// unmanaged, so a later bridge on the same prefix would be masqueraded.
func removeUnusedBridges(networks []Network) error {
	for _, network := range networks {
		inUse, err := bridgeInUse(network.Name)
		if err != nil {
			return err
		}
		if inUse {
			fmt.Printf("Network %s kept: still in use\n", network.Name)
			continue
		}
		if err := removeBridge(network); err != nil {
			return err
		}
	}
	return nil
}

func removeBridge(network Network) error {
	existed, err := deleteBridge(network.Name)
	if err != nil {
		return err
	}
	if err := removeBridgeFiles(network); err != nil {
		return err
	}
	if existed {
		fmt.Printf("Network %s destroyed\n", network.Name)
	} else {
		fmt.Printf("Network %s not found\n", network.Name)
	}
	return nil
}
