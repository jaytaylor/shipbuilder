package main

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type (
	NodeStatus struct {
		Host          string
		FreeMemoryMb  int
		Containers    []string
		DeployCounter int
		Err           error
	}
	NodeStatusRequest struct {
		host          string
		resultChannel chan NodeStatus
	}
)

var nodeStatusRequestChannel = make(chan NodeStatusRequest)

func remoteCommand(sshHost string, sshArgs ...string) (string, error) {
	sshFrontArgs := []string{DEFAULT_NODE_USERNAME + "@" + sshHost, "-o", "StrictHostKeyChecking no", "-o", "BatchMode yes"}
	sshCombinedArgs := append(sshFrontArgs, sshArgs...)

	//fmt.Printf("debug: cmd is -> ssh %v <-\n", sshCombinedArgs)
	bs, err := exec.Command("ssh", sshCombinedArgs...).CombinedOutput()

	if err != nil {
		return "", err
	}

	return string(bs), nil
}

func (this *NodeStatus) parse(input string, err error) {
	if err != nil {
		this.Err = err
		return
	}

	tokens := strings.Fields(strings.TrimSpace(input))
	if len(tokens) == 0 {
		this.Err = fmt.Errorf("Parse failed for input '%v'", input)
		return
	}

	this.FreeMemoryMb, err = strconv.Atoi(tokens[0])
	if err != nil {
		this.Err = fmt.Errorf("Integer conversion failed for token '%v' (tokens=%v)", tokens[0], tokens)
		return
	}

	this.Containers = tokens[1:]
}

func checkServer(sshHost string, currentDeployCounter int, ch chan NodeStatus) {
	// Shell command which combines free MB with list of running containers.
	statusCheck := `echo $(free -m | sed '1,2d' | head -n1 | grep --only '[0-9]\+$') $(sudo lxc-ls --fancy | grep '[^ ]\+ \+RUNNING \+' | cut -f1 -d' ' | tr '\n' ' ')`
	done := make(chan NodeStatus)
	go func() {
		result := NodeStatus{
			Host:          sshHost,
			FreeMemoryMb:  -1,
			Containers:    nil,
			DeployCounter: currentDeployCounter,
			Err:           nil,
		}
		result.parse(remoteCommand(sshHost, statusCheck))
		done <- result
	}()
	select {
	case result := <-done: // Captures completed status update.
		ch <- result // Sends result to channel.
	case <-time.After(15 * time.Second):
		ch <- NodeStatus{
			Host:          sshHost,
			FreeMemoryMb:  -1,
			Containers:    nil,
			DeployCounter: currentDeployCounter,
			Err:           fmt.Errorf("Timed out for host %v", sshHost),
		} // Sends timeout result to channel.
	}
}

func (this *Server) checkNodes(resultChan chan NodeStatus) error {
	cfg, err := this.getConfig(true)
	if err != nil {
		return err
	}
	currentDeployCounter := deployLock.value()

	for _, node := range cfg.Nodes {
		go checkServer(node.Host, currentDeployCounter, resultChan)
	}
	return nil
}

func (this *Server) monitorFreeMemory() {
	repeater := time.Tick(15 * time.Second)
	myChan := make(chan NodeStatus)
	hostStatusMap := make(map[string]NodeStatus)

	// Kick off the initial checks so we don't have to wait for the next tick.
	this.checkNodes(myChan)

	for {
		select {
		case <-repeater:
			this.checkNodes(myChan)

		case result := <-myChan:
			if deployLock.validateLatest(result.DeployCounter) {
				hostStatusMap[result.Host] = result
				this.PruneDynos(result)
			}

		case request := <-nodeStatusRequestChannel:
			status, ok := hostStatusMap[request.host]
			if !ok {
				status = NodeStatus{
					Host:          request.host,
					FreeMemoryMb:  -1,
					Containers:    nil,
					DeployCounter: -1,
					Err:           fmt.Errorf("Unknown host %v", request.host),
				}
			}
			request.resultChannel <- status
		}
	}
}

func (this *Server) getNodeStatus(node *Node) NodeStatus {
	request := NodeStatusRequest{node.Host, make(chan NodeStatus)}
	nodeStatusRequestChannel <- request
	status := <-request.resultChannel
	//fmt.Printf("boom! %v\n", status)
	return status
}
