package consoleChan

import (
	"time"
	"net"
)
const (
	netType = "tcp"
)
func TelnetDial(addr, username, password string, timeout time.Duration) (*Session, error) {
	conn,err:=net.DialTimeout(netType,addr,timeout)
	if err != nil {
		return nil,err
	}
	session:=newConsoleSession()
	session.rawSession=conn
	session.moreString = MORE_STRING
	session.IOHandle(conn,conn,nil)
	err=session.login(username,password)
	if err != nil {
		return nil,err
	}
	//session.consoleIn.Write([]byte("\n"))
	return session,nil
}
