package nntp

import (
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strings"
)

type Conn struct {
	*textproto.Conn
	// client *Client

	// mutex  sync.Mutex
	// Closed chan struct{}

	group  *Group
	closed bool

	// busy        bool
	// shutingDown bool
}

type Config struct {
	Servers []*ServerConfig `yaml:"servers"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Auth *Auth  `yaml:"auth"`

	MaxConn int `yaml:"maxConn"`

	TLS    bool     `yaml:"tls"`
	Cipher []string `yaml:"cipher"`
}

type Auth struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// func (c *Conn) startWork() (func(), bool) {
// 	c.mutex.Lock()
// 	if c.shutingDown {
// 		defer c.mutex.Unlock()
// 		c.Close()
// 		return nil, false
// 	}
// 	c.busy = true
// 	return func() {
// 		c.busy = false
// 		c.mutex.Unlock()
// 	}, true
// }

// func (c *Conn) Idle() {
// 	c.client.mutex.Lock()
// 	defer c.client.mutex.Unlock()

// 	c.client.busyCount--
// 	c.client.idleConns[c] = true
// }

// func (c *Conn) Name() string {
// 	return c.client.Host
// }

// func (c *Conn) Shutdown() {
// 	c.mutex.Lock()
// 	defer c.mutex.Unlock()
// 	c.shutingDown = true
// 	if !c.busy {
// 		c.Close()
// 	}
// }

func (c *Conn) Auth(a *Auth) error {
	// done, ok := c.startWork()
	// if !ok {
	// 	return io.EOF
	// }
	// defer done()

	id, err := c.Cmd("AUTHINFO USER %s", a.User)
	if err != nil {
		c.checkEOF(err)
		return err
	}
	c.StartResponse(id)
	_, _, err = c.ReadResponse(381)
	c.EndResponse(id)
	if err != nil {
		return err
	}
	id, err = c.Cmd("AUTHINFO PASS %s", a.Password)
	if err != nil {
		return err
	}
	c.StartResponse(id)
	_, _, err = c.ReadResponse(281)
	c.EndResponse(id)
	if err != nil {
		c.checkEOF(err)
		return err
	}

	return nil
}

func (c *Conn) Capabilities(group string) ([]string, error) {
	// done, ok := c.startWork()
	// if !ok {
	// 	return nil, io.EOF
	// }
	// defer done()

	id, err := c.Cmd("CAPABILITIES")
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	_, _, err = c.ReadResponse(101)
	if err != nil {
		c.checkEOF(err)
		return nil, err
	}
	return c.ReadDotLines()
}

type Group struct {
	Name   string
	Number int
	Low    int
	High   int
}

func (c *Conn) Groups(groups []string) (*Group, error) {
	if c.group != nil {
		for _, g := range groups {
			if g == c.group.Name {
				return c.group, nil
			}
		}
	}

	var err error
	var g *Group
	for _, name := range groups {
		g, err = c.Group(name)
		if err == nil {
			return g, nil
		}
	}
	return nil, err
}

func (c *Conn) Group(group string) (*Group, error) {
	// done, ok := c.startWork()
	// if !ok {
	// 	return nil, io.EOF
	// }
	// defer done()

	id, err := c.Cmd("GROUP %s", group)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	_, _, err = c.ReadResponse(211)
	if err != nil {
		c.checkEOF(err)
		return nil, err
	}
	c.group = &Group{
		Name: group,
	}
	return c.group, nil
	// splitted := strings.Split(msg, " ")
	// num, err := strconv.Atoi(splitted[0])
	// if err != nil {
	// 	return nil, err
	// }
	// low, err := strconv.Atoi(splitted[1])
	// if err != nil {
	// 	return nil, err
	// }
	// high, err := strconv.Atoi(splitted[2])
	// if err != nil {
	// 	return nil, err
	// }

	// return &Group{
	// 	Name:   splitted[3],
	// 	Number: num,
	// 	Low:    low,
	// 	High:   high,
	// }, nil
}

func (c *Conn) Head(article string) (textproto.MIMEHeader, error) {
	// done, ok := c.startWork()
	// if !ok {
	// 	return nil, io.EOF
	// }
	// defer done()

	id, err := c.Cmd("HEAD %s", article)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)
	return c.ReadMIMEHeader()
}

type BodyCB func(io.Reader) error

func (c *Conn) Body(article string, cb BodyCB) error {
	// done, ok := c.startWork()
	// if !ok {
	// 	return io.EOF
	// }
	// defer done()
	id, err := c.Cmd("BODY %s", article)
	if err != nil {
		return err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	line, err := c.ReadLine()
	if err != nil {
		c.checkEOF(err)
		return err
	}
	line = strings.TrimLeft(line, " \r\n")
	if !strings.HasPrefix(line, "222 ") {
		return fmt.Errorf("invalid response code %s expect 222", line[0:3])
	}
	return cb(c.R)
}

func (c *Conn) checkEOF(err error) {
	if errors.Is(err, io.EOF) {
		c.closed = true
	}
}
