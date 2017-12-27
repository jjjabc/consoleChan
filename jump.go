package consoleChan

import (
	"fmt"
	"time"
)

// JumpDial for connect to and login target addr through jump host. Jump host to target just only use telnet protocol
// jumpType only "ssh" or "telnet"
func JumpDial(jumpHostAddr, jumpHostUsername, jumpHostPassword, jumpType, targetAddr, targetUsername, targetPassword string, timeout time.Duration) (*Session, error) {
	var session *Session
	var err error
	switch jumpType {
	case "ssh":
		session, err = SshDial(jumpHostAddr, jumpHostUsername, jumpHostPassword, timeout)
	case "telnet":
		session, err = TelnetDial(jumpHostAddr, jumpHostUsername, jumpHostPassword, timeout)
	default:
		return nil, fmt.Errorf("\"%s\" protocol isn't supported.We only support \"telnet\",\"ssh\"")
	}
	if err != nil {
		return nil, err
	}
	err = session.telnetJump(targetAddr, targetUsername, targetPassword, timeout)
	if err != nil {
		return nil, err
	}
	return session, err

}
