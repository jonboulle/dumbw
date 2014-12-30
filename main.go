package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/rpc"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

/*
Inter-|   Receive                                                |  Transmit
face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
virbr0-nic:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
virbr0:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
wlp2s0: 39631108358 27753100    0    0    0     0          0         0 2365593650 14270663    0    0    0     0       0          0
*/

const (
	procNetDev = "/proc/net/dev"

	fields   = "interface bytes packets errs drop fifo frame compressed multicast bytes packets errs drop fifo colls carrier compressed"
	iIface   = 0
	iRxBytes = 1
	iTxBytes = 9

	interval = 1 * time.Second
)

var (
	runDir   = fmt.Sprintf("/run/user/%d/dumbw", os.Getuid())
	sockFile = filepath.Join(runDir, "socket")
)

type Rate float64

func (r Rate) String() string {
	f := float64(r)
	suffix := "B/s"
	if f > 1024.0 {
		f = f / 1024.0
		suffix = "KB/s"
	}
	if f > 1024.0 {
		f = f / 1024.0
		suffix = "MB/s"
	}
	if f > 1024.0 {
		f = f / 1024.0
		suffix = "GB/s"
	}
	return fmt.Sprintf("%f%s", f, suffix)
}

type StatsMap map[string]stats

type stats struct {
	rxRate Rate
	txRate Rate
	snapshot
}

type snapshot struct {
	rxBytes uint64
	txBytes uint64
}

func lock(dir string) bool {
	err := os.MkdirAll(dir, 0755)
	if err != nil {
		panic(err)
	}
	f, err := syscall.Open(dir, syscall.O_RDONLY|syscall.O_DIRECTORY, uint32(0755))
	if err != nil {
		panic(err)
	}
	err = syscall.Flock(f, syscall.LOCK_EX|syscall.LOCK_NB)
	switch err {
	case nil:
		return true
	case syscall.EWOULDBLOCK:
		return false
	default:
		panic(err)
	}
}

func main() {
	if lock(runDir) {
		fmt.Printf("no lock found - daemonising\n")
		// TODO(jonboulle): actually daemonise?
		runDaemon()
	}
	fmt.Printf("found dumbw daemon - running as client\n")
	var flagIface string
	flag.StringVar(&flagIface, "iface", "wlp2s0", "interface for statz")
	flag.Parse()
	args := flag.Args()
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "usage: %v [--iface IFACE] type\n", os.Args[0])
		os.Exit(1)
	}
	method := args[0]
	client, err := rpc.DialHTTP("unix", sockFile)
	if err != nil {
		log.Fatalf("dialing:", err)
	}
	m := fmt.Sprintf("StatsMap.%s", method)
	switch method {
	case "TxRate", "RxRate":
		var r Rate
		err := client.Call(m, flagIface, &r)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		} else {
			fmt.Printf("%v %v %v\n", flagIface, method, r)
		}
	case "TxBytes", "RxBytes":
		var b uint64
		err := client.Call(m, flagIface, &b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		} else {
			fmt.Printf("%v %v %v\n", flagIface, method, b)
		}

	default:
		log.Fatalf("unknown method: %v", method)
	}
}

// never returns
func runDaemon() {
	ss := StatsMap{}
	go serve(ss)
	for {
		select {
		case <-time.After(interval):
			for iface, v := range getSnapshot() {
				s, ok := ss[iface]
				if !ok {
					s = stats{}
				}
				rxr := Rate((float64(v.rxBytes) - float64(s.rxBytes)) / (float64(interval) / float64(time.Second)))
				txr := Rate((float64(v.txBytes) - float64(s.txBytes)) / (float64(interval) / float64(time.Second)))
				s.rxBytes = v.rxBytes
				s.txBytes = v.txBytes
				s.rxRate = rxr
				s.txRate = txr
				ss[iface] = s
			}
			dumpStats(ss)

		}
	}
}

func (ss StatsMap) RxBytes(iface string, bytes *uint64) error {
	s, ok := ss[iface]
	if !ok {
		return fmt.Errorf("no such interface: %q", iface)
	}
	*bytes = s.rxBytes
	return nil
}

func (ss StatsMap) TxBytes(iface string, bytes *uint64) error {
	s, ok := ss[iface]
	if !ok {
		return fmt.Errorf("no such interface: %q", iface)
	}
	*bytes = s.txBytes
	return nil
}

func (ss StatsMap) RxRate(iface string, Bps *Rate) error {
	s, ok := ss[iface]
	if !ok {
		return fmt.Errorf("no such interface: %q", iface)
	}
	*Bps = s.rxRate
	return nil
}

func (ss StatsMap) TxRate(iface string, Bps *Rate) error {
	s, ok := ss[iface]
	if !ok {
		return fmt.Errorf("no such interface: %q", iface)
	}
	*Bps = s.txRate
	return nil
}

func serve(ss StatsMap) {
	rpc.Register(ss)
	rpc.HandleHTTP()
	os.Remove(sockFile)
	l, err := net.Listen("unix", sockFile)
	if err != nil {
		panic(err)
	}
	http.Serve(l, nil)
}

func dumpStats(ss map[string]stats) {
	for _, iface := range sortedKeys(ss) {
		s := ss[iface]
		fmt.Printf("%s:\n", iface)
		fmt.Printf("\ttxBytes: %d\n", s.txBytes)
		fmt.Printf("\trxBytes: %d\n", s.rxBytes)
		fmt.Printf("\ttxRate:  %s\n", s.txRate)
		fmt.Printf("\trxRate:  %s\n", s.rxRate)
	}
}

func sortedKeys(m map[string]stats) (s []string) {
	for key, _ := range m {
		s = append(s, key)
	}
	sort.Strings(s)
	return
}

func getSnapshot() map[string]snapshot {
	f, err := os.Open(procNetDev)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	snap := map[string]snapshot{}
	s := bufio.NewScanner(f)
	i := 0
	for s.Scan() {
		i++
		if i <= 2 {
			continue
		}
		line := s.Text()
		tokens := strings.Fields(line)
		iface := strings.TrimSuffix(tokens[iIface], ":")
		rxBytes, err := strconv.ParseUint(tokens[iRxBytes], 10, 64)
		if err != nil {
			panic(err)
		}
		txBytes, err := strconv.ParseUint(tokens[iTxBytes], 10, 64)
		if err != nil {
			panic(err)
		}
		snap[iface] = snapshot{
			rxBytes: rxBytes,
			txBytes: txBytes,
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}
	return snap
}
