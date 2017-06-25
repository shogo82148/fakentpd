// Golang port of Fake SNTP server (written in Perl) http://jjy.nict.go.jp/tsp/link/leap.html

package main

import (
	"encoding/binary"
	"log"
	"net"
	"time"
)

var offset = int64(time.Unix(0, 0).Sub(time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)) / time.Second)

func main() {
	log.SetFlags(log.Ldate | log.Lmicroseconds)
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
	ret := uint64(t.Unix()+offset) << 32
	ret |= uint64(float64(t.Nanosecond()) / 1e9 * (1 << 32))
	return ret
}

func fromNTP(t uint64) time.Time {
	return time.Unix(int64(t>>32), int64(float64(t&0xFFFFFFFF)/(1<<32)*1e9))
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
	data[0] = flag
	data[1] = str                               // Stratum
	data[2] = poll                              // Poll Interval
	data[3] = prec                              // Precision
	big.PutUint32(data[4:], 0)                  // Root Delay
	big.PutUint32(data[8:], 0x10)               // Root Dispersion
	copy(data[12:], "LOCL")                     // Ref ID
	big.PutUint64(data[16:], toNTP(t))          // Reference Timestamp
	copy(data[24:], data[40:48])                // Originate Timestamp
	big.PutUint64(data[32:], toNTP(t))          // Receive Timestamp
	big.PutUint64(data[40:], toNTP(time.Now())) // Transmit Timestamp
	conn.WriteToUDP(data, remote)
}
