package consoleChan

import (
	"fmt"
	"time"
)

// JumpDial for connect to and login target addr through jump host. Jump host to target just only use telnet protocol
// jumpType only "ssh" or "telnet"
func JumpDial(sshCmd, sshDone, telnetCmd, telnetDone, jumpHostAddr, jumpHostUsername, jumpHostPassword, jumpType, targetAddr, targetUsername, targetPassword, targetType string, timeout time.Duration) (*Session, error) {
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
		return nil, fmt.Errorf("connection(type:%s,address:%s) error:%s", jumpType, jumpHostAddr, err.Error())
	}
	/*err = session.telnetJump(targetAddr, targetUsername, targetPassword, timeout)*/
	err = session.jump(sshCmd, sshDone, telnetCmd, telnetDone, targetAddr, targetUsername, targetPassword, targetType, timeout)
	if err != nil {
		return nil, err
	}
	return session, err

}
