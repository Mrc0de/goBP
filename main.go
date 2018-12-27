package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

func main() {
	r := mux.NewRouter()
	r.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Upgrade Error: %s", err)
		}
		for {
			// Read message from browser
			msgType, msg, err := conn.ReadMessage()
			if err != nil {
				log.Printf("ReadMessage Error: %s", err)
			}
			log.Printf("%s sent: %s\n", conn.RemoteAddr(), string(msg))
			if err = conn.WriteMessage(msgType, msg); err != nil {
				log.Printf("WriteMessage Error: %s", err)
			}
		}
	})
	r.HandleFunc("/", HomeHandler)
	fmt.Println("Starting...")

	var wait time.Duration
	flag.DurationVar(&wait, "graceful-timeout", time.Second*15, "the duration for which the server gracefully wait for existing connections to finish")
	flag.Parse()

	srv := &http.Server{
		Addr:         "0.0.0.0:80",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServe(); err != nil {
			log.Println(err)
		}
	}()
	fmt.Println("Started.")
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	// Block until  signal.
	<-c
	// Create a deadline
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	srv.Shutdown(ctx)
	log.Println("dying...")
	os.Exit(0)
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	var (
		data struct {
			Ip string
		}
	)
	data.Ip = r.RemoteAddr
	tmpl, err := template.ParseFiles("templates\\Home.html")
	if err != nil {
		log.Printf("Template Parse Error: %s\n", err)
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Template Exec Error: %s\n", err)
	}
}
