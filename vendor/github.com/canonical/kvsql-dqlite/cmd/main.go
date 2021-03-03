package main

import (
	"github.com/canonical/kvsql-dqlite/server"
	"log"
)

func main() {
	_, err := server.New("/tmp/node1")
	if err != nil {
		log.Fatal(err)
	}
}
