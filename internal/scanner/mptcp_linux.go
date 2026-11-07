package scanner

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// Layouts and attribute numbers from linux/inet_diag.h.
const (
	mptcpDiagReqLen = 56
	mptcpDiagMsgLen = 72
	inetDiagInfo    = 2
	inetDiagULPInfo = 19
	inetULPInfoName = 1
)

func scanMPTCP() ([]Connection, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	if err := unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &unix.Timeval{Sec: 2}); err != nil {
		return nil, err
	}
	var out []Connection
	for i, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		conns, err := dumpMPTCP(fd, family, uint32(i+1))
		if err != nil {
			// IPv6 or its diagnostics may be disabled independently of IPv4.
			if family == unix.AF_INET6 {
				continue
			}
			return nil, err
		}
		out = append(out, conns...)
	}
	return out, nil
}

func mptcpRequest(family uint8, seq uint32) []byte {
	req := make([]byte, unix.NLMSG_HDRLEN+mptcpDiagReqLen)
	binary.NativeEndian.PutUint32(req, uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:], unix.SOCK_DIAG_BY_FAMILY)
	binary.NativeEndian.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], seq)
	body := req[unix.NLMSG_HDRLEN:]
	body[0] = family
	body[1] = unix.IPPROTO_TCP
	// INET_DIAG_INFO also requests the TCP upper-layer protocol name.
	body[2] = 1 << (inetDiagInfo - 1)
	binary.NativeEndian.PutUint32(body[4:], ^uint32(0))
	return req
}

func dumpMPTCP(fd int, family uint8, seq uint32) ([]Connection, error) {
	if err := unix.Sendto(fd, mptcpRequest(family, seq), 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	var out []Connection
	buf := make([]byte, 64*1024)
	for {
		n, _, flags, from, err := unix.Recvmsg(fd, buf, nil, 0)
		if err != nil {
			return nil, err
		}
		if peer, ok := from.(*unix.SockaddrNetlink); !ok || peer.Pid != 0 {
			continue
		}
		if flags&unix.MSG_TRUNC != 0 {
			return nil, fmt.Errorf("scanner: truncated MPTCP diagnostics")
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return nil, err
		}
		for _, msg := range msgs {
			if msg.Header.Seq != seq {
				continue
			}
			switch msg.Header.Type {
			case unix.NLMSG_DONE:
				if msg.Header.Flags&unix.NLM_F_DUMP_INTR != 0 {
					return nil, fmt.Errorf("scanner: interrupted MPTCP diagnostics")
				}
				if len(msg.Data) >= 4 {
					if err := mptcpDiagError(msg.Data); err != nil {
						return nil, err
					}
				}
				return out, nil
			case unix.NLMSG_ERROR:
				if err := mptcpDiagError(msg.Data); err != nil {
					return nil, err
				}
			case unix.SOCK_DIAG_BY_FAMILY:
				if c, ok := parseMPTCPDiag(msg.Data); ok {
					out = append(out, c)
				}
			}
		}
	}
}

func mptcpDiagError(data []byte) error {
	if len(data) < 4 {
		return fmt.Errorf("scanner: short MPTCP diagnostic error")
	}
	code := int32(binary.NativeEndian.Uint32(data))
	if code != 0 {
		return fmt.Errorf("scanner: MPTCP diagnostics: %w", syscall.Errno(-code))
	}
	return nil
}

func parseMPTCPDiag(data []byte) (Connection, bool) {
	if len(data) < mptcpDiagMsgLen || (data[0] != unix.AF_INET && data[0] != unix.AF_INET6) {
		return Connection{}, false
	}
	ulp := diagAttribute(data[mptcpDiagMsgLen:], inetDiagULPInfo)
	if !bytes.Equal(diagAttribute(ulp, inetULPInfoName), []byte("mptcp\x00")) {
		return Connection{}, false
	}
	size := net.IPv4len
	ipv6 := data[0] == unix.AF_INET6
	if ipv6 {
		size = net.IPv6len
	}
	state, ok := tcpStates[fmt.Sprintf("%02X", data[1])]
	if !ok {
		state = StateUnknown
	}
	return Connection{
		Protocol:   TCP,
		MPTCP:      true,
		LocalAddr:  append(net.IP(nil), data[8:8+size]...),
		LocalPort:  binary.BigEndian.Uint16(data[4:]),
		RemoteAddr: append(net.IP(nil), data[24:24+size]...),
		RemotePort: binary.BigEndian.Uint16(data[6:]),
		State:      state,
		UID:        binary.NativeEndian.Uint32(data[64:]),
		Inode:      uint64(binary.NativeEndian.Uint32(data[68:])),
		IPv6:       ipv6,
	}, true
}

func diagAttribute(attrs []byte, kind uint16) []byte {
	for len(attrs) >= 4 {
		length := int(binary.NativeEndian.Uint16(attrs))
		if length < 4 || length > len(attrs) {
			return nil
		}
		// Nested attributes may carry NLA_F_NESTED in their type.
		if binary.NativeEndian.Uint16(attrs[2:])&0x3fff == kind {
			return attrs[4:length]
		}
		aligned := (length + 3) &^ 3
		if aligned > len(attrs) {
			return nil
		}
		attrs = attrs[aligned:]
	}
	return nil
}

func markMPTCP(conns, subflows []Connection) {
	type endpoints struct {
		local, remote         string
		localPort, remotePort uint16
		ipv6                  bool
	}
	key := func(c Connection) endpoints {
		return endpoints{c.LocalAddr.String(), c.RemoteAddr.String(), c.LocalPort, c.RemotePort, c.IPv6}
	}
	found := make(map[endpoints]bool, len(subflows))
	for _, c := range subflows {
		found[key(c)] = true
	}
	for i := range conns {
		if conns[i].Protocol == TCP && found[key(conns[i])] {
			conns[i].MPTCP = true
		}
	}
}
