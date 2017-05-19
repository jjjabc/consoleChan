package consoleChan

type Session interface {
	Cmd(cmd string)(reply string,err error)
}