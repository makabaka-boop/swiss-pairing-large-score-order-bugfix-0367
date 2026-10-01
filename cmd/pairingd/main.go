// Command pairingd runs the next-round pairing HTTP service.
package main

import (
	"log"
	"net/http"
	"os"

	"pairing"
)

func main() {
	addr := os.Getenv("PAIRING_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("pairing service listening on %s", addr)
	if err := http.ListenAndServe(addr, pairing.NewHandler()); err != nil {
		log.Fatal(err)
	}
}
