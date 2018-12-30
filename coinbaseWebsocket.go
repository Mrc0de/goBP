package main

import (
	"encoding/json"
	"flag"
	"github.com/gorilla/websocket"
	"log"
	"net/url"
	"os"
	"os/signal"
	"time"
)

type CoinbaseSubscribeRequest struct {
	Type       string   `json:"type"`
	ProductIDs []string `json:"product_ids"`
	Channels   []string `json:"channels"`
}

func remove(slice []*websocket.Conn, s int) []*websocket.Conn {
	return append(slice[:s], slice[s+1:]...)
}

func connectWebSocket(host string, port string, path string) {
	var addr = flag.String("addr", host+":"+port, "http service address")
	////////////
	flag.Parse()
	log.SetFlags(0)
	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt)
	u := url.URL{Scheme: "wss", Host: *addr, Path: path}
	log.Printf("connecting to %s", u.String())
	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Printf("dial: %s", err)
	}
	defer c.Close()
	done := make(chan struct{})

	go func() {
		defer close(done)
		for {
			_, message, err := c.ReadMessage()
			if err != nil {
				log.Println("<coinbase websocket read error: ", err)
				return
			}
			log.Printf("<: %s", message)
			if len(conns) > 0 {
				//log.Printf("Conns Exist! %d",len(conns))
				for i, _ := range conns {
					err = conns[i].WriteMessage(websocket.TextMessage, []byte(message))
					if err != nil {
						log.Printf(":Broadcast error> %s", err)
						conns[i].Close()
						conns = remove(conns, i)
					}
				}
			}
		}
	}()

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	//subscribe
	var sub CoinbaseSubscribeRequest
	sub.Type = "subscribe"
	sub.Channels = append(sub.Channels, "ticker")
	sub.Channels = append(sub.Channels, "heartbeat")
	sub.ProductIDs = append(sub.ProductIDs, "LTC-USD")
	t, _ := json.Marshal(sub)
	err = c.WriteMessage(websocket.TextMessage, []byte(t))
	log.Printf(":Sent> - %s", t)
	if err != nil {
		log.Println(":err> ", err)
		return
	}

	for {
		select {
		case <-done:
			return
		case <-interrupt:
			{
				log.Println("interrupt")
				// Cleanly close the connection by sending a close message and then
				// waiting (with timeout) for the server to close the connection.

				err := c.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
				if err != nil {
					log.Println(":write close error: ", err)
					return
				}
				select {
				case <-done:
				case <-time.After(time.Second):
				}
				return
			}
		}
	}
}
