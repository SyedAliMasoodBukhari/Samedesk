package pairing

import (
	"context"
	"fmt"
	"net"
	"syscall"
)

// listen opens the shared pairing port with SO_REUSEADDR (so it can be shared)
// and SO_BROADCAST (so we can announce).
func listen(port int) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			for _, opt := range []int{syscall.SO_REUSEADDR, syscall.SO_BROADCAST} {
				if e := syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, opt, 1); e != nil && serr == nil {
					serr = e
				}
			}
		})
		if err != nil {
			return err
		}
		return serr
	}}
	return lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf("0.0.0.0:%d", port))
}
