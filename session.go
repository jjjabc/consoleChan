package consoleChan

import (
	"errors"
	"fmt"
	"golang.org/x/crypto/ssh"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"
)

var (
	ErrNeedPassword = errors.New("Need to input password")
)

const (
	PromptStd = iota
	PromptEnable
	PromptPassword
)

type PromptType int
type Session interface {
	SetHostname(hostname string)
	Cmd(cmd string, timeout time.Duration) (reply string, err error)
	Enable(password string) error
	Close()
}

type SshSession struct {
	runFlag                                 chan bool
	in                                      chan<- string
	Stderr                                  <-chan string
	buf                                     [65 * 1024]byte
	hostname                                string
	moreString                              string
	consoleIn                               io.Writer
	consoleOut                              io.Reader
	consoleErr                              io.Reader
	golangSession                           *ssh.Session
	promptStd, promptEnable, promptPassword string
}

func newSshSession() *SshSession {
	s := &SshSession{}
	s.promptPassword = "password:"
	s.runFlag = make(chan bool, 1)
	return s
}
func (s *SshSession) SetHostname(hostname string) {
	s.hostname = hostname
	s.promptStd = s.hostname + ">"
	s.promptEnable = s.hostname + "#"
}
func (s *SshSession) Cmd(cmd string, timeout time.Duration) (reply string, err error) {
	select {
	case s.runFlag <- true:
		defer func() { <-s.runFlag }()
	default:
		return "", fmt.Errorf("console isn't ready")
	}
	err = nil
	pType, err := s.findPrompt()
	if err != nil {
		return
	}
	if pType == PromptPassword {
		err = ErrNeedPassword
		return
	}
	_, err = s.consoleIn.Write([]byte(cmd + "\n"))
	if err != nil {
		return
	}
	return s.readReply(timeout,true)
}
func (s *SshSession) Enable(password string) error {
	select {
	case s.runFlag <- true:
		defer func() { <-s.runFlag }()
	default:
		return fmt.Errorf("console isn't ready")
	}
	pType, err := s.findPrompt()
	if err != nil {
		return err
	}
	if pType == PromptEnable {
		return nil
	}
	if pType == PromptStd {
		s.consoleIn.Write([]byte("enable\n"))
		reply, err := s.readReply(time.Second,false)
		if err != nil &&err!=ErrNeedPassword{
			log.Printf(reply)
			return fmt.Errorf("Cann't find password pormpt:" + err.Error())
		}
		if len(reply) >= len(s.promptEnable) &&
			strings.Compare(reply[len(reply)-len(s.promptEnable):], s.promptEnable) == 0 {
			return nil
		}
		if len(reply) <= len(s.promptPassword) ||
			strings.Compare(reply[len(reply)-len(s.promptPassword):], s.promptPassword) != 0 {
			return fmt.Errorf("Cann't find password pormpt!" + reply)
		}
	}
	_, err = s.consoleIn.Write([]byte(password+"\n"))
	if err != nil {
		return err
	}
	reply, err := s.readReply(time.Second,true)
	if err != nil {
		if err == ErrNeedPassword {
			return fmt.Errorf("Wrong password")
		}
		return fmt.Errorf("Input password error:"+err.Error())
	}
	if !strings.Contains(reply, s.promptEnable) {
		return fmt.Errorf("Enable :"+reply)
	}
	return nil
}
func (s *SshSession) findPrompt() (PromptType, error) {
	reply, err := s.readReply(time.Second,false)
	if err != nil {
		s.consoleIn.Write([]byte("\n"))
		reply, err = s.readReply(time.Second,false)
		if err != nil {
			return -1, fmt.Errorf("Finding prompt error:" + err.Error())
		}
	}
	if len(reply) >= (len(s.promptStd)) {
		if strings.Compare(reply[len(reply)-len(s.promptStd):], s.promptStd) == 0 {
			return PromptStd, nil
		} else if strings.Compare(reply[len(reply)-len(s.promptEnable):], s.promptEnable) == 0 {
			return PromptEnable, nil
		}
	} else if len(reply) >= len(s.promptPassword) {
		if strings.Compare(reply[len(reply)-len(s.promptPassword):], s.promptPassword) == 0 {
			return PromptPassword, nil
		}
	}
	replys := strings.SplitAfter(reply, "\n")
	return -1, fmt.Errorf("Finding prompt error:prompt is incorrect,prompt is \"%s\"", replys[len(replys)-1])
}
func (s *SshSession) readReply(timeout time.Duration,needPorpmt bool) (reply string, err error) {
	var (
		t, oldT int = 0, 0
	)
	err = nil
	finish := make(chan bool)
	renew := make(chan bool)

	go func() {
		var n int
		for {
			n, err = s.consoleOut.Read(s.buf[t:])
			if err != nil {
				if err == io.EOF {
					reply = string(s.buf[:t+n])
					finish <- false
					return
				}
				err = fmt.Errorf("read error:", err.Error())
				finish <- false
				return
			}
			oldT = t
			t += n
			reply = string(s.buf[:t])
			renew <- true
			if strings.Contains(string(s.buf[oldT:t]), s.promptEnable) ||
				strings.Contains(string(s.buf[oldT:t]), s.promptStd) {
				if needPorpmt{
				// 准备提示符
					s.consoleIn.Write([]byte("\n"))
				}
				finish <- true
				return
			} else if strings.Contains(reply, s.moreString) {
				// 处理一屏幕显示不完情况，需要输入空格
				s.consoleIn.Write([]byte(" "))
			} else if strings.Contains(string(s.buf[oldT:t]), s.promptPassword) {
				err = ErrNeedPassword
				if needPorpmt{
					// 准备提示符
					s.consoleIn.Write([]byte("\n"))
				}
				finish <- false
				return
			}
		}
	}()
	for {
		select {
		case <-finish:
			return
		case <-renew:
			continue
		case <-time.After(timeout):
			err = fmt.Errorf("read reply timeout")
			return
		}
	}

}
func (s *SshSession) IOHandle(w io.Writer, r, e io.Reader) {
	go func() {
		//todo output stderr
	}()
	s.consoleIn = w
	s.consoleOut = r
	s.consoleErr = e
}
func (s *SshSession) Close() {
	s.golangSession.Close()
}
func saveHostPublicKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	fmt.Printf("publicKey:\n%x\n", key.Marshal())
	return nil
}
func SshDial(addr, username, password string, moreStr string, timeout time.Duration) (Session, error) {
	// An SSH client is represented with a ClientConn.
	//
	// To authenticate with the remote server you must pass at least one
	// implementation of AuthMethod via the Auth field in ClientConfig,
	// and provide a HostKeyCallback.
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: saveHostPublicKey,
		Timeout:         timeout,
	}
	client, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return nil, fmt.Errorf("Failed to dial: ", err)
	}

	// Each ClientConn can support multiple interactive sessions,
	// represented by a Session.
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("Failed to create session: ", err)
	}

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	// Request pseudo terminal
	if err := session.RequestPty("vt100", 40, 80, modes); err != nil {
		return nil, fmt.Errorf("request for pseudo terminal failed: ", err)
	}

	w, err := session.StdinPipe()
	if err != nil {
		return nil, err
	}
	r, err := session.StdoutPipe()
	if err != nil {
		return nil, err
	}
	e, err := session.StderrPipe()
	if err != nil {
		return nil, err
	}

	sshSession := newSshSession()
	sshSession.moreString = moreStr
	sshSession.golangSession = session
	sshSession.IOHandle(w, r, e)
	if err := session.Shell(); err != nil {
		log.Fatal(err)
	}

	go session.Wait()
	return sshSession, nil
}
func MuxShell(w io.Writer, r, e io.Reader) (chan<- string, <-chan string) {
	in := make(chan string, 1)
	out := make(chan string, 1)
	var wWg, rWg sync.WaitGroup

	rWg.Add(1) //for the shell itself
	go func() {
		for cmd := range in {
			rWg.Add(1)
			w.Write([]byte(cmd + "\n"))
			wWg.Done()
			rWg.Wait()
		}
	}()

	go func() {
		var (
			buf [65 * 1024]byte
			t   int
		)
		for {
			wWg.Wait()
			n, err := r.Read(buf[t:])
			if err != nil {
				fmt.Println("read error:" + err.Error())
				close(in)
				close(out)
				return
			}
			t += n
			result := string(buf[:t])
			if strings.Contains(result, "username:") ||
				strings.Contains(result, "password:") ||
				strings.Contains(result, "router#") ||
				strings.Contains(result, "router>") {
				out <- string(buf[:t])
				t = 0
				rWg.Done()
				wWg.Add(1)
			} else if strings.Contains(result, "---MORE---") {
				//strings.Replace(result,"---MORE---","",-1)
				w.Write([]byte(" "))
			}
		}
	}()
	return in, out
}
