# decnspawn

A declarative CLI tool for managing systemd-nspawn containers and their networks using YAML configuration.

## Overview

`decnspawn` creates, starts, stops and destroys the systemd-nspawn containers and host bridges defined in a YAML file. It is the systemd-nspawn counterpart of [zinrai/declxc](https://github.com/zinrai/declxc).

Where LXC keeps a container's network in one config file, systemd-nspawn spreads it over the `.nspawn` file, the host's systemd-networkd and the container's systemd-networkd. `decnspawn` generates all of them from the YAML file, in the places systemd expects.

You prepare a base rootfs by hand; `decnspawn` clones each container from it and removes only what it created. The base itself is never modified or removed.

Provisioning inside the container (users, packages, services) is out of scope; run a configuration management tool against the created containers instead.

## Requirements

- Root privileges
- `systemd-container` (`systemd-nspawn`, `machinectl`)
- `systemd-networkd` enabled on the host. It manages only the interfaces its `.network` files match, so it coexists with ifupdown managing the physical NIC.
- A base rootfs under `/var/lib/machines/`

Debian GNU/Linux:

```bash
$ sudo apt install systemd-container debootstrap
$ sudo systemctl enable --now systemd-networkd
```

### Base rootfs

The base must:

- have `systemd` and `dbus`, so that the container boots and `machinectl shell` and `machinectl login` work
- have `systemd-networkd` enabled, which applies the generated addresses
- have an empty `/etc/machine-id` and no `/var/lib/dbus/machine-id` file, so that each clone gets its own machine ID. The `dbus` package writes the latter, and systemd copies it into the machine ID when `/etc/machine-id` is empty.
- have no `/etc/hostname`, so that each clone takes its container name as hostname
- have no SSH host keys if `openssh-server` is installed, so that clones do not share them

Example for Debian trixie:

```bash
$ sudo debootstrap --include=dbus trixie /var/lib/machines/trixie https://deb.debian.org/debian
$ sudo systemd-nspawn -q -D /var/lib/machines/trixie systemctl enable systemd-networkd
$ sudo truncate -s 0 /var/lib/machines/trixie/etc/machine-id
$ sudo rm -f /var/lib/machines/trixie/var/lib/dbus/machine-id
$ sudo rm -f /var/lib/machines/trixie/etc/hostname
```

## Usage

Create networks and containers:

```bash
$ sudo decnspawn create -f containers.yaml
```

Start containers:

```bash
$ sudo decnspawn start -f containers.yaml
```

Stop containers:

```bash
$ sudo decnspawn stop -f containers.yaml
```

Destroy containers and networks:

```bash
$ sudo decnspawn destroy -f containers.yaml
```

Log in to a running container:

```bash
$ sudo machinectl shell web1
```

## YAML Configuration

Example: `examples/containers.yaml`

```yaml
networks:
  - name: br0
    address: 192.168.100.1/24
    masquerade: true

containers:
  - name: web1
    base: trixie
    networks:
      - bridge: br0
        ipv4_address: 192.168.100.10/24
        ipv4_gateway: 192.168.100.1
```

Unknown keys are rejected. IPv6 is not supported.

### networks

| Option     | Description                                             | Required |
|------------|---------------------------------------------------------|----------|
| name       | Bridge name: an interface name, at most 15 characters   | Yes      |
| address    | Host address on the bridge, in CIDR notation            | No       |
| masquerade | Masquerade traffic from the bridge (`IPMasquerade=ipv4`) | No (default: false) |

With `masquerade: true`, systemd-networkd also enables IPv4 forwarding for the whole host. `destroy` leaves it enabled, since it cannot tell whether anything else relies on it.

### containers

| Option   | Description                                                      | Required |
|----------|------------------------------------------------------------------|----------|
| name     | Container name: a hostname, at most 10 characters                | Yes      |
| base     | Base rootfs to clone, `/var/lib/machines/<base>`                 | Yes      |
| networks | Interfaces, at most 10. The n-th one is `eth<n>` in the container | No       |

The name limit keeps the host-side interface `ve-<name>-<n>` within the kernel's 15 characters. A container cannot share its name with any `base` in the file.

### containers[].networks

| Option       | Description                                  | Required |
|--------------|----------------------------------------------|----------|
| bridge       | A bridge defined in `networks`               | Yes      |
| ipv4_address | Container address, in CIDR notation          | No       |
| ipv4_gateway | Default gateway                              | No       |

## How It Works

### Generated files

For each network:

- `/etc/systemd/network/50-decnspawn-<bridge>.netdev`
- `/etc/systemd/network/50-decnspawn-<bridge>.network`

For each container:

- `/var/lib/machines/<name>/`, cloned from the base with `machinectl clone`
- `/etc/systemd/nspawn/<name>.nspawn`, starting with `# Managed by decnspawn`
- `/etc/systemd/network/50-decnspawn-ve-<name>-<n>.network`, attaching the host side of each interface to its bridge
- `/var/lib/machines/<name>/etc/systemd/network/50-decnspawn-eth<n>.network`, configuring each interface inside the container

The `50-` prefix makes these files match before the shipped `80-container-ve.network`, which would otherwise run a DHCP server on the interfaces.

### create

`create` checks the YAML file and the host first, and changes nothing if any check fails. It refuses:

- a base without `/var/lib/machines/<base>/usr/lib/os-release`
- a base that breaks the requirements on machine ID, hostname or SSH host keys above
- an existing `/var/lib/machines/<name>` whose `.nspawn` lacks the `# Managed by decnspawn` line
- an existing interface named after a bridge that `decnspawn` did not generate
- existing bridge files that differ from the YAML file

It then writes the files above and reloads systemd-networkd. Containers created earlier are not cloned again, but their generated files are rewritten from the YAML file, and the files of interfaces removed from it are deleted. Changes inside a container take effect when it restarts. `create` is safe to run again, including after it failed half way.

### start and stop

`start` checks that the bridges exist, then runs `systemctl start systemd-nspawn@<name>`, which waits until the container has booted (`NotifyReady=yes`). `stop` runs `systemctl stop systemd-nspawn@<name>`, which waits until the container is down. Both refuse containers not created by `decnspawn`.

### destroy

`destroy` stops each container, removes its rootfs with `machinectl remove` and deletes its generated files. It refuses a container directory whose `.nspawn` lacks the marker, and treats a container that no longer exists as already destroyed, so it is safe to run again.

A bridge is removed only when no container attaches to it any more, together with its masquerade rule. Several YAML files can therefore share a bridge as long as they define it identically: it stays until the last container using it is destroyed.

## License

This project is licensed under the [MIT License](./LICENSE).
