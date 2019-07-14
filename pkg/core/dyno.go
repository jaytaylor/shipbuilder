package core

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jaytaylor/shipbuilder/pkg/appender"
	"github.com/jaytaylor/shipbuilder/pkg/stringsutil"
	log "github.com/sirupsen/logrus"
)

const (
	DYNO_DELIMITER     = "-"
	DYNO_STATE_RUNNING = "running"
	DYNO_STATE_STOPPED = "stopped"
)

var dynoPortTracker = DynoPortTracker{allocations: map[string][]int{}, lock: sync.Mutex{}}

type Dyno struct {
	Host, Container, Application, Process, Version, Port, State string
	VersionNumber, PortNumber                                   int
}

type NodeStatusRunning struct {
	status  NodeStatus
	running bool
}

type NodeStatuses []NodeStatusRunning

type DynoGenerator struct {
	server      *Server
	statuses    []NodeStatusRunning
	position    int
	application string
	version     string
	usedPorts   []int
}

type DynoPortTracker struct {
	allocations map[string][]int
	lock        sync.Mutex
}

func (dyno *Dyno) Info() string {
	return fmt.Sprintf("host=%v app=%v version=%v proc=%v port=%v state=%v", dyno.Host, dyno.Application, dyno.Version, dyno.Process, dyno.Port, dyno.State)
}

func (dyno *Dyno) Shutdown(e *Executor) error {
	fmt.Fprintf(e.Logger, "Shutting down dyno: %v\n", dyno.Info())
	sshHost := "root@" + dyno.Host
	if err := e.SyncContainerScripts(sshHost + ":/tmp/"); err != nil {
		fmt.Fprintf(e.Logger, "Warning: failed to sync container control scripts to %q (will continue shutdown attempt despite this): %s\n", sshHost, err)
	}
	if dyno.State == DYNO_STATE_RUNNING {
		// Shutdown then destroy.
		return e.Run("ssh", sshHost, "/tmp/shutdown_container.py", dyno.Container)
	} else {
		// Destroy only.
		return e.Run("ssh", sshHost, "/tmp/shutdown_container.py", dyno.Container, "destroy-only")
	}
}

func (dyno *Dyno) AttachAndExecute(exe *Executor, args ...string) error {
	// If the Dyno isn't running we won't be able to attach to it.
	if dyno.State != DYNO_STATE_RUNNING {
		return fmt.Errorf("can't run %q when dyno is not running, details: %v", args, dyno.Info())
	}
	args = appender.Strings(
		[]string{
			DEFAULT_NODE_USERNAME + "@" + dyno.Host,
			"sudo",
			LXC_BIN,
			"exec",
			dyno.Container,
			"--",
		},
		args...,
	)
	return exe.Run("ssh", args...)
}

func (dyno *Dyno) RestartService(e *Executor) error {
	fmt.Fprintf(e.Logger, "Restarting app service for dyno %v\n", dyno.Info())
	return dyno.AttachAndExecute(e, "service", "app", "restart")
}

func (dyno *Dyno) StartService(e *Executor) error {
	fmt.Fprintf(e.Logger, "Starting app service for dyno %v\n", dyno.Info())
	return dyno.AttachAndExecute(e, "service", "app", "start")
}

func (dyno *Dyno) StopService(e *Executor) error {
	fmt.Fprintf(e.Logger, "Stopping app service for dyno %v\n", dyno.Info())
	return dyno.AttachAndExecute(e, "service", "app", "stop")
}

func (dyno *Dyno) GetServiceStatus(e *Executor) error {
	fmt.Fprintf(e.Logger, "Getting app service status for dyno %v\n", dyno.Info())
	return dyno.AttachAndExecute(e, "service", "app", "status")
}

// Check if a port is already in use.
func (dpt *DynoPortTracker) AlreadyInUse(host string, port int) bool {
	dpt.lock.Lock()
	defer dpt.lock.Unlock()
	if ports, ok := dpt.allocations[host]; ok {
		for _, p := range ports {
			if p == port {
				return true
			}
		}
	}
	return false
}

// Attempt to allocate a port for a node host.
func (dpt *DynoPortTracker) Allocate(host string, port int) error {
	dpt.lock.Lock()
	defer dpt.lock.Unlock()
	if ports, ok := dpt.allocations[host]; ok {
		// Require that the port not be already in use.
		for _, p := range ports {
			if p == port {
				return fmt.Errorf("Host/port combination %v/%v is already in use", host, port)
			}
		}
		dpt.allocations[host] = append(ports, port)
	} else {
		dpt.allocations[host] = []int{port}
	}
	// Schedule the port to be automatically freed once the status monitor will have picked up the in-use port.
	go func(host string, port int) {
		time.Sleep(1200 * time.Second)
		dpt.Release(host, port)
	}(host, port)
	log.Infof("DynoPortTracker.Allocate :: added host=%v port=%v", host, port)
	return nil
}

// Release a previously allocated host/port pair if it is still in the allocations table.
func (dpt *DynoPortTracker) Release(host string, port int) {
	log.Infof("DynoPortTracker.Release :: releasing port %v from host %v", port, host)
	dpt.lock.Lock()
	defer dpt.lock.Unlock()
	if ports, ok := dpt.allocations[host]; ok {
		newPorts := []int{}
		for _, p := range ports {
			if p != port {
				newPorts = append(newPorts, p)
			}
		}
		dpt.allocations[host] = newPorts
	}
}

// n.b. Container name format is: app-version-process-port.
// n.b. "container" param must container app-version-process-port-status.
func ContainerToDyno(host string, container string) (Dyno, error) {
	tokens := strings.SplitN(stringsutil.Reverse(container), DYNO_DELIMITER, 5)

	if len(tokens) != 5 {
		return Dyno{}, fmt.Errorf("unable to parse container string %q into 5 tokens", container)
	}

	// Un-reverse the things.
	for i, j := 0, len(tokens)-1; i < j; i, j = i+1, j-1 {
		tokens[i], tokens[j] = stringsutil.Reverse(tokens[j]), stringsutil.Reverse(tokens[i])
	}
	tokens[2] = stringsutil.Reverse(tokens[2])

	if !strings.HasPrefix(tokens[1], "v") {
		return Dyno{}, fmt.Errorf("invalid dyno version value %q, must begin with a 'v'", tokens[1])
	}
	versionNumber, err := strconv.Atoi(strings.TrimPrefix(tokens[1], "v"))
	if err != nil {
		return Dyno{}, err
	}
	portNumber, err := strconv.Atoi(tokens[3])
	if err != nil {
		return Dyno{}, err
	}
	dyno := Dyno{
		Host:          host,
		Container:     tokens[0] + DYNO_DELIMITER + tokens[1] + DYNO_DELIMITER + tokens[2] + DYNO_DELIMITER + tokens[3],
		Application:   tokens[0],
		Version:       tokens[1],
		Process:       tokens[2],
		Port:          tokens[3],
		State:         strings.ToLower(tokens[4]),
		VersionNumber: versionNumber,
		PortNumber:    portNumber,
	}
	return dyno, nil
}

func NodeStatusToDynos(nodeStatus *NodeStatus) ([]Dyno, error) {
	dynos := make([]Dyno, len(nodeStatus.Containers))
	for i, container := range nodeStatus.Containers {
		dyno, err := ContainerToDyno(nodeStatus.Host, container)
		if err != nil {
			return dynos, err
		}
		dynos[i] = dyno
	}
	return dynos, nil
}

func (server *Server) GetRunningDynos(application string, processType string) ([]Dyno, error) {
	var (
		dynos           = []Dyno{}
		expectedProcess = normalizeAppProcessName(processType)
	)

	cfg, err := server.getConfig(true)
	if err != nil {
		return dynos, err
	}

	for _, node := range cfg.Nodes {
		status := server.getNodeStatus(node)
		// skip this node if there's an error
		if status.Err != nil {
			continue
		}
		for _, container := range status.Containers {
			dyno, err := ContainerToDyno(node.Host, container)
			if err != nil {
				log.Errorf("parsing Container->Dyno for host/container=%v/%v: %v\n", node.Host, container, err)
			} else if dyno.State == DYNO_STATE_RUNNING && dyno.Application == application && dyno.Process == expectedProcess {
				dynos = append(dynos, dyno)
			}
		}
	}
	return dynos, nil
}

// NewDynoGenerator chooses which nodes to run the next N-count dynos on.
func (server *Server) NewDynoGenerator(nodes []*Node, application string, version string) (*DynoGenerator, error) {
	// Produce sorted sequence of NodeStatuses.
	allStatuses := []NodeStatusRunning{}
	for _, node := range nodes {
		running := false
		nodeStatus := server.getNodeStatus(node)
		// Determine if there is an identical app/version container already running on the node.
		for _, container := range nodeStatus.Containers {
			dyno, _ := ContainerToDyno(node.Host, container)
			if dyno.State == DYNO_STATE_RUNNING && dyno.Application == application && dyno.Version == version {
				running = true
				break
			}
		}
		allStatuses = append(allStatuses, NodeStatusRunning{nodeStatus, running})
	}

	if len(allStatuses) == 0 {
		return nil, fmt.Errorf("node list was empty, which means deployment is presently not possible")
	}

	sort.Sort(NodeStatuses(allStatuses))

	return &DynoGenerator{
		server:      server,
		statuses:    allStatuses,
		position:    0,
		application: application,
		version:     version,
		usedPorts:   []int{},
	}, nil

}

func (dg *DynoGenerator) Next(process string) (Dyno, error) {
	process = normalizeAppProcessName(process)
	nodeStatus := dg.statuses[dg.position%len(dg.statuses)].status
	dg.position++
	port := fmt.Sprint(dg.server.getNextPort(&nodeStatus, &dg.usedPorts))
	dyno, err := ContainerToDyno(nodeStatus.Host, dg.application+DYNO_DELIMITER+dg.version+DYNO_DELIMITER+process+DYNO_DELIMITER+port+DYNO_DELIMITER+DYNO_STATE_STOPPED)

	// Don't lose the process type!  This field gets re-used externally when error
	// is non-nil.
	dyno.Process = process
	return dyno, err
}

// NodeStatus sorting.
func (ns NodeStatuses) Len() int { return len(ns) } // boilerplate.

// NodeStatus sorting.
func (ns NodeStatuses) Swap(i int, j int) { ns[i], ns[j] = ns[j], ns[i] } // boilerplate.

// NodeStatus sorting.
func (ns NodeStatuses) Less(i int, j int) bool { // actual sorting logic.
	if ns[i].running && !ns[j].running {
		return true
	}
	if !ns[i].running && ns[j].running {
		return false
	}
	return ns[i].status.FreeMemoryMb > ns[j].status.FreeMemoryMb
}

func AppendIfMissing(slice []int, i int) []int {
	for _, ele := range slice {
		if ele == i {
			return slice
		}
	}
	return append(slice, i)
}

// Get the next available port for a node.
func (server *Server) getNextPort(nodeStatus *NodeStatus, usedPorts *[]int) int {
	port := server.GlobalPortTracker.Next()
	for _, container := range nodeStatus.Containers {
		dyno, err := ContainerToDyno(nodeStatus.Host, container)
		if err != nil {
			log.Warnf("Server.getNextPort :: Failed to create Dyno from container %q: %v", container, err)
			continue
		}
		if dyno.State == DYNO_STATE_RUNNING && dyno.PortNumber > 0 {
			*usedPorts = AppendIfMissing(*usedPorts, dyno.PortNumber)
		}
	}
	sort.Ints(*usedPorts)
	log.Infof("Server.getNextPort :: Found used ports: %v", *usedPorts)
	for _, usedPort := range *usedPorts {
		if port == usedPort || dynoPortTracker.AlreadyInUse(nodeStatus.Host, port) {
			port++
		} else if usedPort > port {
			break
		}
	}
	if err := dynoPortTracker.Allocate(nodeStatus.Host, port); err != nil {
		log.Infof("Server.getNextPort :: host/port combination %v/%v already in use, will find another", nodeStatus.Host, port)
		*usedPorts = AppendIfMissing(*usedPorts, port)
		return server.getNextPort(nodeStatus, usedPorts)
	}
	log.Infof("Server.getNextPort :: Result port: %v", port)
	*usedPorts = AppendIfMissing(*usedPorts, port)
	server.GlobalPortTracker.Using(port)
	return port
}

// normalizeAppProcessName converts dashes to camelCase.
func normalizeAppProcessName(process string) string {
	// Camel-case process name to align with conversion in container creation
	// shell script.
	p := regexp.MustCompile(`[-_]+([a-zA-Z0-9])`).ReplaceAllStringFunc(process, func(s string) string {
		return strings.ToUpper(string(s[1]))
	})
	return p
}
