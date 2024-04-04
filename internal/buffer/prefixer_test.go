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
		{name: "non-sub command is untouched", input: "PING", expected: "PING"},
		{name: "sub command gets prefixed", input: "SUB abc.xyz 2", expected: "SUB prefixed.abc.xyz 2"},
		{name: "sub command with multiple spaces gets prefixed", input: "SUB      abc.xyz 2", expected: "SUB prefixed.     abc.xyz 2"},
		{name: "unrelated command with sub prefix is untouched", input: "SUBTRACT", expected: "SUBTRACT"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := bytes.NewReader([]byte(tc.input))
			w := bytes.NewBuffer(nil)

			count, err := buffer.CopyCommandWithPrefix("prefixed.", w, r)
			actual := w.Bytes()
			if err != nil {
				t.Fatalf("unexpected error: %e", err)
			}
			diff := cmp.Diff(string(actual), tc.expected)
			if diff != "" {
				t.Fatalf(diff)
			}
			if count != int64(len(tc.expected)) {
				t.Fatalf("expected count %d, got %d", len(tc.expected), count)
			}
		})
	}
}
