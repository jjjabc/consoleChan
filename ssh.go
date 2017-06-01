package consoleChan

import (
	"fmt"
	"golang.org/x/crypto/ssh"
	"log"
	"net"
	"time"
)

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

	sshSession := newConsoleSession()
	sshSession.rawSession = session
	sshSession.IOHandle(w, r, e)
	if err := session.Shell(); err != nil {
		log.Fatal(err)
	}

	go session.Wait()
	return sshSession, nil
}
