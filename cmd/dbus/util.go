package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
)

type indenter struct {
	prefix     string
	indentNext bool
}

func (i *indenter) v(v any) {
	fmt.Fprintf(i, "%v\n", v)
}

func (i *indenter) s(msg string) {
	io.WriteString(i, msg+"\n")
}

func (i *indenter) f(msg string, args ...any) {
	fmt.Fprintf(i, msg+"\n", args...)
}

func (i *indenter) Write(bs []byte) (int, error) {
	ret := 0
	for len(bs) > 0 {
		if i.indentNext {
			i.indentNext = false
			_, err := io.WriteString(os.Stdout, i.prefix)
			if err != nil {
				return ret, err
			}
		}

		var wr []byte
		idx := bytes.IndexByte(bs, '\n')
		if idx >= 0 {
			i.indentNext = true
			wr, bs = bs[:idx+1], bs[idx+1:]
		} else {
			wr, bs = wr, nil
		}

		n, err := os.Stdout.Write(wr)
		ret += n
		if err != nil {
			return ret, err
		}
	}
	return ret, nil
}

func (i *indenter) indent() {
	i.prefix += "  "
}

func (i *indenter) dedent() {
	i.prefix = i.prefix[:len(i.prefix)-2]
}
