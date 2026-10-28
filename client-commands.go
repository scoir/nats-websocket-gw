package gw

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/scoir/nats-websocket-gw/internal/buffer"
)

// maxControlLine matches the default max_control_line of the NATS server.
const maxControlLine = 4096

// ErrInvalidCommand is wrapped by the errors passed to the ErrorHandler when a
// connection is closed because the client sent a command that isn't allowed or
// can't be parsed, as opposed to the connection failing. Check for it with
// errors.Is.
var ErrInvalidCommand = errors.New("invalid command")

var crlf = []byte("\r\n")

// forwardClientCommands parses the NATS client protocol commands in a single
// websocket frame (src), checks each one against Settings.IsAllowedCommand and
// forwards it to dst, applying Settings.Prefix to SUBs. br is reused between
// frames to avoid allocating a buffer per frame.
//
// PUB payloads are streamed through without being inspected, so a
// payload that looks like a command is never mistaken for one. A command and
// its payload must arrive in the same frame.
func (gw *Gateway) forwardClientCommands(dst io.Writer, br *bufio.Reader, src io.Reader) error {
	br.Reset(src)
	for {
		line, err := br.ReadSlice('\n')
		if err == io.EOF && len(line) == 0 {
			return nil
		}
		if err == io.EOF {
			return fmt.Errorf("%w: unterminated command", ErrInvalidCommand)
		}
		if err == bufio.ErrBufferFull {
			return fmt.Errorf("%w: command longer than %d bytes", ErrInvalidCommand, br.Size())
		}
		if err != nil {
			return err
		}

		args := splitArgs(line)
		if len(args) == 0 {
			return fmt.Errorf("%w: empty command", ErrInvalidCommand)
		}
		verb := string(bytes.ToUpper(args[0]))
		if gw.settings.IsAllowedCommand != nil && !gw.settings.IsAllowedCommand(verb) {
			return fmt.Errorf("%w: command not allowed: %q", ErrInvalidCommand, verb)
		}

		payloadLen, err := payloadLength(verb, args)
		if err != nil {
			return err
		}
		if verb == "PUB" {
			if err = gw.checkPublish(args); err != nil {
				return err
			}
		}

		if err = buffer.WriteCommandWithPrefix(gw.settings.Prefix, dst, line); err != nil {
			return err
		}

		if payloadLen < 0 {
			continue
		}
		n, err := io.Copy(dst, io.LimitReader(br, payloadLen))
		if err != nil {
			return err
		}
		if n != payloadLen {
			return fmt.Errorf("%w: %s payload cut short: got %d of %d bytes", ErrInvalidCommand, verb, n, payloadLen)
		}
		// the payload must be followed by a trailing \r\n
		if end, err := br.Peek(len(crlf)); err != nil || !bytes.Equal(end, crlf) {
			return fmt.Errorf("%w: %s payload is not followed by CRLF", ErrInvalidCommand, verb)
		}
		br.Discard(len(crlf))
		if _, err = dst.Write(crlf); err != nil {
			return err
		}
	}
}

// checkPublish checks a PUB's subject against Settings.IsAllowedPublish. Its
// reply-to, if any, must be inside Settings.Prefix: the client only receives on
// prefixed subjects, so any other reply-to would only point replies at
// somebody else.
func (gw *Gateway) checkPublish(args [][]byte) error {
	subject := string(args[1])
	if gw.settings.IsAllowedPublish != nil && !gw.settings.IsAllowedPublish(subject) {
		return fmt.Errorf("%w: publish not allowed: %q", ErrInvalidCommand, subject)
	}
	if len(args) == 4 && gw.settings.Prefix != "" && !bytes.HasPrefix(args[2], []byte(gw.settings.Prefix)) {
		return fmt.Errorf("%w: reply-to not allowed: %q", ErrInvalidCommand, args[2])
	}
	return nil
}

// splitArgs splits a control line on the same delimiters as the NATS server.
func splitArgs(line []byte) [][]byte {
	return bytes.FieldsFunc(line, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\r' || r == '\n'
	})
}

// payloadLength returns the size of the payload following a command, or -1 if
// the command has no payload. HPUB is rejected rather than passed through,
// since forwarding it without its payload would desync the stream.
func payloadLength(verb string, args [][]byte) (int64, error) {
	switch verb {
	case "HPUB":
		return 0, fmt.Errorf("%w: HPUB is not supported", ErrInvalidCommand)
	case "PUB": // PUB <subject> [reply-to] <#bytes>
		if len(args) < 3 || len(args) > 4 {
			return 0, fmt.Errorf("%w: invalid PUB: wrong number of arguments", ErrInvalidCommand)
		}
		size, err := strconv.ParseUint(string(args[len(args)-1]), 10, 31)
		if err != nil {
			return 0, fmt.Errorf("%w: invalid PUB payload size: %v", ErrInvalidCommand, err)
		}
		return int64(size), nil
	}
	return -1, nil
}
