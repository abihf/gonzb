package nntp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"sync"
)

type Client struct {
	Dialer net.Dialer
	Host   string
	Port   int
	TLS    bool

	TLSConfig tls.Config
	MaxConn   int

	Closed bool

	cons  map[*Conn]bool
	mutex sync.Mutex
}

var ErrLimitExceeded = fmt.Errorf("Maximum client connection exceeded")

func (c *Client) Connect(ctx context.Context) (*Conn, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	if len(c.cons) >= c.MaxConn {
		return nil, ErrLimitExceeded
	}

	var inner io.ReadWriteCloser
	tcpConn, err := c.Dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", c.Host, c.Port))

	if err != nil {
		return nil, err
	}
	if c.TLS {
		if c.TLSConfig.ServerName == "" {
			c.TLSConfig.ServerName = c.Host
		}
		tlsCon := tls.Client(tcpConn, &c.TLSConfig)
		tlsCon.HandshakeContext(ctx)
		inner = tlsCon
	} else {
		inner = tcpConn
	}

	conn := &Conn{client: c, Closed: make(chan interface{}, 1)}
	onClose := func(err error) error {
		conn.Closed <- nil
		c.mutex.Lock()
		defer c.mutex.Unlock()
		delete(c.cons, conn)
		return err
	}

	conn.Conn = textproto.NewConn(&closeInjector{inner, onClose})
	c.cons[conn] = true
	return conn, nil
}

func (c *Client) Len() int {
	return len(c.cons)
}

func (c *Client) CloseAll() {
	for conn := range c.cons {
		go conn.Close()
	}
}

type closeInjector struct {
	io.ReadWriteCloser
	OnClose func(error) error
}

func (ci *closeInjector) Close() error {
	err := ci.ReadWriteCloser.Close()
	if ci.OnClose != nil {
		return ci.OnClose(err)
	}
	return err
}
