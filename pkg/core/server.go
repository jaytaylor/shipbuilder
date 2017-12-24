package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"

	"github.com/jaytaylor/shipbuilder/pkg/domain"

	lsbase "github.com/jaytaylor/logserver"
	logserver "github.com/jaytaylor/logserver/server"
	log "github.com/sirupsen/logrus"
)

const (
	MinDynoPort = 10000
	MaxDynoPort = 60000

	DefaultListenAddr = ":9999"
)

var (
	globalLock sync.Mutex
	appLocks   = map[string]bool{}
)

type Server struct {
	ListenAddr                string
	LogServerListenAddr       string
	LogServer                 *logserver.Server
	BuildpacksProvider        domain.BuildpacksProvider
	ReleasesProvider          domain.ReleasesProvider
	GlobalPortTracker         *GlobalPortTracker
	currentLoadBalancerConfig string
}

func run(name string, args ...string) error {
	log.Printf("= %v %v", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (server *Server) getSimpleLogger(conn net.Conn) io.Writer {
	return NewMessageLogger(conn)
}

func (server *Server) getLogger(conn net.Conn) io.Writer {
	return NewTimeLogger(server.getSimpleLogger(conn))
}

func (server *Server) getTitleAndDimLoggers(conn net.Conn) (io.Writer, io.Writer) {
	var (
		logger      = server.getLogger(conn)
		titleLogger = NewFormatter(logger, GREEN)
		dimLogger   = NewFormatter(logger, DIM)
	)
	return titleLogger, dimLogger
}

// Provides common functionality for appending tokenized unique items to a list, with logging of details regarding which items were added
// or rejected.  Helps avoid repetition in some of the cmd_* methods.
func (server *Server) UniqueStringsAppender(conn net.Conn, items []string, addItems []string, itemType string, addListenerFn func(string)) []string {
	titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
	fmt.Fprintf(titleLogger, "=== Adding %vs\n", itemType)

	for _, addItem := range addItems {
		if len(addItem) == 0 {
			continue
		}
		found := false
		for _, existingItem := range items {
			if strings.ToLower(addItem) == strings.ToLower(existingItem) {
				fmt.Fprintf(dimLogger, "%v already exists: %v\n", strings.Title(itemType), addItem)
				found = true
				break
			}
		}
		if !found {
			fmt.Fprintf(dimLogger, "Adding %v: %v\n", itemType, addItem)
			items = append(items, addItem)
			if addListenerFn != nil {
				addListenerFn(addItem)
			}
		}
	}
	return items
}

// Provides common functionality for removing tokenized items from a list, with logging of details regarding which items were removed.
// Helps avoid repetition in some of the cmd_* methods.
func (server *Server) UniqueStringsRemover(conn net.Conn, items []string, removeItems []string, itemType string, removeListenerFn func(string)) []string {
	titleLogger, dimLogger := server.getTitleAndDimLoggers(conn)
	fmt.Fprintf(titleLogger, "=== Removing %vs\n", itemType)

	originalItems := items
	items = []string{}
	for _, existingItem := range originalItems {
		keep := true
		for _, removeItem := range removeItems {
			if strings.ToLower(removeItem) == strings.ToLower(existingItem) {
				fmt.Fprintf(dimLogger, "Removing %v: %v\n", itemType, existingItem)
				keep = false
				break
			}
		}
		if keep {
			items = append(items, existingItem)
		} else if removeListenerFn != nil {
			removeListenerFn(existingItem)
		}
	}
	return items
}

func (server *Server) handleCall(conn net.Conn, body string) error {
	var args []interface{}
	err := json.Unmarshal([]byte(body), &args)
	if err != nil {
		return err
	}

	// Convert any args of type T=map[string]interface{} to map[string]string.
	for i, arg := range args {
		m, ok := arg.(map[string]interface{})
		if ok {
			nMap := map[string]string{}
			for k, v := range m {
				nMap[k] = fmt.Sprint(v)
			}
			args[i] = nMap
		}
	}
	// Convert multiple string args to []string.
	for i, arg := range args {
		list, ok := arg.([]interface{})
		if ok {
			nList := []string{}
			for _, value := range list {
				nList = append(nList, fmt.Sprint(value))
			}
			args[i] = nList
		}
	}

	if len(args) == 0 {
		return fmt.Errorf("expected command")
	}
	log.Infof("Received cmd: %v", args)
	for _, cmd := range commands {
		if cmd.ServerName == args[0].(string) {
			method, ok := reflect.TypeOf(server).MethodByName(args[0].(string))
			if !ok {
				return fmt.Errorf("unknown method: %v", cmd)
			}
			values := make([]reflect.Value, len(args)+1)
			values[0] = reflect.ValueOf(server)
			values[1] = reflect.ValueOf(conn)
			for i := 1; i < len(args); i++ {
				values[i+1] = reflect.ValueOf(args[i])
			}
			defer func() {
				// reflect can panic so recover here
				if r := recover(); r != nil {
					Errorf(conn, "error running command: %v, %v", args, r)
				}
			}()
			// For any application specific write commands we lock
			//   based on the application name
			needsLock := cmd.AppWrite
			if !needsLock {
				// also lock these, lock is based on git directory
				needsLock = cmd.LongName == "pre-receive" || cmd.LongName == "post-receive"
			}
			if needsLock && args[1] != "" {
				globalLock.Lock()
				active, ok := appLocks[args[1].(string)]
				if ok && active {
					globalLock.Unlock()
					return fmt.Errorf("a command is already running for this application")
				}
				appLocks[args[1].(string)] = true
				globalLock.Unlock()
				// Remove lock when we're done
				defer func() {
					globalLock.Lock()
					delete(appLocks, args[1].(string))
					globalLock.Unlock()
				}()
			}
			values = method.Func.Call(values)

			// Handle an error being returned
			if len(values) >= 0 && values[0].CanInterface() {
				err, ok = values[0].Interface().(error)
				if ok {
					return err
				}
			}

			return nil
		}
	}
	return fmt.Errorf("unknown command: %v", args)
}

func (server *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	msg, err := Receive(conn)
	if err != nil {
		log.Printf("invalid message: %v", err)
		Send(conn, Message{Error, "Error reading message"})
		return
	}
	log.Printf("received: %v", msg)
	switch msg.Type {
	case Call:
		err = server.handleCall(conn, msg.Body)
		if err != nil {
			Send(conn, Message{Error, err.Error()})
		}
	}
}

func (server *Server) verifyRequiredBuildPacks() error {
	return server.WithConfig(func(cfg *Config) error {
		for _, app := range cfg.Applications {
			if _, err := server.BuildpacksProvider.New(app.BuildPack); err != nil {
				return fmt.Errorf("missing build-pack %q for application %q", app.BuildPack, app.Name)
			}
		}
		return nil
	})
}

func (server *Server) Start() error {
	server.init()

	var err error

	if server.LogServer, err = logserver.Start(server.LogServerListenAddr); err != nil {
		return err
	}

	if err = server.initTemplates(); err != nil {
		return err
	}

	server.GlobalPortTracker = &GlobalPortTracker{
		Min: MinDynoPort,
		Max: MaxDynoPort,
	}

	initDrains(server)
	go server.monitorNodes()
	go server.startCrons()

	log.Infof("Starting server on %v", server.ListenAddr)
	ln, err := net.Listen("tcp", server.ListenAddr)
	if err != nil {
		return err
	}

	if err = server.verifyRequiredBuildPacks(); err != nil {
		return err
	}

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				log.Errorf("err in connection loop (will continue on): %v", err)
				continue
			}
			log.Infof("new connection %v", conn.RemoteAddr())
			go server.handleConnection(conn)
		}
	}()

	return nil
}

// init initializes default values to empty struct configuration members.
//
// TODO: This is quick and dirty, and should really go in a `NewServer()'
// constructor.
func (server *Server) init() {
	if server.ListenAddr == "" {
		server.ListenAddr = DefaultListenAddr
	}
	if server.LogServerListenAddr == "" {
		server.LogServerListenAddr = fmt.Sprintf(":%v", lsbase.DefaultPort)
	}
}
