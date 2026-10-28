package buffer_test

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"github.com/scoir/nats-websocket-gw/internal/buffer"
)

func TestWebsocketPrefixing(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "non-sub command is untouched", input: "PING\r\n", expected: "PING\r\n"},
		{name: "connect is passed through verbatim", input: "CONNECT {\"pass\":\"a  b\"}\r\n", expected: "CONNECT {\"pass\":\"a  b\"}\r\n"},
		{name: "unsub is untouched", input: "UNSUB 2\r\n", expected: "UNSUB 2\r\n"},
		{name: "sub command gets prefixed", input: "SUB abc.xyz 2\r\n", expected: "SUB prefixed.abc.xyz 2\r\n"},
		{name: "sub command with multiple spaces gets prefixed", input: "SUB      abc.xyz 2\r\n", expected: "SUB prefixed.abc.xyz 2\r\n"},
		{name: "lowercase sub command gets prefixed", input: "sub abc.xyz 2\r\n", expected: "sub prefixed.abc.xyz 2\r\n"},
		{name: "tab separated sub command gets prefixed", input: "SUB\tabc.xyz\t2\r\n", expected: "SUB prefixed.abc.xyz 2\r\n"},
		{name: "sub command without CR gets prefixed", input: "SUB abc.xyz 2\n", expected: "SUB prefixed.abc.xyz 2\r\n"},
		{name: "sub queue group is not prefixed", input: "SUB abc.xyz workers 2\r\n", expected: "SUB prefixed.abc.xyz workers 2\r\n"},
		{name: "every command in a frame is checked", input: "PING\r\nSUB abc.xyz 2\r\n", expected: "PING\r\nSUB prefixed.abc.xyz 2\r\n"},
		{name: "pub command gets prefixed", input: "PUB abc.xyz 5\r\nhello\r\n", expected: "PUB prefixed.abc.xyz 5\r\nhello\r\n"},
		{name: "lowercase pub command gets prefixed", input: "pub abc.xyz 5\r\nhello\r\n", expected: "pub prefixed.abc.xyz 5\r\nhello\r\n"},
		{name: "pub reply-to gets prefixed", input: "PUB abc.xyz inbox.1 5\r\nhello\r\n", expected: "PUB prefixed.abc.xyz prefixed.inbox.1 5\r\nhello\r\n"},
		{name: "pub with empty payload gets prefixed", input: "PUB abc.xyz 0\r\n\r\n", expected: "PUB prefixed.abc.xyz 0\r\n\r\n"},
		{name: "pub payload is not parsed as a command", input: "PUB abc.xyz 9\r\nSUB > 1\r\n\r\n", expected: "PUB prefixed.abc.xyz 9\r\nSUB > 1\r\n\r\n"},
		{name: "commands after a pub are checked", input: "PUB abc.xyz 5\r\nhello\r\nSUB def 3\r\n", expected: "PUB prefixed.abc.xyz 5\r\nhello\r\nSUB prefixed.def 3\r\n"},
		{name: "empty frame writes nothing", input: "", expected: ""},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReader(strings.NewReader(tc.input))
			w := bytes.NewBuffer(nil)

			err := buffer.CopyCommandsWithPrefix("prefixed.", w, r)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			diff := cmp.Diff(w.String(), tc.expected)
			if diff != "" {
				t.Fatal(diff)
			}
		})
	}
}

func TestWebsocketPrefixingRejects(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{name: "unknown command", input: "SUBTRACT\r\n"},
		{name: "hpub command", input: "HPUB abc.xyz 12 17\r\nNATS/1.0\r\n\r\nhello\r\n"},
		{name: "empty command", input: "\r\n"},
		{name: "unterminated command", input: "SUB abc.xyz 2"},
		{name: "sub with too few arguments", input: "SUB abc.xyz\r\n"},
		{name: "sub with too many arguments", input: "SUB abc.xyz q 2 extra\r\n"},
		{name: "pub with too few arguments", input: "PUB 5\r\nhello\r\n"},
		{name: "pub with invalid size", input: "PUB abc.xyz five\r\nhello\r\n"},
		{name: "pub with negative size", input: "PUB abc.xyz -5\r\nhello\r\n"},
		{name: "pub payload shorter than size", input: "PUB abc.xyz 10\r\nhello\r\n"},
		{name: "pub payload longer than size", input: "PUB abc.xyz 2\r\nhello\r\n"},
		{name: "pub payload without CRLF", input: "PUB abc.xyz 5\r\nhello"},
		{name: "control line longer than the buffer", input: "SUB " + strings.Repeat("a", 5000) + " 1\r\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bufio.NewReaderSize(strings.NewReader(tc.input), 4096)
			w := bytes.NewBuffer(nil)

			err := buffer.CopyCommandsWithPrefix("prefixed.", w, r)
			if !errors.Is(err, buffer.ErrInvalidCommand) {
				t.Fatalf("expected ErrInvalidCommand, got %v", err)
			}
		})
	}
}
