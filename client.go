// Copyright 2018 Jacques Supcik / HEIA-FR
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This code is based on an example "chat" program by the Gorilla WebSocket
// authors :
// https://github.com/gorilla/websocket/blob/master/examples/chat/client.go

package ws2811

import (
	"net/http"
	"time"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"

	"github.com/gorilla/websocket"
)

const (
	// Time allowed to write a message to the peer.
	writeWait = 10 * time.Second

	// Time allowed to read the next pong message from the peer.
	pongWait = 60 * time.Second

	// Send pings to peer with this period. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

// Client is a middleman between the websocket connection and the hub.
type Client struct {
	hub *Hub

	// The websocket connection.
	conn *websocket.Conn

	// Buffered channel of outbound messages.
	send chan []byte
}

func (c *Client) sendMessage(message []byte) error {
	w, err := c.conn.NextWriter(websocket.TextMessage)
	if err != nil {
		return errors.WithMessage(err, "can't get the writer for the next message")
	}
	if _, err = w.Write(message); err != nil {
		return errors.WithMessage(err, "can't send message")
	}

	// Add queued messages to the current websocket message.
	n := len(c.send)
	for i := 0; i < n; i++ {
		if _, err = w.Write(<-c.send); err != nil {
			return errors.WithMessage(err, "can't send queued message")
		}
	}

	if err := w.Close(); err != nil {
		return errors.WithMessage(err, "can't close writer")
	}
}

// writePump pumps messages from the hub to the websocket connection.
//
// A goroutine running writePump is started for each connection. The
// application ensures that there is at most one writer to a connection by
// executing all writes from this goroutine.
func (c *Client) writePump() { // nolint:gocyclo
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		err := c.conn.Close()
		if err != nil {
			log.Error(errors.WithMessage(err, "error on closing connection"))
		}
	}()

	for {
		select {
		case message, ok := <-c.send:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Warn(errors.WithMessage(err, "can't set write deadline"))
			}
			if !ok { // The hub closed the channel.
				if err := c.conn.WriteMessage(websocket.CloseMessage, []byte{}); err != nil {
					log.Error(errors.WithMessage(err, "can't send CloseMessage to the browser"))
				}
				return
			}
			if err := c.sendMessage(message); err != nil {
				log.Error(err)
				return
			}
		case <-ticker.C:
			if err := c.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
				log.Error(errors.WithMessage(err, "can't set write deadline"))
				return
			}
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				log.Error(errors.WithMessage(err, "can't send Ping message"))
				return
			}
		}
	}
}

// ServeWs handles websocket requests from the peer.
func ServeWs(hub *Hub, w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error(errors.WithMessage(err, "can't upgrade connetion"))
		return
	}
	client := &Client{hub: hub, conn: conn, send: make(chan []byte, 256)}
	client.hub.register <- client

	// Allow collection of memory referenced by the caller by doing all work in
	// new goroutines.
	go client.writePump()
}
