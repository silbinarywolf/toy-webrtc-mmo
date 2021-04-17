package main

import (
	"log"
	"net/http"
)

// main will start serving all files in the "dist" folder on the server
// on port 8080
func main() {
	fs := http.FileServer(http.Dir("./dist"))
	http.Handle("/", fs)

	log.Println("Listening on :8080...")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
