package buffer

import (
	"bytes"
	"io"
)

func CopyCommandWithPrefix(prefix string, dst io.Writer, src io.Reader) (int64, error) {
	// Tee out the first 4 bytes to check if it's a SUB command
	cmdBuffer := bytes.NewBuffer(make([]byte, 0, 4))
	cmdTeeReader := io.TeeReader(io.LimitReader(src, 4), cmdBuffer)
	cmdLen, err := io.Copy(dst, cmdTeeReader)
	if err != nil {
		return 0, err
	}
	prefixLen := 0
	if bytes.HasPrefix(cmdBuffer.Bytes(), []byte("SUB ")) {
		// write in the prefix so that the websocket can't subscribe to anything but the prefix
		prefixLen, err = dst.Write([]byte(prefix))
		if err != nil {
			return 0, err
		}
	}
	// Copy the rest of the message
	finalLen, _ := io.Copy(dst, src)
	return cmdLen + int64(prefixLen) + finalLen, err
}
