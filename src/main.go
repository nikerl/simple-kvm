package main

import (
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

func handleKeyStroke(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("upgrade:", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(512)

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
		fmt.Println(string(msg)) // e.g. "down:KeyA" / "up:KeyA"
	}
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

	mux := http.NewServeMux()

	// Login Logout
	mux.HandleFunc("GET /login", loginPage)
	mux.HandleFunc("POST /login", login)
	mux.HandleFunc("POST /logout", logout)

	// Index
	mux.HandleFunc("/", requireAuth(serveIndex))

	// Keystrokes
	mux.HandleFunc("/keystroke", requireAuth(handleKeyStroke))

	// Stream proxy
	target, _ := url.Parse(os.Getenv("USTREAMER_URL"))
	proxy := httputil.NewSingleHostReverseProxy(target)
	mux.Handle("/stream", requireAuth(proxy.ServeHTTP))

	// Screenshots proxy
	mux.HandleFunc("/snapshot", requireAuth(handleSnapshot))

	// Serve static files
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))

	fmt.Println("Listening on 0.0.0.0:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}
