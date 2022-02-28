package nntp

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"sync"
	"time"

	"github.com/marusama/semaphore/v2"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
	Auth *Auth  `yaml:"auth"`

	MaxConn int `yaml:"maxConn"`

	TLS    bool     `yaml:"tls"`
	Cipher []string `yaml:"cipher"`
}

type Client struct {
	Config
	Dialer    net.Dialer
	TLSConfig tls.Config

	idleConns map[*Conn]bool
	mutex     sync.RWMutex

	busyCount int
	sem       semaphore.Semaphore
	err       error
}

type Auth struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

func New(conf Config) *Client {
	c := &Client{
		Config: conf,
		sem:    semaphore.New(5),

		idleConns: map[*Conn]bool{},
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
		c.TLSConfig.CipherSuites = codes
	}

	return c
}

var ErrLimitExceeded = fmt.Errorf("Maximum client connection exceeded")

func (c *Client) getIdleConnection(n int) (res []*Conn) {
	c.mutex.Lock()
	defer c.mutex.Unlock()

	for conn := range c.idleConns {
		res = append(res, conn)
		delete(c.idleConns, conn)
		c.busyCount++
		n--
		if n <= 0 {
			return
		}
	}
	return
}

func (c *Client) ConnectN(max int) (res []*Conn, errors []error) {
	iddleConns := c.getIdleConnection(max)
	res = append(res, iddleConns...)
	if len(res) >= max || len(res) >= c.MaxConn {
		return
	}
	max -= len(res)

	var wg sync.WaitGroup
	for i := 0; i < max; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()

			if err := c.sem.Acquire(nil, 1); err != nil {
				errors = append(errors, err)
				return
			}
			defer c.sem.Release(1)

			cLen := c.Len()
			if cLen >= c.MaxConn {
				return
			}

			logger := log.With().Str("host", c.Host).Int("max", c.MaxConn).Logger()

			logger.Debug().Int("len", cLen).Msg("connecting to news server")
			var inner io.ReadWriteCloser
			tmpConn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", c.Host, c.Port))
			if err != nil {
				errors = append(errors, err)
				return
			}
			tcpConn := tmpConn.(*net.TCPConn)
			tcpConn.SetKeepAlive(true)
			tcpConn.SetKeepAlivePeriod(10 * time.Second)
			tcpConn.SetLinger(0)

			if c.TLS {
				if c.TLSConfig.ServerName == "" {
					c.TLSConfig.ServerName = c.Host
				}

				tlsCon := tls.Client(tcpConn, &c.TLSConfig)
				inner = tlsCon
			} else {
				inner = tcpConn
			}

			conn := &Conn{client: c, Closed: make(chan struct{}, 1)}
			onClose := func(err error) error {
				close(conn.Closed)
				c.mutex.Lock()
				defer c.mutex.Unlock()
				if _, ok := c.idleConns[conn]; ok {
					delete(c.idleConns, conn)
				} else {
					c.busyCount--
				}
				logger.Info().Int("len", c.busyCount+len(c.idleConns)).Msg("disconnected from server")
				return err
			}

			conn.Conn = textproto.NewConn(&closeInjector{inner, onClose})

			_, _, err = conn.ReadCodeLine(200)
			if err != nil {
				conn.Close()
				errors = append(errors, err)
				return
			}

			if c.Auth != nil {
				err = conn.Auth(c.Auth)
				if err != nil {
					conn.Close()
					errors = append(errors, err)
					return
				}
			}
			func() {
				c.mutex.Lock()
				defer c.mutex.Unlock()
				c.busyCount++
				res = append(res, conn)
				errors = append(errors, err)
				logger.Info().Int("len", c.busyCount+len(c.idleConns)).Msg("connected to server")
			}()
		}(i)
	}
	wg.Wait()
	return
}

func (c *Client) Len() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.busyCount + len(c.idleConns)
}

func (c *Client) BusyLen() int {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	return c.busyCount
}

// func (c *Client) CloseIdle() {
// 	c.mutex.Lock()
// 	defer c.mutex.Unlock()
// 	for conn := range c.idleConns {
// 		go conn.Shutdown()
// 	}
// }

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
