package main

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"github.com/gorilla/websocket"
	_ "github.com/joho/godotenv/autoload"
)

var USTREAMER_URL = os.Getenv("USTREAMER_URL")

var kvmTemplate = template.Must(template.ParseFiles("templates/index.html"))

type kvmPage struct {
	UstreamerURL string `json:"ustreamer_url"`
}

var upgrader websocket.Upgrader



func serveIndex(w http.ResponseWriter, r *http.Request) {
	if err := kvmTemplate.Execute(w, kvmPage{UstreamerURL: USTREAMER_URL}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func recordKeystroke(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(512)

	// Release everything when the session ends so no key stays stuck
	defer sendKeystrokesToServer("release_all", gadget)


	// Unblock ReadMessage when the session dies
	go func() {
		<-r.Context().Done()
		conn.Close()
	}()

	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if string(msg) == "ping" { // client keepalive
			continue
		}
		sendKeystrokesToServer(string(msg), gadget)
	}
}

func handleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(gadget.Status())
}

func handleSnapshot(w http.ResponseWriter, r *http.Request) {
	resp, err := http.Get(USTREAMER_URL + "/snapshot")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	io.Copy(w, resp.Body)
}



func main() {
	fmt.Println("Starting Simple KVM")

	// Create/adopt the USB HID gadget and keep it healthy
	go gadget.Watch()

	mux := http.NewServeMux()

	// Login Logout
	mux.HandleFunc("GET /login", loginPage)
	mux.HandleFunc("POST /login", login)
	mux.HandleFunc("POST /logout", logout)

	// Index
	mux.HandleFunc("/", requireAuth(serveIndex))

	// Keystrokes
	mux.HandleFunc("/keystroke", requireAuth(recordKeystroke))

	// Keyboard gadget status
	mux.HandleFunc("GET /status", requireAuth(handleStatus))

	// Stream proxy
	target, _ := url.Parse(os.Getenv("USTREAMER_URL"))
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux.Handle("/stream", requireAuth(proxy.ServeHTTP))

	// Screenshots proxy
	mux.HandleFunc("/snapshot", requireAuth(handleSnapshot))

	// Service worker must be served from the root to control the whole app
	mux.HandleFunc("GET /sw.js", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/javascript")
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, "static/sw.js")
	})

	// Serve static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Println("Listening on 0.0.0.0:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
