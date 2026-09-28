//go:build !windows

package pairing

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/unix"
)

// listen opens the shared pairing port. SO_REUSEPORT lets several hubs on one
// machine (and other apps) share it; SO_BROADCAST lets us announce.
func listen(port int) (net.PacketConn, error) {
	lc := net.ListenConfig{Control: func(_, _ string, c syscall.RawConn) error {
		var serr error
		err := c.Control(func(fd uintptr) {
			for _, opt := range []int{unix.SO_REUSEADDR, unix.SO_REUSEPORT, unix.SO_BROADCAST} {
				if e := unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, opt, 1); e != nil && serr == nil {
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
