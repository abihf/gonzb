package yenc

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/crc32"
	"io"
	"strconv"
	"strings"
	"sync"
)

type yencReader interface {
	io.Reader
	io.ByteReader
}

var buffPoll = sync.Pool{
	New: func() interface{} {
		var buf bytes.Buffer
		buf.Grow(200)
		return &buf
	},
}

func Decode(out []byte, r io.Reader) (err error) {
	yr, ok := r.(io.ByteReader)
	if !ok {
		yr = bufio.NewReader(r)
	}

	crc := crc32.NewIEEE()

	buf := buffPoll.Get().(*bytes.Buffer)
	defer buffPoll.Put(buf)

	readLine := func() ([]byte, error) {
		buf.Reset()
		cr := false
		for {
			b, err := yr.ReadByte()
			if err != nil {
				return nil, err
			}

			if cr {
				if b == '\n' {
					return buf.Bytes(), nil
				}
				buf.WriteByte('\r')
				cr = false
			}
			if b == '\r' {
				cr = true
				continue
			} else {
				buf.WriteByte(b)
			}
		}
	}

	var header map[string]string
	var part map[string]string
	var footer map[string]string
	var parseErr error
	offset := 0
	isEscape := false
	for {
		lineB, err := readLine()
		if err != nil {
			return fmt.Errorf("can read line: %w", err)
		}
		line := strings.TrimSpace(string(lineB))
		if parseErr != nil {
			if line == "." {
				break
			}
			continue
		}

		if header == nil && !strings.HasPrefix(line, "=ybegin ") {
			continue
		}
		if footer != nil {
			if line == "." {
				return nil
			}
			continue
		}

		if strings.HasPrefix(line, "=ybegin ") {
			if header != nil {
				parseErr = fmt.Errorf("ybegin marker line found multiple times")
				continue
			}
			header = parseYencCmd(line[8:])
			continue
		}

		if strings.HasPrefix(line, "=ypart ") {
			if header == nil {
				parseErr = fmt.Errorf("found ypart before ybegin")
				continue
			}
			part = parseYencCmd(line[7:])
			begin, err := strconv.Atoi(part["begin"])
			if err == nil {
				offset = begin
			}
			continue
		}

		if strings.HasPrefix(line, "=yend ") {
			if footer != nil {
				parseErr = fmt.Errorf("yend marker line found multiple times")
				continue
			}
			if header == nil {
				parseErr = fmt.Errorf("yend marker line cannot appear before ybegin marker line")
				continue
			}
			footer = parseYencCmd(line[6:])
			var crcStr string
			if part != nil {
				crcStr = footer["pcrc32"]
			} else {
				crcStr = footer["crc32"]
			}
			if crcStr != "" {
				recvCrc, _ := strconv.ParseUint(crcStr, 16, 32)
				realCrc := crc.Sum32()

				if uint32(recvCrc) != realCrc {
					parseErr = fmt.Errorf("CRC not valid, expect %08x got %08x\n", crcStr, realCrc)
					continue
				}
			}

			continue
		}

		originOffset := offset
		for _, b := range lineB {
			if b == 0x3D {
				isEscape = true
				continue
			}
			if isEscape {
				isEscape = false
				b -= 64
			}
			out[offset] = b - 42
			offset++
		}
		// for i := originOffset; i < offset; i += 8 {
		// 	// ptr :=
		// 	*(*uint64)(unsafe.Pointer(&out)) += 0x2a2a2a2a2a2a2a2a
		// }
		crc.Write(out[originOffset:offset])
	}
	return parseErr
}

func parseYencCmd(line string) map[string]string {
	res := map[string]string{}
	splitted := strings.Split(line, " ")
	for i := 0; i < len(splitted); i++ {
		item := strings.SplitN(splitted[i], "=", 2)
		if item[0] == "name" {
			res["name"] = strings.Join(append(item[1:], splitted[i+1:]...), " ")
			break
		} else {
			res[item[0]] = item[1]
		}
	}
	return res
}
