package consoleChan

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
)

var (
	ErrNeedPassword = errors.New("Need to input password")
)

const (
	PromptStd = iota
	PromptEnable
	PromptPassword
	PromptLogin
)
const (
	MORE_STRING = "---MORE---"
)

type PromptType int

type Session struct {
	runFlag                                              chan bool
	in                                                   chan<- string
	out                                                  chan string
	readErr                                              chan error
	Stderr                                               <-chan string
	hostname                                             string
	moreString                                           string
	consoleIn                                            io.Writer
	consoleOut                                           io.Reader
	consoleErr                                           io.Reader
	rawSession                                           io.Closer
	promptStd, promptEnable, promptPassword, promptLogin string
}

func newConsoleSession() *Session {
	s := &Session{}
	s.promptLogin = "login:"
	s.promptPassword = "password:"
	s.runFlag = make(chan bool, 1)
	s.SetHostname("")
	return s
}
func (s *Session) SetHostname(hostname string) {
	s.hostname = hostname
	s.promptStd = s.hostname + ">"
	s.promptEnable = s.hostname + "#"
}
func (s *Session) Cmd(cmd string, timeout time.Duration) (reply string, err error) {
	select {
	case s.runFlag <- true:
		defer func() { <-s.runFlag }()
	default:
		return "", fmt.Errorf("console isn't ready")
	}
	err = nil
	pType, err := s.findPrompt(true)
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
	return s.readReply(timeout, false)
}
func (s *Session) login(username, password string) error {
	if username != "" {
		pType, err := s.findPrompt(false)
		if err != nil {
			return fmt.Errorf("console login error(enter username):" + err.Error())
		}
		if pType != PromptLogin {
			return fmt.Errorf("console not ready for login(enter username):prompt is %d", pType)
		}
		_, err = s.consoleIn.Write([]byte(username + "\n"))
		if err != nil {
			return fmt.Errorf("console login error(enter username):%s", err.Error())
		}
	}
	if password != "" {
		pType, err := s.findPrompt(false)
		if err != nil {
			return fmt.Errorf("console login error(enter password):" + err.Error())
		}
		if pType != PromptPassword {
			return fmt.Errorf("console not ready for login(enter password):prompt is %d", pType)
		}
		s.consoleIn.Write([]byte(password + "\n"))
		if err != nil {
			return fmt.Errorf("console login error(enter password):%s", err.Error())
		}
	}
	pType, err := s.findPrompt(false)
	if err != nil {
		return fmt.Errorf("login err:" + err.Error())
	}
	if pType != PromptStd {
		return fmt.Errorf("login err:maybe username or password wrong!")
	}
	return nil
}
func (s *Session) Enable(password string) error {
	select {
	case s.runFlag <- true:
		defer func() { <-s.runFlag }()
	default:
		return fmt.Errorf("console isn't ready")
	}
	s.readReply(10*time.Millisecond, false)
	pType, err := s.findPrompt(true)
	if err != nil {
		return err
	}
	if pType == PromptEnable {
		return nil
	}
	if pType == PromptStd {
		s.consoleIn.Write([]byte("enable\n"))
		reply, err := s.readReply(time.Second, false)
		log.Printf(reply)
		if err != nil && err != ErrNeedPassword {
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
	_, err = s.consoleIn.Write([]byte(password + "\n"))
	if err != nil {
		return err
	}
	reply, err := s.readReply(time.Second, true)
	if err != nil {
		if err == ErrNeedPassword {
			return fmt.Errorf("Wrong password")
		}
		return fmt.Errorf("Input password error:" + err.Error())
	}
	if !strings.Contains(reply, s.promptEnable) {
		return fmt.Errorf("Enable :" + reply)
	}
	return nil
}
func (s *Session) findPrompt(needCRFirst bool) (PromptType, error) {
	if needCRFirst {
		s.consoleIn.Write([]byte("\n"))
	}
	reply, err := s.readReply(time.Second, false)
	if err != nil {
		return -1, fmt.Errorf("Finding prompt error:" + err.Error())
	}
	if len(reply) >= (len(s.promptStd)) {
		if strings.Compare(reply[len(reply)-len(s.promptStd):], s.promptStd) == 0 {
			return PromptStd, nil
		} else if strings.Compare(reply[len(reply)-len(s.promptEnable):], s.promptEnable) == 0 {
			return PromptEnable, nil
		}
	}
	if len(reply) >= len(s.promptPassword) {
		if strings.Compare(reply[len(reply)-len(s.promptPassword):], s.promptPassword) == 0 {
			return PromptPassword, nil
		}
	}
	if len(reply) >= len(s.promptLogin) {
		if strings.Compare(reply[len(reply)-len(s.promptLogin):], s.promptLogin) == 0 {
			return PromptLogin, nil
		}
	}
	replys := strings.SplitAfter(reply, "\n")
	return -1, fmt.Errorf("Finding prompt error:prompt is incorrect,prompt is \"%s\"", replys[len(replys)-1])
}
func (s *Session) readReply(timeout time.Duration, needPorpmt bool) (reply string, err error) {
	err = nil
	for {
		lastPartOfReply := ""
	readFor:
		for {
			select {
			case s := <-s.out:
				lastPartOfReply = lastPartOfReply + s
			case err = <-s.readErr:
				select {
				case s := <-s.out:
					reply=reply+s
				default:
				}
				return
			case <-time.After(timeout):
				//err = fmt.Errorf("read reply timeout")
				return
			default:
				break readFor
			}
		}
		reply = reply + lastPartOfReply
		if strings.Contains(lastPartOfReply, s.promptEnable) ||
			strings.Contains(lastPartOfReply, s.promptStd) ||
			strings.Contains(lastPartOfReply, s.promptPassword) ||
			strings.Contains(lastPartOfReply, s.promptPassword) {
			return
		} else if strings.Contains(reply, s.moreString) {
			// 处理一屏幕显示不完情况，需要输入空格
			s.consoleIn.Write([]byte(" "))
		}
	}

}
func (s *Session) IOHandle(w io.Writer, r, e io.Reader) {
	go func() {
		//todo output stderr
	}()
	s.consoleIn = w
	s.consoleOut = r
	s.consoleErr = e
}
func (s *Session) Close() error {
	return s.rawSession.Close()
}
func (s *Session) Wait() {
	buf := make([]byte, 64*1024)
	out := make(chan string, 1024)
	s.out = out
	result := ""
	for {
		n, err := s.consoleOut.Read(buf)
		if err != nil {
			out <- result
			s.readErr <- err
			//todo err handle
			return

		}
		result = result + string(buf[:n])
		select {
		case out <- result:
			result = ""
		default:
		}

	}
}
