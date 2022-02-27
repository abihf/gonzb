package downloader

import "github.com/abihf/gonzb/internal/nntp"

type Config struct {
	Servers []nntp.Config `yaml:"servers"`
}
