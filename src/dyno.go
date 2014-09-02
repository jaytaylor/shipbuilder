package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type (
	Dyno struct {
		Host, Container, Application, Process, Version, Port, State string
		VersionNumber, PortNumber                                   int
	}
	NodeStatusRunning struct {
		status  NodeStatus
		running bool
	}
	NodeStatuses  []NodeStatusRunning
	DynoGenerator struct {
		server      *Server
		statuses    []NodeStatusRunning
		position    int
		application string
		version     string
		usedPorts   []int
	}
	DynoPortTracker struct {
		allocations map[string][]int
		lock        sync.Mutex
	}
)

const (
	DYNO_DELIMITER     = "_"
	DYNO_STATE_RUNNING = "running"
	DYNO_STATE_STOPPED = "stopped"
)

var (
	dynoPortTracker = DynoPortTracker{allocations: map[string][]int{}, lock: sync.Mutex{}}
)

func (this *Dyno) Info() string {
	return fmt.Sprintf("host=%v app=%v version=%v proc=%v port=%v state=%v", this.Host, this.Application, this.Version, this.Process, this.Port, this.State)
}

func (this *Dyno) Shutdown(e *Executor) error {
	fmt.Fprintf(e.logger, "Shutting down dyno: %v\n", this.Info())
	if this.State == DYNO_STATE_RUNNING {
		// Shutdown then destroy.
		return e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+this.Host, "sudo", "/tmp/shutdown_container.py", this.Container)
	} else {
		// Destroy only.
		return e.Run("ssh", DEFAULT_NODE_USERNAME+"@"+this.Host, "sudo", "/tmp/shutdown_container.py", this.Container, "destroy-only")
	}
}

func (this *Dyno) AttachAndExecute(e *Executor, args ...string) error {
	// If the Dyno isn't running we won't be able to attach to it.
	if this.State != DYNO_STATE_RUNNING {
		return fmt.Errorf("can't run `%v` when dyno is not running, details: %v", args, this.Info())
	}
	args = AppendStrings([]string{DEFAULT_NODE_USERNAME + "@" + this.Host, "sudo", "lxc-attach", "-n", this.Container, "--"}, args...)
	return e.Run("ssh", args...)
}

func (this *Dyno) RestartService(e *Executor) error {
	fmt.Fprintf(e.logger, "Restarting app service for dyno %v\n", this.Info())
	return this.AttachAndExecute(e, "service", "app", "restart")
}

func (this *Dyno) StartService(e *Executor) error {
	fmt.Fprintf(e.logger, "Starting app service for dyno %v\n", this.Info())
	return this.AttachAndExecute(e, "service", "app", "start")
}

func (this *Dyno) StopService(e *Executor) error {
	fmt.Fprintf(e.logger, "Stopping app service for dyno %v\n", this.Info())
	return this.AttachAndExecute(e, "service", "app", "stop")
}

func (this *Dyno) GetServiceStatus(e *Executor) error {
	fmt.Fprintf(e.logger, "Getting app service status for dyno %v\n", this.Info())
	return this.AttachAndExecute(e, "service", "app", "status")
}

// Check if a port is already in use.
func (this *DynoPortTracker) AlreadyInUse(host string, port int) bool {
	this.lock.Lock()
	defer this.lock.Unlock()
	if ports, ok := this.allocations[host]; ok {
		for _, p := range ports {
			if p == port {
				return true
			}
		}
	}
	return false
}

// Attempt to allocate a port for a node host.
func (this *DynoPortTracker) Allocate(host string, port int) error {
	this.lock.Lock()
	defer this.lock.Unlock()
	if ports, ok := this.allocations[host]; ok {
		// Require that the port not be already in use.
		for _, p := range ports {
			if p == port {
				return fmt.Errorf("Host/port combination %v/%v is already in use", host, port)
			}
		}
		this.allocations[host] = append(ports, port)
	} else {
		this.allocations[host] = []int{port}
	}
	// Schedule the port to be automatically freed once the status monitor will have picked up the in-use port.
	go func(host string, port int) {
		time.Sleep(1200 * time.Second)
		this.Release(host, port)
	}(host, port)
	fmt.Printf("DynoPortTracker.Allocate :: added host=%v port=%v\n", host, port)
	return nil
}

// Release a previously allocated host/port pair if it is still in the allocations table.
func (this *DynoPortTracker) Release(host string, port int) {
	fmt.Printf("DynoPortTracker.Release :: releasing port %v from host %v\n", port, host)
	this.lock.Lock()
	defer this.lock.Unlock()
	if ports, ok := this.allocations[host]; ok {
		newPorts := []int{}
		for _, p := range ports {
			if p != port {
				newPorts = append(newPorts, p)
			}
		}
		this.allocations[host] = newPorts
	}
}

// NB: Container name format is: appName_version_process_port
func ContainerToDyno(host string, container string) (Dyno, error) {
	tokens := strings.Split(container, DYNO_DELIMITER)
	if len(tokens) != 5 {
		return Dyno{}, fmt.Errorf("Failed to parse container string '%v' into 5 tokens", container)
	}
	if !strings.HasPrefix(tokens[1], "v") {
		return Dyno{}, fmt.Errorf("Invalid dyno version value '%v', must begin with a 'v'", tokens[1])
	}
	versionNumber, err := strconv.Atoi(strings.TrimPrefix(tokens[1], "v"))
	if err != nil {
		return Dyno{}, err
	}
	portNumber, err := strconv.Atoi(tokens[3])
	if err != nil {
		return Dyno{}, err
	}
	return Dyno{
		Host:          host,
		Container:     tokens[0] + DYNO_DELIMITER + tokens[1] + DYNO_DELIMITER + tokens[2] + DYNO_DELIMITER + tokens[3],
		Application:   tokens[0],
		Version:       tokens[1],
		Process:       tokens[2],
		Port:          tokens[3],
		State:         strings.ToLower(tokens[4]),
		VersionNumber: versionNumber,
		PortNumber:    portNumber,
	}, nil
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

func (this *Server) GetRunningDynos(application, processType string) ([]Dyno, error) {
	dynos := []Dyno{}

	cfg, err := this.getConfig(true)
	if err != nil {
		return dynos, err
	}

	for _, node := range cfg.Nodes {
		status := this.getNodeStatus(node)
		// skip this node if there's an error
		if status.Err != nil {
			continue
		}
		for _, container := range status.Containers {
			dyno, err := ContainerToDyno(node.Host, container)
			if err != nil {
				fmt.Printf("error: Container->Dyno parse failed: %v\n", err)
			} else if dyno.State == DYNO_STATE_RUNNING && dyno.Application == application && dyno.Process == processType {
				dynos = append(dynos, dyno)
			}
		}
	}
	return dynos, nil
}

// Decicde which nodes to run the next N-count dynos on.
func (this *Server) NewDynoGenerator(nodes []*Node, application string, version string) (*DynoGenerator, error) {
	// Produce sorted sequence of NodeStatuses.
	allStatuses := []NodeStatusRunning{}
	for _, node := range nodes {
		running := false
		nodeStatus := this.getNodeStatus(node)
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
		return nil, fmt.Errorf("The node list was empty, which means deployment is not presently possible")
	}

	sort.Sort(NodeStatuses(allStatuses))

	return &DynoGenerator{
		server:      this,
		statuses:    allStatuses,
		position:    0,
		application: application,
		version:     version,
		usedPorts:   []int{},
	}, nil

}

func (this *DynoGenerator) Next(process string) Dyno {
	nodeStatus := this.statuses[this.position%len(this.statuses)].status
	this.position++
	port := fmt.Sprint(this.server.getNextPort(&nodeStatus, &this.usedPorts))
	dyno, _ := ContainerToDyno(nodeStatus.Host, this.application+DYNO_DELIMITER+this.version+DYNO_DELIMITER+process+DYNO_DELIMITER+port+DYNO_DELIMITER+DYNO_STATE_STOPPED)
	return dyno
}

// NodeStatus sorting.
func (this NodeStatuses) Len() int { return len(this) } // boilerplate.

// NodeStatus sorting.
func (this NodeStatuses) Swap(i int, j int) { this[i], this[j] = this[j], this[i] } // boilerplate.

// NodeStatus sorting.
func (this NodeStatuses) Less(i int, j int) bool { // actual sorting logic.
	if this[i].running && !this[j].running {
		return true
	}
	if !this[i].running && this[j].running {
		return false
	}
	return this[i].status.FreeMemoryMb > this[j].status.FreeMemoryMb
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
func (this *Server) getNextPort(nodeStatus *NodeStatus, usedPorts *[]int) int {
	port := 10000
	for _, container := range nodeStatus.Containers {
		dyno, err := ContainerToDyno(nodeStatus.Host, container)
		if err != nil {
			fmt.Printf("warning: Server.getNextPort :: Failed to create Dyno from container '%v': %v\n", container, err)
			continue
		}
		if dyno.State == DYNO_STATE_RUNNING && dyno.PortNumber > 0 {
			*usedPorts = AppendIfMissing(*usedPorts, dyno.PortNumber)
		}
	}
	sort.Ints(*usedPorts)
	fmt.Printf("Server.getNextPort :: Found used ports: %v\n", *usedPorts)
	for _, usedPort := range *usedPorts {
		if port == usedPort || dynoPortTracker.AlreadyInUse(nodeStatus.Host, port) {
			port++
		} else if usedPort > port {
			break
		}
	}
	err := dynoPortTracker.Allocate(nodeStatus.Host, port)
	if err != nil {
		fmt.Printf("Server.getNextPort :: host/port combination %v/%v already in use, will find another\n", nodeStatus.Host, port)
		*usedPorts = AppendIfMissing(*usedPorts, port)
		return this.getNextPort(nodeStatus, usedPorts)
	}
	fmt.Printf("Server.getNextPort :: Result port: %v\n", port)
	*usedPorts = AppendIfMissing(*usedPorts, port)
	return port
}
