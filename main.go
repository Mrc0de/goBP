package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	gmux "github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/gorilla/websocket"
	"html/template"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"time"
)

var store = sessions.NewCookieStore([]byte("7MpuK2u6w7tVQvMX7srsa7md"), []byte("7MpuK2u6w7tVQvMX7srsa7mz"))

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
	// redirect to 443
	go http.ListenAndServe(":80", http.HandlerFunc(redirect))
	rmux := http.NewServeMux()
	rmux.HandleFunc("/", index)

	r := gmux.NewRouter()
	r.HandleFunc("/live", func(w http.ResponseWriter, r *http.Request) {
		upgrader.CheckOrigin = func(r *http.Request) bool { return true }
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("Websocket Upgrade Error: %s", err)
		} else {
			conns = append(conns, conn)
			connIp := conn.RemoteAddr().String()[0:strings.Index(conn.RemoteAddr().String(), ":")]
			if len(connIp) < 8 {
				//This is an ipv6, parse it differently
				connIp = connIp + "_IPv6"
			}
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
		Addr:         "0.0.0.0:443",
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      r,
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := srv.ListenAndServeTLS("/etc/letsencrypt/live/geekprojex.com/cert.pem", "/etc/letsencrypt/live/geekprojex.com/privkey.pem"); err != nil {
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

func redirect(w http.ResponseWriter, req *http.Request) {
	target := "https://geekprojex.com"
	log.Printf("redirect to: %s", target)
	http.Redirect(w, req, target, http.StatusTemporaryRedirect)
}

func index(w http.ResponseWriter, req *http.Request) {
	// all calls to unknown url paths should return 404
	if req.URL.Path != "/" && req.URL.Path != "/live" {
		log.Printf("404: %s", req.URL.String())
		http.NotFound(w, req)
		return
	}
	w.Write([]byte("404 - Use https\n"))
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
			Ip       string
			Conf     Config
			loggedIn bool
		}
	)
	data.Conf.WsHost = config.WsHost
	data.Ip = r.RemoteAddr[0:strings.Index(r.RemoteAddr, ":")]
	if len(data.Ip) < 8 {
		//It's IPV6 address, fixup
		data.Ip = data.Ip + "_IPv6"
	}
	session, err := store.Get(r, "goBPSession")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	session.Options = &sessions.Options{
		Path:     "/",
		MaxAge:   43200,
		HttpOnly: false,
		Secure:   true,
	}
	if session.IsNew {
		log.Printf("New Session Created For %s", r.RemoteAddr)
	} else {
		log.Printf("Existing Session Used For %s", r.RemoteAddr)
	}

	loggedIn := session.Values["loggedIn"]
	if loggedIn != nil {
		log.Printf("User is LoggedIn? -> %s", strconv.FormatBool(loggedIn.(bool)))
		data.loggedIn = loggedIn.(bool)
	} else {
		log.Printf("User is NOT Logged In!")
		session.Values["loggedIn"] = false
		data.loggedIn = false
	}
	sessErr := session.Save(r, w)
	if sessErr != nil {
		log.Printf("Session Save Error: %s", sessErr)
	}

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
	writeLog("[ "+data.Ip+" ] - "+r.Method+"  "+r.RequestURI, true)
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
