package consoleChan

import (
	"github.com/goburrow/serial"
	"time"
)

func SerialOpen(comName string, baudrate, dataBits, stopBits int, parity string, username, password string, timeout time.Duration) (*Session, error) {
	conf := &serial.Config{Address: comName, BaudRate: baudrate, DataBits: dataBits, StopBits: stopBits, Parity: parity, Timeout: timeout}
	s, err := serial.Open(conf)
	if err != nil {
		return nil, err
	}
	session := newConsoleSession()
	session.rawSession = s
	session.IOHandle(s, s, nil)
	err = session.login(username, password)
	if err != nil {
		return nil, err
	}
	return session, nil
}
