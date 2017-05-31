package consoleChan

import (
	"github.com/tarm/goserial"
)

func SerialOpen(comName string, baudrate int, username, password string) (*Session, error) {
	conf := &serial.Config{Name: comName, Baud: baudrate}
	s, err := serial.OpenPort(conf)
	if err != nil {
		return nil, err
	}
	session := newConsoleSession()
	session.rawSession = s
	session.moreString = MORE_STRING
	session.IOHandle(s, s, nil)
	err = session.login(username, password)
	if err != nil {
		return nil, err
	}
	return session, nil
}
