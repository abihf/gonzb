package decoder

import "io"

type yenc struct {
	io.Reader
}

func Yenc(r io.Reader) io.Reader {
	return &yenc{r}
}
