package consoleChan

import (
	"net"
	"time"
)

const (
	netType = "tcp"
)

func TelnetDial(addr, username, password string, timeout time.Duration) (*Session, error) {
	conn, err := net.DialTimeout(netType, addr, timeout)
	if err != nil {
		return nil, err
	}
	session := newConsoleSession()
	session.rawSession = conn
	session.IOHandle(conn, conn, nil)
	err = session.login(username, password)
	if err != nil {
		return nil, err
	}
	return session, nil
}
