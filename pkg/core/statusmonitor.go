package core

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
)

const statusMonitorCheckCommand = `echo $(free -m | grep '^Mem:' | grep --only '[0-9]\+$') $(sudo lxc list --format json | jq -r '.[] | select(.status == "Running") | "\(.name)` + DYNO_DELIMITER + `\(.status)"' | tr $'\n' ' ')`

var nodeStatusRequestChannel = make(chan NodeStatusRequest)

type NodeStatus struct {
	Host         string
	FreeMemoryMb int
	Containers   []string
	DeployMarker int
	Ts           time.Time // No need to specify, will be automatically filled by `.Parse()'.
	Err          error
}

type NodeStatusRequest struct {
	host          string
	resultChannel chan NodeStatus
}

func (ns *NodeStatus) Parse(input string, err error) {
	if err != nil {
		ns.Err = err
		return
	}

	tokens := strings.Fields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		ns.Err = fmt.Errorf("status parse failed for input %q", input)
		return
	}

	ns.FreeMemoryMb, err = strconv.Atoi(tokens[0])
	if err != nil {
		ns.Err = fmt.Errorf("integer conversion failed for token %q (tokens=%v)", tokens[0], tokens)
		return
	}

	ns.Containers = tokens[1:]
	ns.Ts = time.Now()
}

func RemoteCommand(DefaultSSHHost string, sshArgs ...string) (string, error) {
	frontArgs := append([]string{"1m", "ssh", DEFAULT_NODE_USERNAME + "@" + DefaultSSHHost}, defaultSSHParametersList...)
	combinedArgs := append(frontArgs, sshArgs...)

	//fmt.Printf("debug: cmd is -> ssh %v <-\n", combinedArgs)
	bs, err := exec.Command("timeout", combinedArgs...).CombinedOutput()

	if err != nil {
		return "", err
	}

	return string(bs), nil
}

func checkServer(DefaultSSHHost string, currentDeployMarker int, ch chan NodeStatus) {
	// Shell command which combines free MB with list of running containers.
	done := make(chan NodeStatus)

	go func() {
		result := NodeStatus{
			Host:         DefaultSSHHost,
			FreeMemoryMb: -1,
			Containers:   nil,
			DeployMarker: currentDeployMarker,
			Err:          nil,
		}
		result.Parse(RemoteCommand(DefaultSSHHost, statusMonitorCheckCommand))
		done <- result
	}()

	select {
	case result := <-done: // Captures completed status update.
		ch <- result // Sends result to channel.
	case <-time.After(30 * time.Second):
		ch <- NodeStatus{
			Host:         DefaultSSHHost,
			FreeMemoryMb: -1,
			Containers:   nil,
			DeployMarker: currentDeployMarker,
			Err:          fmt.Errorf("check timed out for host=%v", DefaultSSHHost),
		} // Sends timeout result to channel.
	}
}

func (server *Server) checkNodes(resultChan chan NodeStatus) error {
	cfg, err := server.getConfig(true)
	if err != nil {
		return err
	}
	currentDeployMarker := deployLock.value()

	for _, node := range cfg.Nodes {
		go checkServer(node.Host, currentDeployMarker, resultChan)
	}
	return nil
}

func (server *Server) monitorNodes() {
	repeater := time.Tick(STATUS_MONITOR_INTERVAL_SECONDS * time.Second)
	nodeStatusChan := make(chan NodeStatus)
	hostStatusMap := map[string]NodeStatus{}

	// Kick off the initial checks so we don't have to wait for the next tick.
	server.checkNodes(nodeStatusChan)

	for {
		select {
		case <-repeater:
			server.checkNodes(nodeStatusChan)

		case result := <-nodeStatusChan:
			if deployLock.validateLatest(result.DeployMarker) {
				hostStatusMap[result.Host] = result
				if err := server.pruneDynos(result, &hostStatusMap); err != nil {
					log.Errorf("Problem pruning dynos: %s", err)
				}
			}

		case request := <-nodeStatusRequestChannel:
			status, ok := hostStatusMap[request.host]
			if !ok {
				status = NodeStatus{
					Host:         request.host,
					FreeMemoryMb: -1,
					Containers:   nil,
					DeployMarker: -1,
					Err:          fmt.Errorf("Unknown host %v", request.host),
				}
			}
			request.resultChannel <- status
		}
	}
}

func (*Server) getNodeStatus(node *Node) NodeStatus {
	request := NodeStatusRequest{node.Host, make(chan NodeStatus)}
	nodeStatusRequestChannel <- request
	status := <-request.resultChannel
	//fmt.Printf("boom! %v\n", status)
	return status
}
