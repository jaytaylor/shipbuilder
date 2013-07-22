package main

import (
	"fmt"
	"net"
	"strings"
)

func (this *Server) Node_Add(conn net.Conn, addresses []string) error {
	addresses = replaceLocalhostWithSystemIp(&addresses)

	titleLogger, dimLogger := this.getTitleAndDimLoggers(conn)

	fmt.Fprintf(titleLogger, "=== Adding Nodes\n\n")

	e := Executor{dimLogger}

	return this.WithPersistentConfig(func(cfg *Config) error {
		for _, addAddress := range addresses {
			if len(addAddress) == 0 {
				continue
			}
			found := false
			for _, node := range cfg.Nodes {
				if strings.ToLower(node.Host) == strings.ToLower(addAddress) {
					fmt.Fprintf(dimLogger, "Node already exists: %v\n", addAddress)
					found = true
					break
				}
			}
			if !found {
				fmt.Fprintf(dimLogger, "Transmitting base LXC container image to node: %v\n", addAddress)
				err := e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+addAddress,
					"sudo", "test", "-e", "/var/lib/lxc/base", "&&",
					"echo", "not creating base image, already exists",
					"||",
					"sudo", "lxc-create", "-n", "base", "-B", "btrfs", "-t", "ubuntu",
				)
				if err != nil {
					return err
				}
				// Rsync the base container over.
				err = e.Run("sudo", "rsync",
					"--recursive",
					"--links",
					"--perms",
					"--times",
					"--devices",
					"--specials",
					"--hard-links",
					"--acls",
					"--delete",
					"--xattrs",
					"--numeric-ids",
					"-e", "ssh -o 'StrictHostKeyChecking no' -o 'BatchMode yes'",
					"/var/lib/lxc/base/rootfs/",
					"root@"+addAddress+":/var/lib/lxc/base/rootfs/",
				)
				if err != nil {
					return err
				}
				fmt.Fprintf(dimLogger, "Adding node: %v\n", addAddress)
				cfg.Nodes = append(cfg.Nodes, &Node{addAddress})
			}
		}
		return nil
	})
}

func (this *Server) Node_List(conn net.Conn) error {
	titleLogger, dimLogger := this.getTitleAndDimLoggers(conn)

	fmt.Fprintf(titleLogger, "=== System Nodes\n\n")

	return this.WithConfig(func(cfg *Config) error {
		for _, node := range cfg.Nodes {
			nodeStatus := this.getNodeStatus(node)
			if nodeStatus.Err == nil {
				fmt.Fprintf(dimLogger, "%v (%vMB free)\n", node.Host, nodeStatus.FreeMemoryMb)
				for _, application := range nodeStatus.Containers {
					fmt.Fprintf(dimLogger, "    `- %v\n", application)
				}
			} else {
				fmt.Fprintf(dimLogger, "%v (unknown status: %v)\n", node.Host, nodeStatus.Err)
			}

		}
		return nil
	})
}

func (this *Server) Node_Remove(conn net.Conn, addresses []string) error {
	addresses = replaceLocalhostWithSystemIp(&addresses)

	titleLogger, dimLogger := this.getTitleAndDimLoggers(conn)

	fmt.Fprintf(titleLogger, "=== Removing Nodes\n\n")

	return this.WithPersistentConfig(func(cfg *Config) error {
		nNodes := []*Node{}
		for _, node := range cfg.Nodes {
			keep := true
			for _, removeAddress := range addresses {
				if strings.ToLower(removeAddress) == strings.ToLower(node.Host) {
					fmt.Fprintf(dimLogger, "Removing node: %v\n", removeAddress)
					keep = false
					break
				}
			}
			if keep {
				nNodes = append(nNodes, node)
			}
		}
		cfg.Nodes = nNodes
		return nil
	})
}
