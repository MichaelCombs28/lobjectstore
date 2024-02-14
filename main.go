package main

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/r3labs/sse/v2"
)

type Config struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Path       string `json:"path"`
	SecretPath string `json:"secretPath"`
}

func main() {
	configPath := flag.String("config", "./config.json", "Server configuration file")
	genSecret := flag.Bool("generate-secret", false, "Ignores all secret paths and generates the secret using urandom")
	secretPath := flag.String("secret", "", "Path where secrets are located, this will override the secretpath in the config")
	flag.Parse()

	// Config
	var config Config
	configFile, err := os.Open(*configPath)
	if err != nil {
		log.Fatalf("Failed to open config path '%s' due to '%s'", *configPath, err.Error())
	}
	if err := json.NewDecoder(configFile).Decode(&config); err != nil {
		defer configFile.Close()
		log.Fatalf("Failed to parse config path '%s' due to '%s'", *configPath, err.Error())
	}
	configFile.Close()

	// Secret
	var secret []byte
	if *genSecret {
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			log.Fatalln("Failed to read random bytes")
		}
	} else {
		var path string
		if *secretPath != "" {
			path = *secretPath
		} else {
			path = config.Path
		}
		secret, err = os.ReadFile(path)
		if err != nil {
			log.Fatalf("Failed to read secret path '%s' due to '%s'", path, err.Error())
		}
	}

	// SSE Event Stream
	events := sse.New()
	stream := events.CreateStream("updates")
	stream.OnSubscribe = func(streamID string, sub *sse.Subscriber) {
		stream.Eventlog.Replay(sub)
	}

	mux := http.NewServeMux()
	mux.Handle("/events", events)

	mux.HandleFunc("/pre-signed/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := strings.TrimPrefix(r.URL.Path, "/pre-signed/")
		// Create pre-signed URL
		if r.Method == http.MethodPost {

			if len(url) > 0 {
				status := http.StatusMethodNotAllowed
				w.WriteHeader(status)
				fmt.Fprint(w, http.StatusText(status))
				return
			}

			var req CreateSignedURLRequest
			dec := json.NewDecoder(r.Body)
			if err := dec.Decode(&req); err != nil {
				w.WriteHeader(http.StatusMethodNotAllowed)
				fmt.Fprintf(w, `{"error": "%s"}`, http.StatusText(http.StatusBadRequest))
				return
			}

			url := &signedURL{
				Path:    req.Path,
				Expiry:  time.Now().Add(req.ExpiryLength),
				IsWrite: req.IsWrite,
			}
			fmt.Fprintf(w, `{"url": "%s"}`, string(toURL(secret, url)))
			return
		}

		// Basic Parsing & Verification
		payload, err := verify(secret, []byte(url))
		if err != nil {
			log.Println(err.Error())
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "invalid signature"}`)
			return
		}
		if payload.Expiry.After(time.Now()) {
			w.WriteHeader(http.StatusBadRequest)
			fmt.Fprint(w, `{"error": "link expired"}`)
			return
		}
		filePath := filepath.Join(config.Path, payload.Path)

		// Handlers
		if r.Method == http.MethodPut {
			if !payload.IsWrite {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error": "signed url does not support writing"}`)
				return
			}
			file, err := os.OpenFile(filePath, os.O_WRONLY, 0600)
			if err != nil {
				log.Println(err.Error())
				fmt.Fprint(w, `{"error": "internal error"}`)
				return
			}
			defer file.Close()

			if _, err := io.Copy(file, r.Body); err != nil {
				log.Println(err.Error())
				os.Remove(filePath)
				fmt.Fprint(w, `{"error": "internal error"}`)
				return
			}
			events.Publish(url, &sse.Event{
				Data: []byte(fmt.Sprintf(`{"event": "FileCreated", "id": "%s"}`, url)),
			})
			fmt.Fprint(w, `{"message": "successfully wrote file"}`)
			return

		} else if r.Method == http.MethodGet {
			if payload.IsWrite {
				w.WriteHeader(http.StatusForbidden)
				fmt.Fprint(w, `{"error": "signed url does not support reading"}`)
				return
			}
			file, err := os.Open(filePath)
			if err != nil {
				log.Println(err.Error())
				fmt.Fprint(w, `{"error": "internal error"}`)
				return
			}
			defer file.Close()
			mimeType := mime.TypeByExtension(filepath.Ext(filePath))
			w.Header().Add("Content-Type", mimeType)
			io.Copy(w, file)
			return
		} else {
			w.WriteHeader(http.StatusMethodNotAllowed)
			fmt.Fprintf(w, `{"error": "%s"}`, http.StatusText(http.StatusMethodNotAllowed))
			return
		}
	}))

	mux.HandleFunc("/objects/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/objects/")
		filePath := filepath.Join(config.Path, path)
		if r.Method == http.MethodGet {
			file, err := os.Open(filePath)
			if err != nil {
				var status int
				if errors.Is(err, os.ErrNotExist) {
					status = http.StatusNotFound
				} else {
					status = http.StatusInternalServerError
				}
				w.WriteHeader(status)
				fmt.Fprint(w, http.StatusText(status))
				return
			}
			defer file.Close()
			mimeType := mime.TypeByExtension(filepath.Ext(filePath))
			w.Header().Add("Content-Type", mimeType)
			io.Copy(w, file)
			return
		} else if r.Method == http.MethodPost {
			if len(path) > 0 {
				status := http.StatusMethodNotAllowed
				w.WriteHeader(status)
				fmt.Fprint(w, http.StatusText(status))
				return
			}
		} else if r.Method == http.MethodPut {
		} else if r.Method == http.MethodDelete {
		}
		status := http.StatusMethodNotAllowed
		w.WriteHeader(status)
		fmt.Fprint(w, http.StatusText(status))
		return
	}))
}

type CreateSignedURLRequest struct {
	Path         string        `json:"path"`
	ExpiryLength time.Duration `json:"expiryLength"`
	IsWrite      bool          `json:"isWrite"`
}
