package buffer

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"
)

var crlf = []byte("\r\n")

// CopyCommandsWithPrefix copies the NATS client commands in src to dst, writing
// prefix in front of every subject a SUB or PUB names so that the websocket
// can't subscribe or publish to anything outside the prefix.
//
// SUB and PUB lines are rebuilt from the arguments parsed here, so the NATS
// server sees exactly what was checked. src must hold whole commands: a PUB's
// payload has to follow it in src.
func CopyCommandsWithPrefix(prefix string, dst io.Writer, src *bufio.Reader) error {
	for {
		line, err := src.ReadSlice('\n')
		if err == io.EOF && len(line) == 0 {
			return nil
		}
		if err == io.EOF {
			return errors.New("unterminated command")
		}
		if err != nil {
			return fmt.Errorf("reading command: %w", err)
		}

		args := bytes.FieldsFunc(line, isDelimiter)
		if len(args) == 0 {
			return errors.New("empty command")
		}
		switch verb := string(bytes.ToUpper(args[0])); verb {
		case "CONNECT", "PING", "PONG", "UNSUB":
			_, err = dst.Write(line)
		case "SUB":
			err = writeSub(prefix, dst, args)
		case "PUB":
			err = writePub(prefix, dst, src, args)
		default:
			// this includes HPUB, whose payload we don't parse
			return fmt.Errorf("command not allowed: %q", verb)
		}
		if err != nil {
			return err
		}
	}
}

// writeSub writes SUB <subject> [queue group] <sid>
func writeSub(prefix string, dst io.Writer, args [][]byte) error {
	if len(args) != 3 && len(args) != 4 {
		return errors.New("invalid SUB: wrong number of arguments")
	}
	args[1] = withPrefix(prefix, args[1])
	return writeLine(dst, args)
}

// writePub writes PUB <subject> [reply-to] <#bytes> and the payload after it.
// The reply-to is prefixed too, so replies can't be sent outside the prefix.
func writePub(prefix string, dst io.Writer, src *bufio.Reader, args [][]byte) error {
	if len(args) != 3 && len(args) != 4 {
		return errors.New("invalid PUB: wrong number of arguments")
	}
	size, err := strconv.ParseUint(string(args[len(args)-1]), 10, 31)
	if err != nil {
		return fmt.Errorf("invalid PUB payload size: %w", err)
	}
	for i := 1; i < len(args)-1; i++ {
		args[i] = withPrefix(prefix, args[i])
	}
	if err = writeLine(dst, args); err != nil {
		return err
	}

	if _, err = io.CopyN(dst, src, int64(size)); err != nil {
		return fmt.Errorf("reading PUB payload: %w", err)
	}
	end, err := src.Peek(len(crlf))
	if err != nil || !bytes.Equal(end, crlf) {
		return errors.New("PUB payload is not followed by CRLF")
	}
	if _, err = src.Discard(len(crlf)); err != nil {
		return err
	}
	_, err = dst.Write(crlf)
	return err
}

func writeLine(dst io.Writer, args [][]byte) error {
	line := append(bytes.Join(args, []byte(" ")), crlf...)
	_, err := dst.Write(line)
	return err
}

func withPrefix(prefix string, subject []byte) []byte {
	return append([]byte(prefix), subject...)
}

// isDelimiter matches the argument delimiters of the NATS server
func isDelimiter(r rune) bool {
	return r == ' ' || r == '\t' || r == '\r' || r == '\n'
}
