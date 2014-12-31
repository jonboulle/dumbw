package main

/*
i3status writes a neverending JSON stream to stdout, adding a new entry
typically every second, in the following format:

	{"version":1}
	[
	[{"name":"time","full_text":"Tue 30 Dec 2014 22:40:57"},{"name":"wireless","instance":"wlp2s0","color":"#00FF00","full_text":"192.168.0.108"},{"name":"battery","instance":"/sys/class/power_supply/BAT0/uevent","full_text":"BAT 85% 05:14:54"},{"name":"cpu_usage","full_text":"-2%"},{"name":"volume","instance":"default.Master.0","full_text":"♪94%"}]
	,[{"name":"time","full_text":"Tue 30 Dec 2014 22:40:58"},{"name":"wireless","instance":"wlp2s0","color":"#00FF00","full_text":"192.168.0.108"},{"name":"battery","instance":"/sys/class/power_supply/BAT0/uevent","full_text":"BAT 85% 05:14:54"},{"name":"cpu_usage","full_text":"04%"},{"name":"volume","instance":"default.Master.0","full_text":"♪94%"}]
	...
*/

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"net/rpc"
	"os"
	"strings"
)

const defaultInterface = "wlp2s0"

type item struct {
	Name     string `json:"name"`
	FullText string `json:"full_text"`
	MinWidth string `json:"min_width,omitempty"`
	Align    string `json:"align,omitempty"`
}

// Wrap the real i3status by consuming its output, modifying it, and then printing to stdout
func i3status() {
	iface := defaultInterface
	if len(os.Args) > 1 {
		iface = os.Args[1]
	}
	client, err := rpc.Dial("unix", sockFile)
	if err != nil {
		log.Fatalf("dialing: %v", err)
	}
	s := bufio.NewScanner(os.Stdin)
	// ignore version line and subsequent line
	s.Scan()
	fmt.Println(s.Text())
	s.Scan()
	fmt.Println(s.Text())
	for s.Scan() {
		line := s.Text()
		prefix := ""
		if strings.HasPrefix(line, ",") {
			prefix = ","
			line = line[1:]
		}
		var list []interface{}
		err := json.Unmarshal([]byte(line), &list)
		if err != nil {
			panic(err)
		}
		rxr := &item{
			Name:     fmt.Sprintf("%s-rxrate", iface),
			FullText: get(client, iface, "Rx"),
		}
		txr := &item{
			Name:     fmt.Sprintf("%s-txrate", iface),
			FullText: get(client, iface, "Tx"),
		}
		l := []interface{}{rxr, txr}
		list = append(l, list...)
		b, err := json.Marshal(list)
		if err != nil {
			panic(err)
		}
		fmt.Println(prefix + string(b))
	}
}

func get(client *rpc.Client, iface, rate string) string {
	method := fmt.Sprintf("StatsMap.%sRate", rate)
	var r Rate
	err := client.Call(method, iface, &r)
	if err != nil {
		return fmt.Sprintf("%s: N/A", rate)
	}
	return fmt.Sprintf("%s: %s", rate, r)
}
