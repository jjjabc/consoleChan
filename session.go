package consoleChan

import (
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
	"bytes"
)

var (
	ErrNeedPassword = errors.New("Need to input password")
	a               = 0
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
	Stderr                                               <-chan string
	buf                                                  []byte
	hostname                                             string
	moreString                                           string
	consoleIn                                            io.Writer
	consoleOut                                           io.Reader
	consoleErr                                           io.Reader
	rawSession                                           io.Closer
	promptStd, promptEnable, promptPassword, promptLogin string
	b *bytes.Buffer
}

func newConsoleSession() *Session {
	s := &Session{}
	s.buf=make([]byte,1024*1024)
	s.b=bytes.NewBuffer([]byte{})
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
	a = 1
	pType, err := s.findPrompt(true)
	a = 0
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
	if a == 1 {
		log.Printf(reply)
	}
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
	var (
		t, oldT int = 0, 0
	)
	err = nil
	finish := make(chan bool)
	renew := make(chan bool)
	if a == 1 {
		log.Printf("%v", renew)
	}
	go func(renew chan bool) {
		var n int
		for {
			n, err = s.b.Read(s.buf[t:])
			if err != nil {
				if err == io.EOF {
					err=nil
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
			if a == 1 {
				log.Printf("%v", strings.Contains(string(s.buf[oldT:t]), s.promptEnable) || strings.Contains(string(s.buf[oldT:t]), s.promptStd))
			}

			renew <- true
			if a == 1 {
				log.Printf("0000")
			}

			if strings.Contains(string(s.buf[oldT:t]), s.promptEnable) ||
				strings.Contains(string(s.buf[oldT:t]), s.promptStd) {

				if needPorpmt {
					// 准备提示符
					//s.consoleIn.Write([]byte("\n"))
				}
				if a == 1 {
					log.Printf("1111")
				}

				finish <- true
				return
			} else if strings.Contains(reply, s.moreString) {
				// 处理一屏幕显示不完情况，需要输入空格
				s.consoleIn.Write([]byte(" "))
			} else if strings.Contains(string(s.buf[oldT:t]), s.promptPassword) {
				//err = ErrNeedPassword
				if needPorpmt {
					// 准备提示符
					//s.consoleIn.Write([]byte("\n"))
				}
				finish <- false
				return
			} else if strings.Contains(string(s.buf[oldT:t]), s.promptLogin) {
				finish <- false
				return
			}
		}
	}(renew)
	for {
		select {
		case <-finish:
			return
		case <-renew:
			if a == 1 {
				log.Printf("renew")
			}
			continue
		case <-time.After(timeout):
			err = fmt.Errorf("read reply timeout")
			return
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
func (s *Session) Wait()  {
	for {
		n, err := s.b.Read(s.buf)
		if err != nil {
				s.b.Write(s.buf[:n])
				return

		}
		s.b.Write(s.buf[:n])
	}
}
