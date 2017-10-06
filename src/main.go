package main

import (
	"os"

	log "github.com/sirupsen/logrus"
)

func main() {
	args := os.Args
	if len(args) < 2 {
		log.Fatalln("expected at least one argument")
		return
	}
	switch args[1] {
	case "server":
		log.Println(new(Server).start())
	default:
		new(Client).Do(args)
	}

}
