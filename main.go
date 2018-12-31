package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"time"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

type Config struct {
	WsHost string `json:"wsHost"`
	DbHost string `json:"dbHost"`
	DbName string `json:"dbName"`
	DbUser string `json:"dbUser"`
	DbPass string `json:"dbPass"`
}

var config Config
var c *websocket.Conn
var conns []*websocket.Conn

func main() {
	loadConfig()
	r := mux.NewRouter()
	r.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		upgrader.CheckOrigin = func(r *http.Request) bool { return true }
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket Upgrade Error: %s", err)
		} else {
			conns = append(conns, conn)
			connIp := conn.RemoteAddr().String()[0:strings.Index(conn.RemoteAddr().String(), ":")]
			writeLog("[ "+connIp+" ] - WEBSOCKET - "+r.RequestURI, true)
			writeLog("[ "+connIp+" ] - Connected...", true)
			broadCastWebSocketChat("[ "+connIp+" ] Connected.", conn)
			for {
				_, msg, err := conn.ReadMessage()
				if err != nil {
					log.Printf("ReadMessage Error: %s", err)
					writeLog(connIp+" Read Error: "+string(err.Error()), true)
					for i, _ := range conns {
						if conns[i] == conn {
							conns = remove(conns, i)
						}
					}
					break
				}
				if len(string(msg)) > 0 {
					log.Printf("%s sent: %s\n", conn.RemoteAddr(), string(msg))
					writeLog(connIp+" sent: "+string(msg), false)
					broadCastWebSocketChat(connIp+": "+string(msg), conn)
				}
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
	// Setup CB Websock
	sockUrl := "ws-feed.pro.coinbase.com"
	connectWebSocket(sockUrl, "443", "")
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

func broadCastWebSocketChat(said string, sayer *websocket.Conn) {
	if len(conns) > 0 {
		//log.Printf("Conns Exist! %d",len(conns))
		for i, _ := range conns {
			err := conns[i].WriteMessage(websocket.TextMessage, []byte(said))
			if err != nil {
				log.Printf(":Broadcast error> %s", err)
			}
		}
	}
}

func HomeHandler(w http.ResponseWriter, r *http.Request) {
	var (
		data struct {
			Ip   string
			Conf Config
		}
	)
	data.Conf.WsHost = config.WsHost
	data.Ip = r.RemoteAddr[0:strings.Index(r.RemoteAddr, ":")]
	var files []string
	if runtime.GOOS == "windows" {
		files = append(files, "templates\\Home.html", "templates\\Base.html")
	} else {
		files = append(files, "./templates/Home.html", "./templates/Base.html")
	}
	tmpl, err := template.ParseFiles(files[0], files[1])
	if err != nil {
		log.Printf("Template Parse Error: %s\n", err)
		writeLog("Template Parse Error: "+err.Error(), false)
	}
	err = tmpl.Execute(w, data)
	if err != nil {
		log.Printf("Template Exec Error: %s\n", err)
		writeLog("Template Exec Error: "+err.Error(), false)
	}
	writeLog("["+data.Ip+"] - "+r.Method+"  "+r.RequestURI, true)
}

func loadConfig() {
	f, err := os.OpenFile("./goBP.json", os.O_RDONLY, os.ModePerm)

	if err != nil {
		writeLog("Error Loading Config..."+err.Error(), true)
		os.Exit(0)
	}
	if err != nil {
		writeLog("Error Reading config.json..."+err.Error(), true)
		os.Exit(0)
	}

	writeLog("Loading Config...", true)
	d := json.NewDecoder(f)
	err = d.Decode(&config)
	if err != nil {
		writeLog("Error Decoding config.json..."+err.Error(), true)
		os.Exit(0)
	}
	writeLog("Websocket Host: "+config.WsHost, true)
	writeLog("Database Host: "+config.DbHost, true)
}

func writeLog(msg string, printStdout bool) {
	msg = msg + "\n"
	f, err := os.OpenFile("./goBP.log", os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Printf("Error writing to goBP.log: %s\n", err)
	}
	defer f.Close()
	if _, err = f.WriteString("[LOG] - " + msg); err != nil {
		fmt.Printf("Error writing to goBP.log: %s\n", err)
	}
	if printStdout {
		fmt.Printf("%s", msg)
	}
}
