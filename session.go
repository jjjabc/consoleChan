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
	Cmd(cmd string) (reply string, err error)
}

type SshSession struct {
	in  chan<- string
	out <-chan string
}

func (s *SshSession) Cmd(cmd string) (reply string, err error) {
	err=nil
	select {
	case s.in<-cmd:
	default:
		return "",fmt.Errorf("console isn't ready")
	}
	s.in<-cmd
	timer:=time.NewTimer(time.Second)
	select {
	case  reply= <-s.out:
		return
	case <-timer.C:
		return "",fmt.Errorf("read Timeout")
	}
}
func saveHostPublicKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	fmt.Printf("publicKey:\n%x\n", key.Marshal())
	return nil
}
func (s *SshSession) Dial(addr, username, password string) error {
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
		return fmt.Errorf("Failed to dial: ", err)
	}

	// Each ClientConn can support multiple interactive sessions,
	// represented by a Session.
	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("Failed to create session: ", err)
	}
	defer session.Close()

	// Set up terminal modes
	modes := ssh.TerminalModes{
		ssh.ECHO:          0,     // disable echoing
		ssh.TTY_OP_ISPEED: 14400, // input speed = 14.4kbaud
		ssh.TTY_OP_OSPEED: 14400, // output speed = 14.4kbaud
	}
	// Request pseudo terminal
	if err := session.RequestPty("vt100", 40, 80, modes); err != nil {
		return fmt.Errorf("request for pseudo terminal failed: ", err)
	}

	w, err := session.StdinPipe()
	if err != nil {
		return err
	}
	r, err := session.StdoutPipe()
	if err != nil {
		return err
	}
	e, err := session.StderrPipe()
	if err != nil {
		return err
	}

	s.in, s.out = MuxShell(w, r, e)
	if err := session.Shell(); err != nil {
		log.Fatal(err)
	}
	<-s.out //ignore the shell output
	go func() {
		for {
			select {
			case s := <-s.out:
				fmt.Printf("%s", s)
			}
		}
	}()

	session.Wait()
	return nil
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

