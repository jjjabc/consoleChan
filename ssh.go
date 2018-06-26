package consoleChan

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"time"
)

type sshSession struct {
	client *ssh.Client
	*ssh.Session
}

func newSshSession(c *ssh.Client) (*sshSession, error) {
	sesson, err := c.NewSession()
	if err != nil {
		return nil, err
	}
	return &sshSession{client: c, Session: sesson}, nil
}
func (s *sshSession) Close() error {
	s.client.Close()

	err := s.Session.Close()
	if err != nil {
		return err
	}
	return s.client.Close()
}
func saveHostPublicKey(hostname string, remote net.Addr, key ssh.PublicKey) error {
	fmt.Printf("publicKey:\n%x\n", key.Marshal())
	return nil
}
func SshDial(addr, username, password string, timeout time.Duration) (*Session, error) {
	// An SSH client is represented with a ClientConn.
	//
	// To authenticate with the remote server you must pass at least one
	// implementation of AuthMethod via the Auth field in ClientConfig,
	// and provide a HostKeyCallback.
	config := ssh.ClientConfig{
		User: username,
		Auth: []ssh.AuthMethod{
			ssh.Password(password),
		},
		HostKeyCallback: saveHostPublicKey,
		Timeout:         timeout,
		Config: ssh.Config{
			Ciphers: []string{
				"aes128-ctr", "aes192-ctr", "aes256-ctr",
				"aes128-gcm@openssh.com",
				"arcfour256", "arcfour128", "aes128-cbc", "3des-cbc", "aes192-cbc", "aes256-cbc",
			},
		},
	}
	var client *ssh.Client
	var err error
	for retry := 0; ; retry++ {
		client, err = ssh.Dial("tcp", addr, &config)
		if err == nil {
			break
		}
		if retry > 2 {
			return nil, fmt.Errorf("Failed to dial: ", err)
		}
		time.Sleep(3 * time.Second * time.Duration(retry+1))
	}

	// Each ClientConn can support multiple interactive sessions,
	// represented by a Session.
	session, err := newSshSession(client)
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
	if err := session.RequestPty("xterm", 40, 80, modes); err != nil {
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

	sshSession := newConsoleSession()
	sshSession.rawSession = session
	sshSession.IOHandle(w, r, e)
	if err := session.Shell(); err != nil {
		log.Fatal(err)
	}

	//go session.Wait()
	return sshSession, nil
}
