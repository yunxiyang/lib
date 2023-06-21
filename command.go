package lib

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"time"
)

type IConn interface {
	WriteBytes(b []byte)
	WriteString(s string)
	WriteStringf(format string, a ...interface{})
	WriteStringLn(s string)
	WriteJson(i interface{})
	WritePrettyJson(i interface{})
}

type Conn struct {
	net.Conn
	auth   int // 0 无权限 1 读权限 2读写权限
	closed bool
}

const Addr = "127.0.0.1:"

type Command struct {
	exit    chan struct{}
	conn    Conn
	execute interface{}
	content interface{}
}

func (cmd *Command) WriteBytes(b []byte) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	_, _ = cmd.conn.Write(b)
}

func (cmd *Command) WriteString(s string) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	_, _ = cmd.conn.Write([]byte(s))
}

func (cmd *Command) WriteStringf(format string, a ...interface{}) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	_, _ = cmd.conn.Write([]byte(fmt.Sprintf(format, a...)))
}

func (cmd *Command) WriteStringLn(s string) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	_, _ = cmd.conn.Write([]byte(s + "\n"))
}

func (cmd *Command) WriteJson(i interface{}) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	b, err := json.Marshal(i)
	if err != nil {
		_, _ = cmd.conn.Write([]byte(err.Error() + "\n"))
	} else {
		_, _ = cmd.conn.Write(b)
		_, _ = cmd.conn.Write([]byte("\n"))
	}
}

func (cmd *Command) WritePrettyJson(i interface{}) {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	b, err := json.Marshal(i)
	if err != nil {
		_, _ = cmd.conn.Write([]byte(err.Error() + "\n"))
	} else {
		_, _ = cmd.conn.Write(PrettyJSON(b))
		_, _ = cmd.conn.Write([]byte("\n"))
	}
}

func ListenCommand(port string, command interface{}, exit chan struct{}) (*Command, error) {
	cmd := Command{}
	cmd.exit = exit
	cmd.conn.auth = 0
	cmd.conn.closed = true

	cv := reflect.ValueOf(command)
	if cv.Kind() != reflect.Ptr {
		panic("command must be a pointer of struct")
	}
	cve := cv.Elem()
	if cve.Kind() != reflect.Struct {
		panic("command's point element must be a struct")
	}
	content := cve.FieldByName("Content")
	if !content.IsValid() || content.Kind() != reflect.Struct {
		panic("command struct must has a field struct named by Content")
	}
	cmd.execute = command
	cmd.content = content.Interface()
	cmd.check(reflect.ValueOf(cmd.execute),
		reflect.TypeOf(cmd.content), "")

	iConn := cve.FieldByName("IConn")
	if iConn.Kind() != reflect.Interface {
		panic("command struct must has a field interface reference by lib.IConn")
	}
	iConn.Set(reflect.ValueOf(&cmd))

	listener, err := net.Listen("tcp", Addr+port)
	if err != nil {
		logger("Fail to start command listener, %s\n", err)
		return nil, err
	}
	logger("Command Server Started ...")
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-cmd.exit:
					return
				default:
					fmt.Printf("Fail to connect, %s\n", err)
				}
				continue
			}
			//每次只接受一个连接
			if cmd.conn.closed {
				_, _ = conn.Write([]byte("ok\n"))
				cmd.conn.Conn = conn
				cmd.conn.closed = false
				//cmd.validateAuth()
				go cmd.connHandler()
			} else {
				_, _ = conn.Write([]byte("busy\n"))
				_ = conn.Close()
			}
		}
	}()
	go func() {
		<-cmd.exit
		logger("接收到退出信号，退出监听command")
		if !cmd.conn.closed {
			_, _ = cmd.conn.Write([]byte("I'm closed,see you next time.\n"))
			cmd.conn.Exit()
		}
		_ = listener.Close()
	}()
	return &cmd, nil
}

func (cmd *Command) validateAuth() {
	go func() {
		//延时一秒，验证权限，如果没通过，则关闭连接
		time.Sleep(time.Second)
		if cmd.conn.auth < 0 {
			cmd.conn.Exit()
		}
	}()
}

func (cmd *Command) connHandler() {
	if cmd.conn.closed || cmd.conn.Conn == nil {
		return
	}
	buf := make([]byte, 4096)
FOREVER:
	for {
		cnt, err := cmd.conn.Read(buf)
		if err != nil || cnt == 0 {
			cmd.conn.Exit()
			break FOREVER
		}
		inStr := strings.TrimSpace(string(buf[0:cnt]))
		inputs := strings.Split(inStr, " ")
		logger(inStr)
		if len(inputs) < 1 {
			continue
		}
		if inputs[0] == "exit" {
			cmd.conn.Exit()
			break FOREVER
		} else if len(inputs) == 1 && inputs[0] == "help" {
			cmd.Help()
		} else {
			executed := cmd.executeRouter(reflect.TypeOf(cmd.content), "", inputs)
			if !executed {
				cmd.unsupported()
			}
		}
		cmd.WriteString("➜ ")
	}
	logger("Connection from %v closed. \n", cmd.conn.RemoteAddr())
}

func firstUpperCase(str string) string {
	if len(str) > 1 {
		str = strings.ToUpper(str[0:1]) + strings.ToLower(str[1:])
	} else {
		str = strings.ToUpper(str)
	}
	return str
}

func (conn *Conn) Exit() {
	_ = conn.Conn.Close()
	conn.auth = 0
	conn.closed = true
}

func (cmd *Command) unsupported() {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	cmd.WriteStringLn("unsupported command,use help to list all commands")
}

func (cmd *Command) Help() {
	if cmd.conn.Conn == nil {
		logger("no connect")
		return
	}
	cmd.help(reflect.TypeOf(cmd.content), "")
}

func (cmd *Command) help(ct reflect.Type, name string) {
	if ct.NumField() == 0 {
		return
	}
	name = strings.ToLower(name)
	for i := 0; i < ct.NumField(); i++ {
		sf := ct.Field(i)
		if sf.Type.Kind() == reflect.Struct {
			cmd.WriteStringf("%v%v \t%v\n", name, strings.ToLower(sf.Name), sf.Tag)
			cmd.help(sf.Type, name+sf.Name+" ")
		}
	}
}

func (cmd *Command) executeRouter(ct reflect.Type, name string, strs []string) (executed bool) {
	if ct.Kind() != reflect.Struct {
		panic("router must be struct")
	}
	if ct.NumField() == 0 ||
		len(strs) == 0 {
		exe := reflect.ValueOf(cmd.execute)
		m := exe.MethodByName(name)
		if m.IsValid() {
			return cmd.call(m, strs)
		}
		return
	}

	child := firstUpperCase(strs[0])
	c, ok := ct.FieldByName(child)
	if !ok {
		return
	}
	executed = cmd.executeRouter(c.Type, name+child, strs[1:])
	if !executed {
		cmdt := reflect.ValueOf(cmd)
		m := cmdt.MethodByName(name)
		if m.IsValid() {
			return cmd.call(m, strs)
		}
	}
	return
}

func (cmd *Command) call(m reflect.Value, strs []string) bool {

	numIn := m.Type().NumIn()
	if numIn == 0 {
		m.Call([]reflect.Value{})
	} else {
		m.Call([]reflect.Value{
			reflect.ValueOf(strs),
		})
	}
	return true
}

func (cmd *Command) check(cmdt reflect.Value, ct reflect.Type, name string) {
	if ct.NumField() == 0 {
		m := cmdt.MethodByName(name)
		if !m.IsValid() {
			panic(name + " Method not implemented")
		}
		t := m.Type()
		if t.NumIn() > 1 {
			panic(name + " Method param number must less or equal than 1 ")
		}
		if t.NumIn() == 1 &&
			t.In(0) != reflect.TypeOf([]string(nil)) {
			panic(name + " Method param must be []string ")
		}
		return
	}
	for i := 0; i < ct.NumField(); i++ {
		sf := ct.Field(i)
		if sf.Type.Kind() != reflect.Struct {
			panic(sf.Name + " field must be struct")
		}
		cmd.check(cmdt, sf.Type, name+sf.Name)
	}
}

func Attach(port string) {
	conn, err := net.Dial("tcp", Addr+port)
	if err != nil {
		fmt.Printf("Fail to connect, %s\n", err)
		return
	}
	buf := make([]byte, 1024)
	cnt, err := conn.Read(buf)
	if err != nil {
		fmt.Printf("连接已关闭")
		return
	}
	if bytes.Equal([]byte("busy\n"), buf[0:cnt]) {
		fmt.Printf("已有其他客户端在连接，连接被关闭。")
		return
	}
	fmt.Printf("已连接\n➜ ")
	go clientHandlerWriter(conn)
	clientHandlerReader(conn)

}

func clientHandlerReader(c net.Conn) {
	defer func(c net.Conn) {
		_ = c.Close()
	}(c)
	buf := make([]byte, 1024)
	for {
		cnt, err := c.Read(buf)
		if err != nil {
			fmt.Printf("连接已关闭")
			break
		}
		fmt.Printf(string(buf[0:cnt]))
	}
}

func clientHandlerWriter(c net.Conn) {
	defer func(c net.Conn) {
		_ = c.Close()
	}(c)
	reader := bufio.NewReader(os.Stdin)
	for {
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "exit" {
			fmt.Printf("byebye")
			return
		}
		_, _ = c.Write([]byte(input))
	}
}
