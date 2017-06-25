// Golang port of Fake SNTP server (written in Perl) http://jjy.nict.go.jp/tsp/link/leap.html

package main

import (
	"encoding/binary"
	"flag"
	"log"
	"net"
	"time"
)

var positiveLeap, negativeLeep bool
var ntpOffset = int64(time.Unix(0, 0).Sub(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) / time.Second)
var trueStart time.Time
var fakeStart time.Time
var drift float64

func main() {
	log.SetFlags(log.Ldate | log.Lmicroseconds)

	flag.BoolVar(&positiveLeap, "p", false, "insert positive leap second")
	flag.BoolVar(&negativeLeep, "n", false, "delete negative leap second")
	flag.Float64Var(&drift, "d", 0, "drift[PPM]")
	flag.Parse()

	trueStart = time.Now()
	if flag.NArg() >= 1 {
		var err error
		fakeStart, err = time.Parse(time.RFC3339, flag.Arg(0))
		if err != nil {
			panic(err)
		}
	} else {
		fakeStart = trueStart
	}

	addr, err := net.ResolveUDPAddr("udp", ":ntp")
	if err != nil {
		panic(err)
	}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	for {
		var data [256]byte
		n, remote, err := conn.ReadFromUDP(data[:])
		if err != nil {
			panic(err)
		}
		go handle(conn, data[:n], remote, time.Now())
	}
}

func toNTP(t time.Time) uint64 {
	d := t.Sub(trueStart)
	d += time.Duration(float64(d) * drift / 1e6)
	t = fakeStart.Add(d)
	unix := t.Unix()

	days := (unix - fakeStart.Truncate(24*time.Hour).Unix()) / (24 * 60 * 60)
	if positiveLeap {
		unix -= days
	}
	if negativeLeep {
		unix += days
	}

	ret := uint64(unix+ntpOffset) << 32
	ret |= uint64(float64(t.Nanosecond()) / 1e9 * (1 << 32))
	return ret
}

func fromNTP(t uint64) time.Time {
	return time.Unix(int64(t>>32)-ntpOffset, int64(float64(t&0xFFFFFFFF)/(1<<32)*1e9))
}

func handle(conn *net.UDPConn, data []byte, remote *net.UDPAddr, t time.Time) {
	if len(data) < 48 {
		return
	}
	big := binary.BigEndian
	log.Printf("recv from %s, transmit at %s", remote, fromNTP(big.Uint64(data[40:])))

	const PositiveLeapFlag = 0x40
	const NegativeLeapFlag = 0x80
	const str = 1           // Stratum 1
	const mode = 4          // Mode 4 (server)
	const poll = 4          // Poll Interval (2^4 sec)
	const prec = 0x100 - 16 // Precision (2^-16 sec)

	vn := data[0] & 0x38 // Version Number
	flag := vn | mode
	if positiveLeap {
		flag |= PositiveLeapFlag
	}
	if negativeLeep {
		flag |= NegativeLeapFlag
	}
	data[0] = flag
	data[1] = str                      // Stratum
	data[2] = poll                     // Poll Interval
	data[3] = prec                     // Precision
	big.PutUint32(data[4:], 0)         // Root Delay
	big.PutUint32(data[8:], 0x10)      // Root Dispersion
	copy(data[12:], "LOCL")            // Ref ID
	big.PutUint64(data[16:], toNTP(t)) // Reference Timestamp
	copy(data[24:], data[40:48])       // Originate Timestamp
	big.PutUint64(data[32:], toNTP(t)) // Receive Timestamp
	u := time.Now()
	big.PutUint64(data[40:], toNTP(u)) // Transmit Timestamp
	conn.WriteToUDP(data, remote)
}
