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
	PromptUnknow
)
const (
	MORE_STRING = "---MORE---"
)
const (
	LoginKey    = "login"
	PasswordKey = "pwd"
	EnableKey   = "enable"
	StandKey    = "std"
)

type PromptType int

type Session struct {
	runFlag    chan bool
	in         chan<- string
	out        chan string
	readErr    chan error
	Stderr     <-chan string
	hostname   string
	moreString string
	consoleIn  io.Writer
	consoleOut io.Reader
	consoleErr io.Reader
	rawSession io.Closer
	prompt     map[string]string
}

func newConsoleSession() *Session {
	s := &Session{}
	s.prompt = make(map[string]string)
	s.prompt[LoginKey] = "login:"
	s.prompt[PasswordKey] = "password:"
	s.runFlag = make(chan bool, 1)
	s.SetHostname("")
	return s
}
func (s *Session) SetHostname(hostname string) {
	s.hostname = hostname
	s.prompt[StandKey] = s.hostname + ">"
	s.prompt[EnableKey] = s.hostname + "#"
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
		if len(reply) >= len(s.prompt[EnableKey]) &&
			strings.Compare(reply[len(reply)-len(s.prompt[EnableKey]):], s.prompt[EnableKey]) == 0 {
			return nil
		}
		if len(reply) <= len(s.prompt[PasswordKey]) ||
			strings.Compare(reply[len(reply)-len(s.prompt[PasswordKey]):], s.prompt[PasswordKey]) != 0 {
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
	if !strings.Contains(reply, s.prompt[EnableKey]) {
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
	for k, p := range s.prompt {
		if len(reply) >= len(p) {
			if strings.Compare(reply[len(reply)-len(p):], p) == 0 {
				switch k {
				case StandKey:
					return PromptStd, nil
				case EnableKey:
					return PromptEnable, nil
				case PasswordKey:
					return PromptPassword, nil
				case LoginKey:
					return PromptLogin, nil
				default:
					return PromptUnknow, nil

				}

			}
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
			case str := <-s.out:
				lastPartOfReply = lastPartOfReply + str
				for {
					select {
					case str := <-s.out:
						lastPartOfReply = lastPartOfReply + str
					default:
						break readFor
					}
				}
			case err = <-s.readErr:
				select {
				case s := <-s.out:
					reply = reply + s
				default:
				}
				return
			case <-time.After(timeout):
				log.Printf("read reply timeout")
				return
				/*			default:
							log.Printf("break")
							break readFor*/
			}
		}
		reply = reply + lastPartOfReply
		if strings.Contains(reply, s.moreString) {
			// 处理一屏幕显示不完情况，需要输入空格
			s.consoleIn.Write([]byte(" "))
		} else {
			for _, p := range s.prompt {
				if strings.Contains(lastPartOfReply, p) {
					return
				}
			}
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
	s.Wait()
}
func (s *Session) Close() error {
	return s.rawSession.Close()
}
func (s *Session) Wait() {
	buf := make([]byte, 64*1024)
	out := make(chan string, 1024)
	s.out = out
	result := ""
	go func() {
		for {
			n, err := s.consoleOut.Read(buf)
			result = result + string(buf[:n])
			if err != nil {
				out <- result
				s.readErr <- err
				//todo err handle
				return

			}
			select {
			case out <- result:
				result = ""
			default:
			}

		}
	}()
}
