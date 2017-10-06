package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"time"
)

func (server *Server) System_ZfsCleanup(conn net.Conn) error {
	logger := NewLogger(server.getLogger(conn), "[ZfsMaintenance] ")
	err := server.sysPerformZfsMaintenance(logger)
	return err
}

func (server *Server) System_SnapshotsCleanup(conn net.Conn) error {
	logger := NewLogger(server.getLogger(conn), "[OrphanedSnapshots] ")
	err := server.sysRemoveOrphanedReleaseSnapshots(logger)
	return err
}

func (server *Server) System_NtpSync(conn net.Conn) error {
	logger := NewLogger(server.getLogger(conn), "[NtpSync] ")
	err := server.sysSyncNtp(logger)
	return err
}

// Cleanup any ZFS containers identified as stragglers.
func (server *Server) sysPerformZfsMaintenance(logger io.Writer) error {
	if lxcFs != "zfs" {
		return fmt.Errorf(`This command requires the LXC filesystem type to be "zfs", but instead found "%v"`, lxcFs)
	}

	deployLock.start()
	defer deployLock.finish()

	maintenanceScriptPath := "/tmp/zfs_maintenance.sh"

	err := ioutil.WriteFile(maintenanceScriptPath, []byte(ZFS_MAINTENANCE), 0777)
	if err != nil {
		fmt.Fprintf(logger, "Error writing maintenance script to %v: %v, operation aborted\n", maintenanceScriptPath, err)
		return err
	}

	e := Executor{logger}

	err = server.WithConfig(func(cfg *Config) error {
		for _, node := range cfg.Nodes {
			fmt.Fprintf(logger, "Starting ZFS maintenance for node=%v\n", node.Host)
			err = e.Run("rsync", "-azve", "ssh "+DEFAULT_SSH_PARAMETERS, maintenanceScriptPath, "root@"+node.Host+":"+maintenanceScriptPath)
			if err != nil {
				fmt.Fprintf(logger, "Error rsync'ing %v to %v: %v, node will be skipped", maintenanceScriptPath, node.Host, err)
				continue
			}
			sshArgs := append(defaultSshParametersList, "root@"+node.Host, maintenanceScriptPath)
			err = e.Run("ssh", sshArgs...)
			if err != nil {
				fmt.Fprintf(logger, "Error running %v on %v: %v", maintenanceScriptPath, node.Host, err)
				continue
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	err = e.Run(maintenanceScriptPath)
	return err
}

// Remove any orphaned release snapshot archives which are more than 2 hours old.
func (server *Server) sysRemoveOrphanedReleaseSnapshots(logger io.Writer) error {
	deployLock.start()
	defer deployLock.finish()

	e := Executor{logger}

	err := e.BashCmd(`sudo find /tmp -xdev -mmin +120 -size +25M -wholename '*_v*.tar.gz' -exec  rm -f {} \;`)

	return err
}

// Get all hostnames of the members of this ShipBuilder cluster.  Includes all nodes and load-balancers.
func (server *Server) GetClusterHosts() ([]string, error) {
	var clusterHosts []string
	err := server.WithConfig(func(cfg *Config) error {
		clusterHosts = cfg.LoadBalancers
		for _, node := range cfg.Nodes {
			clusterHosts = append(clusterHosts, node.Host)
		}
		return nil
	})
	return clusterHosts, err
}

func SyncNtpForHost(host string, logger io.Writer) error {
	logger = NewLogger(logger, "["+host+"] ")
	output, err := RemoteCommand(host, ntpSyncCommand)
	fmt.Fprintf(logger, "%v\n", output)
	return err
}

func (server *Server) sysSyncNtp(logger io.Writer) error {
	// sudo service ntp stop && sudo /usr/sbin/ntpdate 0.pool.ntp.org 1.pool.ntp.org time.apple.com time.windows.com && sudo service ntp start

	type SyncResult struct {
		host string
		err  error
	}

	clusterHosts, err := server.GetClusterHosts()
	if err != nil {
		return err
	}
	syncStep := make(chan SyncResult)

	for _, host := range clusterHosts {
		go func(host string) {
			c := make(chan error, 1)
			go func() { c <- SyncNtpForHost(host, logger) }()
			go func() {
				time.Sleep(NODE_SYNC_TIMEOUT_SECONDS * time.Second)
				c <- fmt.Errorf("Sync operation to host %q timed out after %v seconds", host, NODE_SYNC_TIMEOUT_SECONDS)
			}()
			// Block until chan has something, at which point syncStep will be notified.
			syncStep <- SyncResult{host, <-c}
		}(host)
	}

	// Special case (no SSH required): Run locally on SB server.
	e := Executor{NewLogger(logger, "[localhost] ")}
	err = e.BashCmd(ntpSyncCommand)
	syncResult := SyncResult{"localhost", err}

	failureCount := 0
	numSyncOperations := len(clusterHosts) + 1 // +1 for localhost.

	if syncResult.err == nil {
		fmt.Fprintf(logger, "[%v/%v] succeeded for host=%v\n", 1, numSyncOperations, syncResult.host)
	} else {
		fmt.Fprintf(logger, "[%v/%v] failed for host=%v error=%v\n", 1, numSyncOperations, syncResult.host, syncResult.err)
		failureCount++
	}

	for i, _ := range clusterHosts {
		syncResult = <-syncStep
		//syncResults := append(syncResults, syncResult)
		if syncResult.err == nil {
			fmt.Fprintf(logger, "[%v/%v] succeeded for host=%v\n", i+2, numSyncOperations, syncResult.host)
		} else {
			fmt.Fprintf(logger, "[%v/%v] failed for host=%v error=%v\n", i+2, numSyncOperations, syncResult.host, syncResult.err)
			failureCount++
		}
	}

	fmt.Fprintf(logger, "Cluster NTP Sync completed with %v successes and %v failures\n", numSyncOperations-failureCount, failureCount)

	if failureCount != 0 {
		return fmt.Errorf("Cluster NTP sync had %v failures", failureCount)
	}

	return nil
}
