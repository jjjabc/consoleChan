package consoleChan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log"
	"strings"
	"time"
	"regexp"
)

var (
	ErrNeedPassword = errors.New("Need to input password")
	ErrTimeout      = errors.New("timeout")
)

const (
	CR = "\r\n"
)

const (
	PromptStd      = iota
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

var PromptWait = 200 * time.Millisecond

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
func (s *Session) Exec(cmd string, timeout time.Duration) (reply string, err error) {
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
	_, err = s.consoleIn.Write([]byte(cmd + CR))
	if err != nil {
		return
	}
	lastPartOfReply := ""
	for {
		select {
		case str, ok := <-s.out:
			lastPartOfReply = lastPartOfReply + str
			if !ok {
				reply = reply + lastPartOfReply
				return reply, fmt.Errorf("连接被关闭")
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
			reply = reply + lastPartOfReply
			return
		}
	}
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
	// 解决多个/r后接/n问题，如/r/r/r/n过滤为/r/n
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
			time.Sleep(5 * time.Second)
			tempErr := err.Error()
			pType, err = s.findPrompt(false)
			if err != nil {
				err = errors.New(tempErr)
			}
		}
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
		time.Sleep(5 * time.Second)
		tempErr := err.Error()
		pType, err = s.findPrompt(false)
		if err != nil {
			err = errors.New(tempErr)
		}
	}
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

const (
	pwdFlagStr  = "{pwd}"
	ipFlagStr   = "{ip}"
	portFlagStr = "{port}"
	userFlagStr = "{username}"
)

func (s *Session) jump(sshCmd, sshDone, telnetCmd, telnetDone string, address, username, pwd, connectType string, timeout time.Duration) error {
	addr := strings.Split(address, ":")
	if len(addr) != 2 {
		fmt.Errorf("地址格式错误")
	}
	var cmd, done string
	switch connectType {
	case "ssh":
		log.Println(sshCmd)
		cmd = sshCmd
		done = sshDone
	case "telnet":
		cmd = telnetCmd
		done = telnetDone
	default:
		return fmt.Errorf("不支持的连接方式(支持ssh或telnet):%s", connectType)
	}
	log.Println(cmd)
	cmd = strings.Replace(cmd, userFlagStr, username, -1)
	cmd = strings.Replace(cmd, pwdFlagStr, pwd, -1)
	cmd = strings.Replace(cmd, ipFlagStr, addr[0], -1)
	cmd = strings.Replace(cmd, portFlagStr, addr[1], -1)
	reply, err := s.Exec(cmd, PromptWait*15)
	if err != nil {
		return err
	}
	//log.Printf("%s\r\n%s",cmd,reply)
	if regexp.MustCompile(`(?i)(yes|y).*[\\/].*(no|n)`).Match([]byte(reply)) {
		reply, err = s.Exec("yes", PromptWait)
		if err != nil {
			return err
		}
		//log.Printf(reply)
	}
	if strings.Contains(reply, "assword:") {
		reply, err = s.Exec(pwd, PromptWait*15)
		if err != nil {
			return err
		}
		//log.Printf("%s\r\n%s",pwd,reply)
	}
	if reply == "" {
		return fmt.Errorf("设备无响应")
	}
	findDone := false
	dones := strings.Split(done, "|")
	for _, doneStr := range dones {
		if strings.Contains(reply, doneStr) {
			findDone = true
			break
		}
	}
	log.Printf(reply)
	if !findDone {
		return fmt.Errorf("跳转登录失败(期待%s):%s", done, reply)
	}
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
		reply, err := s.readReply(10*time.Second, false)
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
			reply = reply + r
			if err == ErrTimeout {
				err = nil
				break
			}
			if err != nil {
				return -1, err
			}
		}
		if err != nil {
			return -1, fmt.Errorf("Finding prompt error:" + err.Error())
		}
		replyTrim:=strings.TrimSpace(reply)
		for k, p := range s.prompt {
			if len(replyTrim) >= len(p) {
				if strings.Compare(replyTrim[len(replyTrim)-len(p):], p) == 0 {
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
	if timeout == 0 {
		return "", nil
	}
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
	checkPrompt:
		reply = reply + lastPartOfReply
		if isLastString(lastPartOfReply, s.moreString) {
			s.consoleIn.Write([]byte(" "))
		} else if isLastString(lastPartOfReply, s.prompt) {
			//遇到类似提示符符号，继续等待500ms如再无输出才判断命令执行完毕
			select {
			case str, _ := <-s.out:
				lastPartOfReply = lastPartOfReply + str
				// 如果还存在输出跳回字串检查处
				goto checkPrompt
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
			case <-time.After(PromptWait):
			}
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
	for _, subStr := range subStrMap {
		if len(s) >= len(subStr) && s[len(s)-len(subStr):] == subStr {
			//无需判断“ #这中情况”
			/*			if k == EnableKey||k == StandKey {
							if len(s) == len(subStr){
								return true
							}
							if len(s) == len(subStr) || s[len(s)-len(subStr)-1:] == " "+subStr {
								continue
							}
						}*/
			return true
		}
	}
	return false
}
func isBeginString(s string, sub string) bool {
	return len(s) >= len(sub) && s[:len(sub)] == sub
}
