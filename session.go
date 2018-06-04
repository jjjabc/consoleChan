package consoleChan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
)

var (
	ErrNeedPassword = errors.New("Need to input password")
	ErrTimeout      = errors.New("timeout")
)

const (
	CR = "\r\n"
)

const (
	PromptStd = iota
	PromptEnable
	PromptPassword
	PromptLogin
	PromptUnknow
)
const (
	MORE_STRING = "-MORE-"
	More_STRING = "-More-"
	more_STRING = "-more-"
)
const (
	LoginKey    = "login"
	PasswordKey = "pwd"
	EnableKey   = "enable"
	StandKey    = "std"
)
const (
	JumpDone = "done"
)

type PromptType int

type Session struct {
	runFlag    chan bool
	in         chan<- string
	out        chan string
	readErr    chan error
	Stderr     <-chan string
	hostname   string
	moreString map[string]string
	consoleIn  io.Writer
	consoleOut io.Reader
	consoleErr io.Reader
	rawSession io.Closer
	prompt     map[string]string
}

func newConsoleSession() *Session {
	s := &Session{}
	s.prompt = make(map[string]string)
	s.moreString = make(map[string]string)
	s.prompt[LoginKey] = "ogin:"
	s.prompt[PasswordKey] = "assword:"
	s.moreString[MORE_STRING] = MORE_STRING
	s.moreString[More_STRING] = More_STRING
	s.moreString[more_STRING] = more_STRING

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
	if s.rawSession == nil {
		err = fmt.Errorf("session not connected")
		return
	}
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
	if pType == PromptPassword || pType == PromptLogin {
		err = ErrNeedPassword
		return
	}
	_, err = s.consoleIn.Write([]byte(cmd + CR))
	if err != nil {
		return
	}
	reply, err = s.readReply(timeout, false, cmd)
	buf := bytes.Buffer{}
	cFlag := false
	for _, c := range reply {
		if c == 13 {
			cFlag = true
			continue
		}
		if cFlag {
			buf.Write([]byte{13})
			cFlag = false
		}
		buf.Write([]byte{byte(c)})
	}
	reply = buf.String()
	return
}
func (s *Session) login(username, password string) error {
	pType, err := s.findPrompt(true)
	if err != nil {
		return fmt.Errorf("console login error(enter username):" + err.Error())
	}
	if pType == PromptLogin {
		_, err = s.consoleIn.Write([]byte(username + CR))
		if err != nil {
			return fmt.Errorf("console login error(enter username):%s", err.Error())
		}
		pType, err = s.findPrompt(false)
		if err != nil {
			return fmt.Errorf("login err:" + err.Error())
		}
	}
	if pType == PromptPassword {
		s.consoleIn.Write([]byte(password + CR))
		if err != nil {
			return fmt.Errorf("console login error(enter password):%s", err.Error())
		}
	}
	pType, err = s.findPrompt(true)
	/*	if pType == PromptStd || pType == PromptEnable {
			// 如無密碼，需回車確認
			pType, err = s.findPrompt(true)
		} else {
			pType, err = s.findPrompt(false)
		}*/
	if err != nil {
		return fmt.Errorf("login err:" + err.Error())
	}
	if pType != PromptStd && pType != PromptEnable {
		return fmt.Errorf("login err:PromptTypeId is %d,maybe username or password wrong!", pType)
	}
	return nil
}
func (s *Session) telnetJump(address, username, pwd string, timeout time.Duration) error {
	addr := strings.Split(address, ":")
	if len(addr) != 2 {
		fmt.Errorf("地址格式错误")
	}
	reply, err := s.Cmd("telnet "+addr[0]+" "+addr[1], timeout)
	if err != nil {
		return fmt.Errorf("进行跳转登录失败：%s", err.Error())
	}
	if !strings.Contains(reply, JumpDone) {
		return fmt.Errorf("跳转登录失败：%s", reply)
	}
	log.Printf(reply)
	return s.login(username, pwd)
}
func (s *Session) Enable(password string) error {
	var err error
	if s.rawSession == nil {
		return fmt.Errorf("session not connected")
	}
	select {
	case s.runFlag <- true:
		defer func() { <-s.runFlag }()
	default:
		return fmt.Errorf("console isn't ready")
	}
	s.readReply(10*time.Millisecond, false)
	/*	pType, err := s.findPrompt(true)
		if err != nil {
			return err

		if pType == PromptEnable {
			return nil
		}
		if pType == PromptStd*/{
		s.consoleIn.Write([]byte("enable" + CR))
		reply, err := s.readReply(time.Second, false)
		if err != nil && err != ErrNeedPassword {
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
	_, err = s.consoleIn.Write([]byte(password + CR))
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
func (s *Session) GetReply(time time.Duration) (string, error) {
	return s.readReply(time, false)
}
func (s *Session) findPrompt(needCRFirst bool) (PromptType, error) {
	if needCRFirst {
		s.consoleIn.Write([]byte(CR))
	}
	var err error
	var reply string
	for retryCount := 0; retryCount < 5; retryCount++ {
		// 确保读取到最后一个提示符
		for {
			r, err := s.readReply(200*time.Millisecond, true)
			if err == ErrTimeout {
				err = nil
				break
			}
			if err != nil {
				return -1, err
			}
			reply = reply + r
		}
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
						if len(reply) == len(p) || reply[len(reply)-len(p)-1:] == " "+p {
							continue
						}
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
	}
	if reply == "" {
		return -1, fmt.Errorf("设备无回应")
	}
	replys := strings.SplitAfter(reply, CR)
	var prompt = ""
	if len(replys) > 0 {
		prompt = replys[len(replys)-1]
	}
	return -1, fmt.Errorf("Finding prompt error:prompt is incorrect,prompt is \"%s\":%s", prompt, reply)
}
func (s *Session) readReply(timeout time.Duration, needPorpmt bool, startWith ...string) (reply string, err error) {
	err = nil
	for {
		lastPartOfReply := ""
	readFor:
		for {
			select {
			case str, ok := <-s.out:
				lastPartOfReply = lastPartOfReply + str
				if !ok {
					reply = reply + lastPartOfReply
					return reply, fmt.Errorf("连接被关闭")
				}
				for {
					select {
					case str, ok := <-s.out:
						if !ok {
							reply = reply + lastPartOfReply
							return reply, fmt.Errorf("连接被关闭")
						}
						lastPartOfReply = lastPartOfReply + str
					default:
						break readFor
					}
				}
			case err = <-s.readErr:
				select {
				case s, ok := <-s.out:
					reply = reply + s
					if !ok {
						err = fmt.Errorf("连接被关闭")
					}
				default:
				}
				if err == io.EOF {
					err = fmt.Errorf("远程设备关闭链接:%s", err.Error())
				}
				return
			case <-time.After(timeout):
				err = ErrTimeout
				return
			}
		}
		reply = reply + lastPartOfReply
		if isLastString(lastPartOfReply, s.moreString) {
			s.consoleIn.Write([]byte(" "))
		} else if isLastString(lastPartOfReply, s.prompt) {
			return
			/*			if len(startWith) == 0 {
							return
						} else if isBeginString(reply, startWith[0]) {
							return
						}*/
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
	if s.rawSession != nil {
		return s.rawSession.Close()
	}
	return nil
}
func (s *Session) IsSessionOK() (isOk bool) {
	if s.rawSession != nil {
		defer func() {
			if recover() != nil {
				isOk = false
				return
			}
		}()
		s.out <- "" // panic if ch is closed
		return true
	}
	return false
}
func (s *Session) Wait() {
	buf := make([]byte, 64*1024)
	out := make(chan string, 1024)
	s.readErr = make(chan error, 1)
	s.out = out
	result := ""
	go func() {
		for {
			n, err := s.consoleOut.Read(buf)
			result = result + string(buf[:n])
			if err != nil {
				log.Printf("consoleOut close:" + err.Error())
				out <- result
				s.readErr <- err
				//todo err handle
				close(out)
				s.Close()
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
func (s *Session) SetMoreStr(key, moreStr string) {
	s.moreString[key] = moreStr
}
func (s *Session) SetPrompt(key, promptStr string) {
	s.prompt[key] = promptStr
}
func isContainsString(s string, subStrMap map[string]string) bool {
	for _, subStr := range subStrMap {
		if strings.Contains(s, subStr) {
			return true
		}
	}
	return false
}
func isLastString(s string, subStrMap map[string]string) bool {
	for k, subStr := range subStrMap {
		if len(s) >= len(subStr) && s[len(s)-len(subStr):] == subStr {
			if k == EnableKey {
				if len(s) == len(subStr) || s[len(s)-len(subStr)-1:] == " "+subStr {
					continue
				}
			}
			return true
		}
	}
	return false
}
func isBeginString(s string, sub string) bool {
	return len(s) >= len(sub) && s[:len(sub)] == sub
}
