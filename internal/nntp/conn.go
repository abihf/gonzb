package nntp

import (
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

type Conn struct {
	*textproto.Conn
	client *Client

	Closed chan interface{}
	mutex  sync.Mutex
	busy   bool
}

func (c *Conn) startWork() func() {
	c.mutex.Lock()
	c.busy = true
	return func() {
		c.busy = false
		c.mutex.Unlock()
	}
}

func (c *Conn) Capabilities(group string) ([]string, error) {
	defer c.startWork()()

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

func (c *Conn) Group(group string) (*Group, error) {
	defer c.startWork()()

	id, err := c.Cmd("GROUP %s", group)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)

	_, msg, err := c.ReadResponse(211)
	if err != nil {
		return nil, err
	}

	splitted := strings.Split(msg, " ")
	num, err := strconv.Atoi(splitted[0])
	if err != nil {
		return nil, err
	}
	low, err := strconv.Atoi(splitted[1])
	if err != nil {
		return nil, err
	}
	high, err := strconv.Atoi(splitted[2])
	if err != nil {
		return nil, err
	}

	return &Group{
		Name:   splitted[3],
		Number: num,
		Low:    low,
		High:   high,
	}, nil
}

func (c *Conn) Head(article string) (textproto.MIMEHeader, error) {
	defer c.startWork()()

	id, err := c.Cmd("HEAD %s", article)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	defer c.EndResponse(id)
	return c.ReadMIMEHeader()
}

func (c *Conn) Body(article string) (io.ReadCloser, error) {
	c.mutex.Lock()
	c.busy = true
	id, err := c.Cmd("BODY %s", article)
	if err != nil {
		return nil, err
	}
	c.StartResponse(id)
	return &dotCloser{c.DotReader(), c, id}, nil
}

type dotCloser struct {
	io.Reader
	c  *Conn
	id uint
}

func (d *dotCloser) Close() error {
	d.c.EndResponse(d.id)
	d.c.busy = false
	d.c.mutex.Unlock()
	return nil
}
