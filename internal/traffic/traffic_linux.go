package traffic

import (
	"encoding/binary"
	"errors"
	"net"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

// Layouts from linux/inet_diag.h and linux/tcp.h.
const (
	inetDiagReqV2Len = 56 // family, protocol, ext, pad, states, sockid
	inetDiagMsgLen   = 72 // family, state, timer, retrans, sockid, expires, rqueue, wqueue, uid, inode
	inetDiagInfo     = 2  // INET_DIAG_INFO: the attribute carrying struct tcp_info
	tcpiBytesAcked   = 120
	tcpiBytesRecv    = 128
	tcpEstablished   = 1
	tcpListen        = 10
)

func sample() (map[Key]Counter, error) {
	fd, err := unix.Socket(unix.AF_NETLINK, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, unix.NETLINK_SOCK_DIAG)
	if err != nil {
		return nil, err
	}
	defer unix.Close(fd)
	if err := unix.Bind(fd, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return nil, err
	}
	tv := unix.Timeval{Sec: 2}
	_ = unix.SetsockoptTimeval(fd, unix.SOL_SOCKET, unix.SO_RCVTIMEO, &tv)

	var socks []diagSocket
	for i, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		if err := dump(fd, family, uint32(i+1), &socks); err != nil {
			if family == unix.AF_INET6 {
				continue // IPv6 disabled
			}
			return nil, err
		}
	}
	return counters(socks), nil
}

// diagSocket is one socket of a sock_diag dump.
type diagSocket struct {
	state      uint8
	src, dst   net.IP
	sport      uint16
	dport      uint16
	acked, rcv uint64 // tcpi_bytes_acked, tcpi_bytes_received
}

// counters turns the established sockets into Counters. bytes_acked counts
// the SYN of a connection this host opened, but starts at zero on one it
// accepted; a connection whose local address has a listener was accepted.
func counters(socks []diagSocket) map[Key]Counter {
	listening := map[string]bool{}
	for _, s := range socks {
		if s.state == tcpListen {
			listening[listenKey(s.src, s.sport)] = true
		}
	}
	out := map[Key]Counter{}
	for _, s := range socks {
		if s.state != tcpEstablished {
			continue
		}
		tx := s.acked
		accepted := listening[listenKey(s.src, s.sport)] || listening[listenKey(net.IPv6unspecified, s.sport)]
		if !accepted && tx > 0 {
			tx--
		}
		out[KeyFor(s.src, s.sport, s.dst, s.dport)] = Counter{TX: tx, RX: s.rcv}
	}
	return out
}

// listenKey matches a listener whatever its family: 0.0.0.0 and :: are the
// same wildcard, and 127.0.0.1 is the same address as ::ffff:127.0.0.1.
func listenKey(ip net.IP, port uint16) string {
	if ip.IsUnspecified() {
		ip = net.IPv6unspecified
	}
	return net.JoinHostPort(ip.To16().String(), strconv.Itoa(int(port)))
}

// dump asks for every established or listening TCP socket of one address
// family, with its tcp_info, and appends them to socks.
func dump(fd int, family uint8, seq uint32, socks *[]diagSocket) error {
	req := make([]byte, unix.NLMSG_HDRLEN+inetDiagReqV2Len)
	binary.NativeEndian.PutUint32(req[0:], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:], unix.SOCK_DIAG_BY_FAMILY)
	binary.NativeEndian.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], seq)
	body := req[unix.NLMSG_HDRLEN:]
	body[0] = family
	body[1] = unix.IPPROTO_TCP
	body[2] = 1 << (inetDiagInfo - 1)
	binary.NativeEndian.PutUint32(body[4:], 1<<tcpEstablished|1<<tcpListen)
	if err := unix.Sendto(fd, req, 0, &unix.SockaddrNetlink{Family: unix.AF_NETLINK}); err != nil {
		return err
	}

	buf := make([]byte, 64*1024)
	for {
		n, _, err := unix.Recvfrom(fd, buf, 0)
		if err != nil {
			return err
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return err
		}
		for _, m := range msgs {
			switch m.Header.Type {
			case unix.NLMSG_DONE:
				return nil
			case unix.NLMSG_ERROR:
				return errors.New("traffic: sock_diag request refused")
			}
			if s, ok := parseDiagMsg(m.Data); ok {
				*socks = append(*socks, s)
			}
		}
	}
}

// parseDiagMsg reads one inet_diag_msg and its INET_DIAG_INFO attribute.
func parseDiagMsg(b []byte) (diagSocket, bool) {
	if len(b) < inetDiagMsgLen {
		return diagSocket{}, false
	}
	family, state := b[0], b[1]
	sport := binary.BigEndian.Uint16(b[4:])
	dport := binary.BigEndian.Uint16(b[6:])
	src, dst := net.IP(append([]byte(nil), b[8:24]...)), net.IP(append([]byte(nil), b[24:40]...))
	if family == unix.AF_INET {
		src, dst = src[:4], dst[:4]
	}
	for attrs := b[inetDiagMsgLen:]; len(attrs) >= 4; {
		l := int(binary.NativeEndian.Uint16(attrs[0:]))
		typ := binary.NativeEndian.Uint16(attrs[2:])
		if l < 4 || l > len(attrs) {
			break
		}
		if typ == inetDiagInfo && l-4 >= tcpiBytesRecv+8 {
			info := attrs[4:l]
			return diagSocket{
				state: state, src: src, dst: dst, sport: sport, dport: dport,
				acked: binary.NativeEndian.Uint64(info[tcpiBytesAcked:]),
				rcv:   binary.NativeEndian.Uint64(info[tcpiBytesRecv:]),
			}, true
		}
		attrs = attrs[(l+3)&^3:]
	}
	return diagSocket{}, false
}
