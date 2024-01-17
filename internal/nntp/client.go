package nntp

import (
	"crypto/tls"
	"fmt"
	"net"
)

type Config struct {
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

type ArticleRequest struct {
	Groups  []string
	Id      string
	Handler func(Body) error
	OnError func(error)
}

type Client struct {
	*Config
	Dialer    net.Dialer
	TLSConfig tls.Config
}

func New(conf *Config) *Client {
	c := &Client{
		Config: conf,
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

func (c *Client) StartWorker(ch chan *ArticleRequest) error {
	for i := 0; i < c.MaxConn; i++ {
		go c.worker(ch)
	}
	return nil
}

func (c *Client) worker(ch chan *ArticleRequest) {
	conn := Conn{client: c}
	for {
		req := <-ch
		if req == nil {
			return
		}
		conn.Groups(req.Groups)
		err := conn.Body(req.Id, req.Handler)
		if err != nil {
			if req.OnError != nil {
				req.OnError(err)
			} else {
				fmt.Println(err)
			}
		}
	}
}
