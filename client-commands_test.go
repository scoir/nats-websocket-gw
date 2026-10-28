package gw

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"
)

func forward(t *testing.T, settings Settings, input string) (string, error) {
	t.Helper()
	gw := NewGateway(settings)
	var out bytes.Buffer
	err := gw.forwardClientCommands(&out, bufio.NewReaderSize(nil, maxControlLine), strings.NewReader(input))
	return out.String(), err
}

func TestForwardClientCommands(t *testing.T) {
	tests := []struct {
		name     string
		settings Settings
		input    string
		expected string
		wantErr  string
	}{
		{name: "single command", input: "PING\r\n", expected: "PING\r\n"},
		{name: "empty frame", input: "", expected: ""},
		{name: "multiple commands", input: "CONNECT {}\r\nPING\r\n", expected: "CONNECT {}\r\nPING\r\n"},
		{name: "pub payload is forwarded", input: "PUB foo 5\r\nhello\r\nPING\r\n", expected: "PUB foo 5\r\nhello\r\nPING\r\n"},
		{name: "pub with reply", input: "PUB foo bar 2\r\nhi\r\n", expected: "PUB foo bar 2\r\nhi\r\n"},
		{name: "hpub is rejected", input: "HPUB foo 12 17\r\n", wantErr: "HPUB is not supported"},
		{name: "empty payload", input: "PUB foo 0\r\n\r\n", expected: "PUB foo 0\r\n\r\n"},
		{
			name:     "payload that looks like a command is not checked",
			settings: Settings{IsAllowedCommand: func(c string) bool { return c == "PUB" }},
			input:    "PUB foo 5\r\nUNSUB\r\n",
			expected: "PUB foo 5\r\nUNSUB\r\n",
		},
		{
			name:     "second command in a frame is checked",
			settings: Settings{IsAllowedCommand: func(c string) bool { return c == "PING" }},
			input:    "PING\r\nUNSUB 1\r\n",
			expected: "PING\r\n",
			wantErr:  `command not allowed: "UNSUB"`,
		},
		{
			name:     "verb is uppercased for the callback",
			settings: Settings{IsAllowedCommand: func(c string) bool { return c == "PING" }},
			input:    "ping\r\n",
			expected: "ping\r\n",
		},
		{name: "sub gets prefixed", settings: Settings{Prefix: "p."}, input: "SUB abc 2\r\n", expected: "SUB p.abc 2\r\n"},
		{name: "lowercase sub gets prefixed", settings: Settings{Prefix: "p."}, input: "sub abc 2\r\n", expected: "sub p.abc 2\r\n"},
		{name: "tab after sub gets prefixed", settings: Settings{Prefix: "p."}, input: "SUB\tabc 2\r\n", expected: "SUB\tp.abc 2\r\n"},
		{name: "payload starting with SUB is not prefixed", settings: Settings{Prefix: "p."}, input: "PUB foo 5\r\nSUB x\r\n", expected: "PUB foo 5\r\nSUB x\r\n"},
		{
			name:     "allowed publish is forwarded",
			settings: Settings{Prefix: "p.", IsAllowedPublish: func(s string) bool { return s == "ui.monitor" }},
			input:    "PUB ui.monitor 2\r\nhi\r\n",
			expected: "PUB ui.monitor 2\r\nhi\r\n",
		},
		{
			name:     "publish outside the allowlist is rejected",
			settings: Settings{Prefix: "p.", IsAllowedPublish: func(s string) bool { return s == "ui.monitor" }},
			input:    "PUB p2.secret 2\r\nhi\r\n",
			wantErr:  `publish not allowed: "p2.secret"`,
		},
		{
			name:     "lowercase publish outside the allowlist is rejected",
			settings: Settings{IsAllowedPublish: func(s string) bool { return s == "ui.monitor" }},
			input:    "pub\tp2.secret 2\r\nhi\r\n",
			wantErr:  `publish not allowed: "p2.secret"`,
		},
		{
			name:     "second publish in a frame is checked",
			settings: Settings{IsAllowedPublish: func(s string) bool { return s == "ui.monitor" }},
			input:    "PUB ui.monitor 2\r\nhi\r\nPUB p2.secret 2\r\nhi\r\n",
			expected: "PUB ui.monitor 2\r\nhi\r\n",
			wantErr:  `publish not allowed: "p2.secret"`,
		},
		{name: "reply-to inside the prefix is allowed", settings: Settings{Prefix: "p."}, input: "PUB foo p.inbox 2\r\nhi\r\n", expected: "PUB foo p.inbox 2\r\nhi\r\n"},
		{name: "reply-to outside the prefix is rejected", settings: Settings{Prefix: "p."}, input: "PUB foo p2.inbox 2\r\nhi\r\n", wantErr: `reply-to not allowed: "p2.inbox"`},
		{name: "empty line", input: "\r\n", wantErr: "empty command"},
		{name: "short line does not panic", input: "\n", wantErr: "empty command"},
		{name: "unterminated", input: "PING", wantErr: "unterminated command"},
		{name: "bad size", input: "PUB foo -1\r\n", wantErr: "invalid PUB payload size"},
		{name: "missing size", input: "PUB foo\r\n", wantErr: "wrong number of arguments"},
		{name: "payload without CRLF", input: "PUB foo 2\r\nhi", expected: "PUB foo 2\r\nhi", wantErr: "payload is not followed by CRLF"},
		{name: "payload longer than its size", input: "PUB foo 2\r\nhiSUB > 1\r\n", expected: "PUB foo 2\r\nhi", wantErr: "payload is not followed by CRLF"},
		{name: "control line longer than max_control_line", input: "SUB " + strings.Repeat("a", maxControlLine) + " 1\r\n", wantErr: "command longer than"},
		{name: "truncated payload", input: "PUB foo 10\r\nhi\r\n", expected: "PUB foo 10\r\nhi\r\n", wantErr: "payload cut short"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := forward(t, tc.settings, tc.input)
			if tc.wantErr == "" && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			if tc.wantErr != "" && !errors.Is(err, ErrInvalidCommand) {
				t.Fatalf("expected ErrInvalidCommand, got %v", err)
			}
			if out != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, out)
			}
		})
	}
}
