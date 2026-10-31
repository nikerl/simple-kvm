package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
	"io"
)

const USTREAMER_URL = "http://192.168.0.30:8080/stream"

var kvmTemplate = template.Must(template.ParseFiles("static/index.html"))

type kvmPage struct {
	UstreamerURL string `json:"ustreamer_url"`
}

func serveIndex(w http.ResponseWriter, r *http.Request) {
	if err := kvmTemplate.Execute(w, kvmPage{UstreamerURL: USTREAMER_URL}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func handleKeyStroke(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(io.LimitReader(r.Body, 64))
	fmt.Println(string(b))
}

func main() {
	fmt.Println("Starting Simple KVM")

	http.HandleFunc("/keystroke", handleKeyStroke)

	http.HandleFunc("/", serveIndex)

	fmt.Println("Listening on 0.0.0.0:8090")
	log.Fatal(http.ListenAndServe(":8090", nil))
}
