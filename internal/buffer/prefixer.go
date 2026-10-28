package buffer

import (
	"bytes"
	"io"
)

// WriteCommandWithPrefix writes a single NATS control line to dst. If the line
// is a SUB, prefix is inserted at the start of the subject so that the
// websocket can't subscribe to anything but the prefix. Like the NATS server,
// it treats the verb case-insensitively and accepts spaces or tabs between
// the verb and the subject.
func WriteCommandWithPrefix(prefix string, dst io.Writer, line []byte) error {
	verbStart := skipDelimiters(line, 0)
	verbEnd := verbStart
	for verbEnd < len(line) && !isDelimiter(line[verbEnd]) {
		verbEnd++
	}
	subject := skipDelimiters(line, verbEnd)

	if !bytes.EqualFold(line[verbStart:verbEnd], []byte("SUB")) || subject == len(line) {
		_, err := dst.Write(line)
		return err
	}
	for _, part := range [][]byte{line[:subject], []byte(prefix), line[subject:]} {
		if _, err := dst.Write(part); err != nil {
			return err
		}
	}
	return nil
}

func skipDelimiters(line []byte, i int) int {
	for i < len(line) && isDelimiter(line[i]) {
		i++
	}
	return i
}

func isDelimiter(b byte) bool {
	return b == ' ' || b == '\t' || b == '\r' || b == '\n'
}
