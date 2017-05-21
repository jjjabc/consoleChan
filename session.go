package consoleChan

import (
	"fmt"
	"strings"
	"io"
	"sync"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"time"
)

type Session interface {
	Cmd(cmd string, timeout time.Duration) (reply string, err error)
	Close()
}

type SshSession struct {
	runFlag	chan bool
	in         chan<- string
	Stderr        <-chan string
	buf        [65 * 1024]byte
	hostname   string
	moreString string
	consoleIn io.Writer
	consoleOut io.Reader
	consoleErr io.Reader
	golangSession *ssh.Session
}

func (s *SshSession) Cmd(cmd string, timeout time.Duration) (reply string, err error) {
	select {
	case s.runFlag<-true:
		defer func(){<-s.runFlag}()
	default:
		return "", fmt.Errorf("console isn't ready")
	}
	err = nil
	s.consoleIn.Write([]byte(cmd + "\n"))
	return s.readReply(timeout)
}
func (s *SshSession) readReply(timeout time.Duration) (reply string, err error) {
	var (
		t   int   = 0
	)
	err = nil
	finish := make(chan bool)

	go func() {
		var n int
		for {
			n, err = s.consoleOut.Read(s.buf[t:])
			if err != nil {
				if err == io.EOF {
					// 测试此处
					reply = string(s.buf[:t])
					finish <- true
					return
				}
				err = fmt.Errorf("read error:", err.Error())
				finish <- false
				return
			}
			t += n
			reply = string(s.buf[:t])
			if strings.Contains(reply, s.moreString) {
				// 处理一屏幕显示不完情况，需要输入空格
				s.consoleIn.Write([]byte(" "))
			}
		}
	}()
	select {
	case <-finish:
		return reply,err
	case <-time.After(timeout):
		return reply,fmt.Errorf("read reply timeout")
	}
}
func (s *SshSession) IOHandle(w io.Writer, r, e io.Reader) {
	go func() {
		//todo output stderr
	}()
	s.consoleIn=w
	s.consoleOut=r
	s.consoleErr=e
}
func (s *SshSession)Close()  {
	s.golangSession.Close()
}
func saveHostPublicKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	fmt.Printf("publicKey:\n%x\n", key.Marshal())
	return nil
}
func SshDial(addr, username, password string, moreStr string) (Session, error) {
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

	sshSession := new(SshSession)
	sshSession.moreString = moreStr
	sshSession.golangSession=session
	sshSession.IOHandle(w, r, e)
	if err := session.Shell(); err != nil {
		log.Fatal(err)
	}
	go session.Wait()
	return sshSession,nil
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
