package traffic

import (
	"encoding/binary"
	"errors"
	"net"
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

	out := map[Key]Counter{}
	for i, family := range []uint8{unix.AF_INET, unix.AF_INET6} {
		if err := dump(fd, family, uint32(i+1), out); err != nil {
			if family == unix.AF_INET6 {
				continue // IPv6 disabled
			}
			return nil, err
		}
	}
	return out, nil
}

// dump asks for every TCP socket of one address family with its tcp_info
// and adds the established ones' counters to out.
func dump(fd int, family uint8, seq uint32, out map[Key]Counter) error {
	req := make([]byte, unix.NLMSG_HDRLEN+inetDiagReqV2Len)
	binary.NativeEndian.PutUint32(req[0:], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:], unix.SOCK_DIAG_BY_FAMILY)
	binary.NativeEndian.PutUint16(req[6:], unix.NLM_F_REQUEST|unix.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], seq)
	body := req[unix.NLMSG_HDRLEN:]
	body[0] = family
	body[1] = unix.IPPROTO_TCP
	body[2] = 1 << (inetDiagInfo - 1)
	binary.NativeEndian.PutUint32(body[4:], 1<<1) // TCP_ESTABLISHED only
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
			if k, c, ok := parseDiagMsg(m.Data); ok {
				out[k] = c
			}
		}
	}
}

// parseDiagMsg reads one inet_diag_msg and its INET_DIAG_INFO attribute.
func parseDiagMsg(b []byte) (Key, Counter, bool) {
	if len(b) < inetDiagMsgLen {
		return Key{}, Counter{}, false
	}
	family := b[0]
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
			tx := binary.NativeEndian.Uint64(info[tcpiBytesAcked:])
			if tx > 0 {
				tx-- // the acknowledged SYN is counted as a byte
			}
			return KeyFor(src, sport, dst, dport), Counter{
				TX: tx,
				RX: binary.NativeEndian.Uint64(info[tcpiBytesRecv:]),
			}, true
		}
		attrs = attrs[(l+3)&^3:]
	}
	return Key{}, Counter{}, false
}
