package scanner

import (
	"encoding/binary"
	"net"
)

// This file decodes the owner-PID socket tables returned by the Windows
// IP Helper API (GetExtendedTcpTable / GetExtendedUdpTable). It has no
// build tag so the decoding can be unit tested on every platform; only
// backend_windows.go calls the API itself.
//
// Every table starts with a DWORD row count followed by fixed-size rows.
// DWORDs are little-endian. Addresses are stored in network byte order,
// and ports occupy the first two bytes of their DWORD in network byte
// order, so both are read as raw bytes rather than as integers.

const (
	tcp4RowSize = 24 // MIB_TCPROW_OWNER_PID
	tcp6RowSize = 56 // MIB_TCP6ROW_OWNER_PID
	udp4RowSize = 12 // MIB_UDPROW_OWNER_PID
	udp6RowSize = 28 // MIB_UDP6ROW_OWNER_PID
)

// mibTCPStates maps MIB_TCP_STATE values to portop's Linux-style names.
var mibTCPStates = map[uint32]State{
	1:  StateClose, // MIB_TCP_STATE_CLOSED
	2:  StateListen,
	3:  StateSynSent,
	4:  StateSynRecv,
	5:  StateEstablished,
	6:  StateFinWait1,
	7:  StateFinWait2,
	8:  StateCloseWait,
	9:  StateClosing,
	10: StateLastAck,
	11: StateTimeWait,
	12: StateClose, // MIB_TCP_STATE_DELETE_TCB
}

func tableRows(buf []byte, rowSize int) [][]byte {
	if len(buf) < 4 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(buf))
	var rows [][]byte
	for i := 0; i < n; i++ {
		off := 4 + i*rowSize
		if off+rowSize > len(buf) {
			break
		}
		rows = append(rows, buf[off:off+rowSize])
	}
	return rows
}

func mibIP4(b []byte) net.IP  { return net.IPv4(b[0], b[1], b[2], b[3]).To4() }
func mibIP6(b []byte) net.IP  { return append(net.IP(nil), b[:16]...) }
func mibPort(b []byte) uint16 { return uint16(b[0])<<8 | uint16(b[1]) }
func mibDword(b []byte) uint32 {
	return binary.LittleEndian.Uint32(b)
}

func tcpState(v uint32) State {
	if s, ok := mibTCPStates[v]; ok {
		return s
	}
	return StateUnknown
}

func udpState(remotePort uint16) State {
	if remotePort != 0 {
		return StateEstablished
	}
	return StateUnconn
}

func parseTCP4Table(buf []byte) []Connection {
	var out []Connection
	for _, r := range tableRows(buf, tcp4RowSize) {
		c := Connection{
			Protocol:   TCP,
			State:      tcpState(mibDword(r[0:])),
			LocalAddr:  mibIP4(r[4:]),
			LocalPort:  mibPort(r[8:]),
			RemoteAddr: mibIP4(r[12:]),
			RemotePort: mibPort(r[16:]),
			PID:        int(mibDword(r[20:])),
		}
		if c.State == StateListen {
			c.RemotePort = 0 // Windows leaves garbage here for listeners
		}
		out = append(out, c)
	}
	return out
}

func parseTCP6Table(buf []byte) []Connection {
	var out []Connection
	for _, r := range tableRows(buf, tcp6RowSize) {
		c := Connection{
			Protocol:   TCP,
			IPv6:       true,
			LocalAddr:  mibIP6(r[0:]),
			LocalPort:  mibPort(r[20:]),
			RemoteAddr: mibIP6(r[24:]),
			RemotePort: mibPort(r[44:]),
			State:      tcpState(mibDword(r[48:])),
			PID:        int(mibDword(r[52:])),
		}
		if c.State == StateListen {
			c.RemotePort = 0
		}
		out = append(out, c)
	}
	return out
}

func parseUDP4Table(buf []byte) []Connection {
	var out []Connection
	for _, r := range tableRows(buf, udp4RowSize) {
		out = append(out, Connection{
			Protocol:   UDP,
			LocalAddr:  mibIP4(r[0:]),
			LocalPort:  mibPort(r[4:]),
			RemoteAddr: net.IPv4zero,
			State:      udpState(0),
			PID:        int(mibDword(r[8:])),
		})
	}
	return out
}

func parseUDP6Table(buf []byte) []Connection {
	var out []Connection
	for _, r := range tableRows(buf, udp6RowSize) {
		out = append(out, Connection{
			Protocol:   UDP,
			IPv6:       true,
			LocalAddr:  mibIP6(r[0:]),
			LocalPort:  mibPort(r[20:]),
			RemoteAddr: net.IPv6zero,
			State:      udpState(0),
			PID:        int(mibDword(r[24:])),
		})
	}
	return out
}
