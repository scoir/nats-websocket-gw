package buffer_test

import (
	"bytes"
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
		{name: "sub command gets prefixed", input: "SUB abc.xyz 2\r\n", expected: "SUB prefixed.abc.xyz 2\r\n"},
		{name: "sub with queue group gets prefixed", input: "SUB abc.xyz q 2\r\n", expected: "SUB prefixed.abc.xyz q 2\r\n"},
		{name: "sub command with multiple spaces gets prefixed", input: "SUB      abc.xyz 2\r\n", expected: "SUB      prefixed.abc.xyz 2\r\n"},
		{name: "lowercase sub gets prefixed", input: "sub abc.xyz 2\r\n", expected: "sub prefixed.abc.xyz 2\r\n"},
		{name: "mixed case sub gets prefixed", input: "SuB abc.xyz 2\r\n", expected: "SuB prefixed.abc.xyz 2\r\n"},
		{name: "tab after sub gets prefixed", input: "SUB\tabc.xyz 2\r\n", expected: "SUB\tprefixed.abc.xyz 2\r\n"},
		{name: "mixed tabs and spaces get prefixed", input: "sub \t abc.xyz 2\r\n", expected: "sub \t prefixed.abc.xyz 2\r\n"},
		{name: "wildcard subject gets prefixed", input: "SUB > 1\r\n", expected: "SUB prefixed.> 1\r\n"},
		{name: "unrelated command with sub prefix is untouched", input: "SUBTRACT x\r\n", expected: "SUBTRACT x\r\n"},
		{name: "unsub is untouched", input: "UNSUB 1\r\n", expected: "UNSUB 1\r\n"},
		{name: "sub without a subject is untouched", input: "SUB\r\n", expected: "SUB\r\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w := bytes.NewBuffer(nil)

			if err := buffer.WriteCommandWithPrefix("prefixed.", w, []byte(tc.input)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := cmp.Diff(tc.expected, w.String()); diff != "" {
				t.Fatalf(diff)
			}
		})
	}
}
