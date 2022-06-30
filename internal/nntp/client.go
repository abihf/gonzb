package nntp

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"time"

	"github.com/abihf/gonzb/internal/pool"
)

type Client struct {
	Config    *ServerConfig
	Dialer    net.Dialer
	TLSConfig tls.Config

	pool *pool.Pool[*Conn]
}

func New(conf *ServerConfig) *Client {
	c := Client{Config: conf}
	c.pool = pool.New(&pool.Config[*Conn]{
		Factory: c.connect,
		Close:   func(c *Conn) error { return c.Close() },
		MaxCon:  int32(conf.MaxConn),
		Valid:   func(c *Conn) bool { return !c.closed },
	})
	return &c
}

func (c *Client) connect(ctx context.Context) (*Conn, error) {
	conf := c.Config
	tmpConn, err := c.Dialer.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", conf.Host, conf.Port))
	if err != nil {
		return nil, err
	}
	tcpConn := tmpConn.(*net.TCPConn)
	tcpConn.SetKeepAlive(true)
	tcpConn.SetKeepAlivePeriod(10 * time.Second)
	tcpConn.SetLinger(0)

	var inner io.ReadWriteCloser = tcpConn

	if conf.TLS {
		tlsConf := c.TLSConfig

		if tlsConf.ServerName == "" {
			tlsConf.ServerName = conf.Host
		}

		if len(conf.Cipher) > 0 {
			codes := []uint16{}
			suites := tls.CipherSuites()
			suites = append(suites, tls.InsecureCipherSuites()...)

			for _, c := range conf.Cipher {
				for _, s := range suites {
					if s.Name == c {
						codes = append(codes, s.ID)
					}
				}
			}
			tlsConf.CipherSuites = codes
		}

		tlsCon := tls.Client(tcpConn, &tlsConf)
		inner = tlsCon
	}

	conn := &Conn{Conn: textproto.NewConn(inner)}
	_, msg, err := conn.ReadCodeLine(200)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("expect welcome message got %s: %w", msg, err)
	}

	if conf.Auth != nil {
		err = conn.Auth(conf.Auth)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to authenticate: %w", err)
		}
	}

	return conn, nil
}

func (c *Client) Download(ctx context.Context, groups []string, article string, cb BodyCB) error {
	conn, err := c.pool.Get(ctx)
	if err != nil {
		return err
	}
	defer c.pool.Put(conn)

	_, err = conn.Groups(groups)
	if err != nil {
		return err
	}

	return conn.Body(article, cb)
}

func (c *Client) Close() error {
	c.pool.Release()
	return nil
}
