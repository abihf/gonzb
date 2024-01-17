package nntp

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"time"
)

type Conn struct {
	*textproto.Conn
	client *Client

	mutex     sync.Mutex
	connected bool

	group *Group

	busy        bool
	shutingDown bool
}

func (c *Conn) startWork() (func(), error) {
	c.mutex.Lock()
	if c.shutingDown {
		defer c.mutex.Unlock()
		c.Close()
		return nil, fmt.Errorf("shuting down")
	}
	if !c.connected {
		err := c.connect()
		if err != nil {
			defer c.mutex.Unlock()
			c.Close()
			return nil, err
		}
	}

	c.busy = true
	return func() {
		c.busy = false
		c.mutex.Unlock()
	}, nil
}

func (c *Conn) Name() string {
	return c.client.Host
}

func (c *Conn) connect() error {
	var inner io.ReadWriteCloser
	tmpConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.client.Host, c.client.Port))
	if err != nil {
		return err
	}
	tcpConn := tmpConn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(10 * time.Second)
	tcpConn.SetLinger(10)

	if c.client.TLS {
		if c.client.TLSConfig.ServerName == "" {
			c.client.TLSConfig.ServerName = c.client.Host
		}

		tlsCon := tls.Client(tcpConn, &c.client.TLSConfig)
		inner = tlsCon
	} else {
		inner = tcpConn
	}

	c.Conn = textproto.NewConn(inner)
	_, _, err = c.ReadCodeLine(200)
	if err != nil {
		c.Close()
		return err
	}

	c.connected = true

	if c.client.Auth != nil {
		err = c.Auth(c.client.Auth)
		if err != nil {
			c.Close()
			return err
		}
	}
	return nil
}

func (c *Conn) Shutdown() {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.shutingDown = true
	if !c.busy {
		c.Close()
	}
}

func (c *Conn) Auth(a *Auth) error {
	id, err := c.Cmd("AUTHINFO USER %s", a.User)
	if err != nil {
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
		return err
	}

	return nil
}

func (c *Conn) Capabilities(group string) ([]string, error) {
	done, err := c.startWork()
	if err != nil {
		return nil, err
	}
	defer done()

	id, err := c.Cmd("CAPABILITIES")
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	_, _, err = c.ReadResponse(101)
	if err != nil {
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
	done, err := c.startWork()
	if err != nil {
		return nil, err
	}
	defer done()

	id, err := c.Cmd("GROUP %s", group)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	_, _, err = c.ReadResponse(211)
	if err != nil {
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
	done, err := c.startWork()
	if err != nil {
		return nil, err
	}
	defer done()

	id, err := c.Cmd("HEAD %s", article)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)
	return c.ReadMIMEHeader()
}

type Body interface {
	io.Reader
	io.ByteReader
}

func (c *Conn) Body(article string, handler func(Body) error) error {
	done, err := c.startWork()
	if err != nil {
		return err
	}
	defer done()
	id, err := c.Cmd("BODY <%s>", article)
	if err != nil {
		return err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	line, err := c.ReadLine()
	if err != nil {
		return err
	}
	line = strings.TrimLeft(line, " \r\n")
	if !strings.HasPrefix(strings.TrimLeft(line, " \r\n"), "222 ") {
		return fmt.Errorf("invalid response code %s expect 222", line[0:3])
	}
	return handler(c.R)
}

type bodyCloser struct {
	Body
	done func()
}

func (d *bodyCloser) Close() error {
	defer d.done()
	return nil
}
