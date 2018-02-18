package core

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"

	log "github.com/sirupsen/logrus"
	//"code.google.com/p/go.crypto/ssh/terminal"
)

const (
	STDIN_FD  = 0
	STDOUT_FD = 1
	STDERR_FD = 2
)

type Client struct{}

func fail(format string, args ...interface{}) {
	fmt.Printf("\033[%vm%v\033[0m\n", RED, fmt.Sprintf(format, args...))
	os.Exit(1)
}

func (*Client) send(msg Message, disableTunnel bool) error {
	log.WithField("msg", fmt.Sprintf("%+v", msg)).Debug("CLIENT DEBUG")
	// Open a tunnel if necessary
	/*if terminal.IsTerminal(STDOUT_FD) {
		fmt.Print("HEY DUDE, I CAN TELL THIS IS RUNNING IN A TERMINAL\n")
	} else {
		fmt.Print("HEY DUDE, I COULD TELL DIZ AIN'T NO TERMNAL\n")
	}*/

	if !disableTunnel && !strings.Contains(strings.ToLower(DefaultSSHHost), "localhost") && !strings.Contains(strings.ToLower(DefaultSSHHost), "127.0.0.1") {
		bs, err := exec.Command("hostname").Output()
		if err != nil || !bytes.HasPrefix(bs, []byte("ip-")) {
			t, err := OpenTunnel()
			if err != nil {
				return err
			}
			defer t.Close()
		}
	}

	conn, err := net.Dial("tcp", "localhost:9999")
	if err != nil {
		return err
	}
	defer conn.Close()

	err = Send(conn, msg)
	if err != nil {
		return err
	}

	for {
		msg, err := Receive(conn)
		if err != nil {
			return err
		}
		switch msg.Type {
		case ReadLineRequest:
			fmt.Printf("\033[%vm%v\033[0m", RED, msg.Body)
			response, err := bufio.NewReader(os.Stdin).ReadString('\n')
			if err != nil {
				fmt.Printf("local error, operation aborted: \033[%vm%v\033[0m\n", RED, err)
				os.Exit(1)
			}
			Send(conn, Message{ReadLineResponse, response})
		case Hijack:
			ec := make(chan error, 1)
			go func() {
				_, err := io.Copy(conn, os.Stdin)
				ec <- err
			}()
			go func() {
				_, err := io.Copy(os.Stdout, conn)
				ec <- err
			}()
			return <-ec
		case Log:
			fmt.Printf("%v", msg.Body)
		case Error:
			fmt.Printf("\033[%vm%v\033[0m\n", RED, msg.Body)
			os.Exit(1)
		default:
			log.Printf("received %v", msg)
		}
	}

	return nil
}

// RemoteExec takes a Shipbuilder server method name and corresponding args, and
// invokes it remotely.
func (client *Client) RemoteExec(methodName string, args ...interface{}) error {
	bs, err := json.Marshal(append([]interface{}{methodName}, args...))
	if err != nil {
		return err
	}
	disableTunnel := methodName == "PreReceive" || methodName == "PostReceive"
	if err := client.send(Message{Call, string(bs)}, disableTunnel); err != nil {
		return err
	}
	return nil
}

func (client *Client) Do(args []string) {
	var (
		local     = &Local{}
		localType = reflect.TypeOf(local)
	)

	// log.Infof("Args=%+v", args)
	for _, cmd := range commands {
		if args[0] == cmd.ShortName || args[0] == cmd.LongName {
			parsed, err := cmd.Parse(args[1:])
			if err != nil {
				fail("%v", err)
				return
			}

			if method, ok := localType.MethodByName(cmd.ServerName); ok {
				vs := []reflect.Value{reflect.ValueOf(local)}
				for _, v := range parsed {
					vs = append(vs, reflect.ValueOf(v))
				}
				vs = method.Func.Call(vs)

				// Handle an error being returned
				if len(vs) > 0 && vs[0].CanInterface() {
					err, ok = vs[0].Interface().(error)
					if ok {
						fail("%v", err)
						return
					}
				}

				return
			}

			/*if cmd.LongName == "logger" {
				err = (&Local{}).Logger(args[0], args[1], parsed[2])
				if err != nil {
					fail("%v", err)
				}
				return
			}*/

			bs, _ := json.Marshal(append([]interface{}{cmd.ServerName}, parsed...))
			disableTunnel := cmd.ServerName == "PreReceive" || cmd.ServerName == "PostReceive"
			// fmt.Println(string(bs))
			err = client.send(Message{Call, string(bs)}, disableTunnel)
			if err != nil {
				fail("%v", err)
				return
			}
			return
		}
	}

	fail("Unknown command; args=%v", args)
}
