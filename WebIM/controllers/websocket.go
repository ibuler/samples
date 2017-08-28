// Copyright 2013 Beego Samples authors
//
// Licensed under the Apache License, Version 2.0 (the "License"): you may
// not use this file except in compliance with the License. You may obtain
// a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
// WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
// License for the specific language governing permissions and limitations
// under the License.

package controllers

import (
	"bufio"
	"encoding/json"
	"fmt"
	"github.com/astaxie/beego"
	"github.com/beego/samples/WebIM/models"
	"github.com/fsnotify/fsnotify"
	"github.com/gorilla/websocket"
	"io"
	"net/http"
	"os"
	"time"
)

// WebSocketController handles WebSocket requests.
type WebSocketController struct {
	baseController
}

// Get method handles GET requests for WebSocketController.
func (this *WebSocketController) Get() {
	// Safe check.
	uname := this.GetString("uname")
	if len(uname) == 0 {
		this.Redirect("/", 302)
		return
	}

	this.TplName = "websocket.html"
	this.Data["IsWebSocket"] = true
	this.Data["UserName"] = uname
}

// Join method handles WebSocket requests for WebSocketController.
func (this *WebSocketController) Join() {
	uname := this.GetString("uname")
	if len(uname) == 0 {
		this.Redirect("/", 302)
		return
	}

	// Upgrade from http request to WebSocket.
	ws, err := websocket.Upgrade(this.Ctx.ResponseWriter, this.Ctx.Request, nil, 1024, 1024)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(this.Ctx.ResponseWriter, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		beego.Error("Cannot setup WebSocket connection:", err)
		return
	}

	// Join chat room.
	Join(uname, ws)
	defer Leave(uname)

	// Message receive loop.
	for {
		_, p, err := ws.ReadMessage()
		if err != nil {
			return
		}
		publish <- newEvent(models.EVENT_MESSAGE, uname, string(p))
	}
}

func (this *WebSocketController) Tail() {
	//jobName := this.GetString("job")
	logFilePath := "/tmp/foo"
	ch := make(chan struct{})

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		beego.Error("Cannot new fsnotify watcher")
		return
	}

	defer watcher.Close()

	ws, err := websocket.Upgrade(this.Ctx.ResponseWriter, this.Ctx.Request, nil, 1024, 1024)
	fmt.Println(ws)
	if _, ok := err.(websocket.HandshakeError); ok {
		http.Error(this.Ctx.ResponseWriter, "Not a websocket handshake", 400)
		return
	} else if err != nil {
		beego.Error("Cannot setup WebSocket connection:", err)
		return
	}

	go func() {
		// 等到文件存在，然后再watch
		for {
			if _, err := os.Stat(logFilePath); err != nil {
				time.Sleep(1 * time.Second)
				continue
			} else {
				if err = watcher.Add(logFilePath); err != nil {
					beego.Error("log")
					continue
				} else {
					break
				}
			}
		}

		// 监听watch事件
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Write == fsnotify.Write {
					ch <- struct{}{}
				}
			case err := <-watcher.Errors:
				beego.Error("watch have error: %s" + err.Error())
			}
		}
	}()

	var inputReader *bufio.Reader
	for {
		inputFile, err := os.Open(logFilePath)
		if err != nil {
			continue
		}
		defer inputFile.Close()
		inputReader = bufio.NewReader(inputFile)
		break
	}

	for {
		select {
		case <-ch:
			fmt.Println("Get event from ch: ")
			sum := 0
			{
				for {
					sum += 1
					fmt.Println(sum)
					text, err := inputReader.ReadString('\n')
					fmt.Println(text)
					if err == io.EOF {
						break
					}
					event := models.Event{
						Type:      models.EVENT_MESSAGE,
						User:      "guang",
						Timestamp: int(time.Now().Unix()),
						Content:   text,
					}
					data, err := json.Marshal(event)
					if err != nil {
						beego.Error("Fail to marshal event:", err)
						return
					}

					ws.WriteMessage(websocket.TextMessage, data)
				}
			}
		}
	}
}

// broadcastWebSocket broadcasts messages to WebSocket users.
func broadcastWebSocket(event models.Event) {
	data, err := json.Marshal(event)
	if err != nil {
		beego.Error("Fail to marshal event:", err)
		return
	}

	for sub := subscribers.Front(); sub != nil; sub = sub.Next() {
		// Immediately send event to WebSocket users.
		ws := sub.Value.(Subscriber).Conn
		if ws != nil {
			if ws.WriteMessage(websocket.TextMessage, data) != nil {
				// User disconnected.
				unsubscribe <- sub.Value.(Subscriber).Name
			}
		}
	}
}
