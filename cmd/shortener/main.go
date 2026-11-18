package main

import (
	"log"
	"net/http"
	"os"
	"strconv"

	"shortener/internal/app"
	"shortener/internal/auth"
	"shortener/internal/storage"
)

func main() {
	secret := os.Getenv("SHORTENER_SECRET")
	if secret == "" {
		secret = "development-only-change-this-secret"
		log.Print("SHORTENER_SECRET is not set: using the educational development key")
	}
	cookieManager, err := auth.NewManager(secret)
	if err != nil {
		log.Fatal(err)
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatal("PORT must be an integer between 1 and 65535")
	}
	baseURL := os.Getenv("PUBLIC_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:" + port
	}
	server := app.New(storage.NewMemory(), cookieManager, baseURL)
	log.Printf("shortener listening on :%s (public URL: %s)", port, baseURL)
	log.Fatal(http.ListenAndServe(":"+port, server.Handler()))
}
