package gw

import (
	"bufio"
	"bytes"
	"fmt"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

type ErrorHandler func(error)
type ConnectHandler func(*NatsConn, *websocket.Conn) error

type NatsServerInfo string

type Settings struct {
	NatsAddr         string
	ConnectHandler   ConnectHandler
	ErrorHandler     ErrorHandler
	WSUpgrader       *websocket.Upgrader
	Prefix           string
	IsAllowedCommand func(cmd string) bool
	IsAllowedPublish func(subject string) bool
}

type Gateway struct {
	settings      Settings
	onError       ErrorHandler
	handleConnect ConnectHandler
}

var defaultUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type NatsConn struct {
	Conn       net.Conn
	CmdReader  CommandsReader
	ServerInfo NatsServerInfo
}

func (gw *Gateway) defaultConnectHandler(natsConn *NatsConn, wsConn *websocket.Conn) error {
	// Default behavior is to let the client on the other side do the CONNECT
	// after having forwarded the 'INFO' command
	infoCmd := append([]byte("INFO "), []byte(natsConn.ServerInfo)...)
	infoCmd = append(infoCmd, byte('\r'), byte('\n'))
	if err := wsConn.WriteMessage(websocket.TextMessage, infoCmd); err != nil {
		return err
	}
	return nil
}

func defaultErrorHandler(err error) {
	fmt.Println("[ERROR]", err)
}

func NewGateway(settings Settings) *Gateway {
	gw := Gateway{
		settings: settings,
	}
	gw.setErrorHandler(settings.ErrorHandler)
	gw.setConnectHandler(settings.ConnectHandler)
	return &gw
}

func (gw *Gateway) setErrorHandler(handler ErrorHandler) {
	if handler == nil {
		gw.onError = defaultErrorHandler
	} else {
		gw.onError = handler
	}
}

func (gw *Gateway) setConnectHandler(handler ConnectHandler) {
	if handler == nil {
		gw.handleConnect = gw.defaultConnectHandler
	} else {
		gw.handleConnect = handler
	}
}

func (gw *Gateway) natsToWsWorker(ws *websocket.Conn, src CommandsReader, doneCh chan<- bool) {
	defer func() {
		doneCh <- true
	}()

	for {
		cmd, err := src.nextCommand()
		if err != nil {
			gw.onError(err)
			return
		}
		if err := ws.WriteMessage(websocket.TextMessage, cmd); err != nil {
			gw.onError(err)
			return
		}
	}
}

func (gw *Gateway) wsToNatsWorker(nats net.Conn, ws *websocket.Conn, doneCh chan<- bool) {
	defer func() {
		doneCh <- true
	}()
	br := bufio.NewReaderSize(nil, maxControlLine)
	out := bufio.NewWriter(nats)

	for {
		ws.SetReadDeadline(time.Now().Add(60 * time.Second))
		_, src, err := ws.NextReader()
		if err != nil {
			gw.onError(err)
			return
		}
		if err = gw.forwardClientCommands(out, br, src); err == nil {
			err = out.Flush()
		}
		if err != nil {
			gw.onError(err)
			return
		}
	}
}

func (gw *Gateway) Handler(w http.ResponseWriter, r *http.Request) {
	upgrader := defaultUpgrader
	if gw.settings.WSUpgrader != nil {
		upgrader = *gw.settings.WSUpgrader
	}
	wsConn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		gw.onError(err)
		return
	}
	natsConn, err := gw.initNatsConnectionForWSConn(wsConn)
	if err != nil {
		gw.onError(err)
		return
	}

	doneCh := make(chan bool)

	go gw.natsToWsWorker(wsConn, natsConn.CmdReader, doneCh)
	go gw.wsToNatsWorker(natsConn.Conn, wsConn, doneCh)

	<-doneCh

	err = natsConn.Conn.Close()
	if err != nil {
		log.Println("unable to close nats connection", err)
	}
	err = wsConn.Close()
	if err != nil {
		log.Println("unable to close ws connection", err)
	}

	<-doneCh
}

func readInfo(cmd []byte) (NatsServerInfo, error) {
	if !bytes.Equal(cmd[:5], []byte("INFO ")) {
		return "", fmt.Errorf("Invalid 'INFO' command: %s", string(cmd))
	}
	return NatsServerInfo(cmd[5 : len(cmd)-2]), nil
}

// initNatsConnectionForRequest open a connection to the nats server, consume the
// INFO message if needed, and finally handle the CONNECT
func (gw *Gateway) initNatsConnectionForWSConn(wsConn *websocket.Conn) (*NatsConn, error) {
	conn, err := net.Dial("tcp", gw.settings.NatsAddr)
	if err != nil {
		return nil, err
	}
	natsConn := NatsConn{Conn: conn, CmdReader: NewCommandsReader(conn)}

	// read the INFO, keep it
	infoCmd, err := natsConn.CmdReader.nextCommand()
	if err != nil {
		return nil, err
	}

	info, err := readInfo(infoCmd)

	if err != nil {
		return nil, err
	}

	natsConn.ServerInfo = info

	if err := gw.handleConnect(&natsConn, wsConn); err != nil {
		return nil, err
	}

	return &natsConn, nil
}
