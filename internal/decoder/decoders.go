package decoder

import "io"

type Decoder func(r io.Reader) error
