package main

import (
	"fmt"
	"html/template"
	"log"
	"net/http"
)

const USTREAMER_URL = "http://192.168.0.30:8080/stream"

var kvmTemplate = template.Must(template.ParseFiles("static/index.html"))

type kvmPage struct {
	UstreamerURL string `json:"ustreamer_url"`
}


func main() {
	fmt.Println("Starting Simple KVM")

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}

		if err := kvmTemplate.Execute(w, kvmPage{UstreamerURL: USTREAMER_URL}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	fmt.Println("Listening on 0.0.0.0:8090")
	log.Fatal(http.ListenAndServe(":8090", nil))
}
