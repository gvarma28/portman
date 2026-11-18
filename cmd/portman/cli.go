package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gvarma28/portman/internal/comm"
)

type Message struct {
	Type string `json:"type"`
	Body string `json:"body"`
}

func runCli() {
	conn, err := net.Dial("unix", "/tmp/portman.sock")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	// send
	data, _ := json.Marshal(os.Args[1:])
	comm.WriteMsg(conn, data)

	// receive
	raw, err := comm.ReadMsg(conn)
	if err != nil { /* handle */
	}
	var r comm.Response
	json.Unmarshal(raw, &r)

	fmt.Println(r.Message)
	if r.Entries != nil {
		hostMap := make(map[string][]string)
		for _, e := range r.Entries {
			hostMap[e.Host] = append(hostMap[e.Host], fmt.Sprintf("%d:%d", e.Local, e.Remote))
		}
		for h, p := range hostMap {
			for _, ports := range p {
				fmt.Println(h, ports)
			}
		}
	}
}
