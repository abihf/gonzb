package decoder

import "io"

type Decoder func(buff []byte, r io.Reader) error
