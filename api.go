package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/r3labs/sse/v2"
)

const maxUploadSize = 10 << 20

func NewAPI(path string, secret []byte) *api {
	a := &api{
		mux:    http.NewServeMux(),
		path:   path,
		secret: secret,
	}
	a.init()
	return a
}

type ErrorResponse struct {
	Error string `json:"error"`
}

type api struct {
	mux    *http.ServeMux
	secret []byte
	path   string
	events *sse.Server
}

func (a *api) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mux.ServeHTTP(w, r)
}

func (a *api) publishCreated(id string) {
	a.events.Publish("updates", &sse.Event{
		Data: []byte(fmt.Sprintf(`{"event": "FileCreated", "id": "%s"}`, id)),
	})
}

func (a *api) init() {

	// SSE Event Stream
	events := sse.New()
	stream := events.CreateStream("updates")
	stream.OnSubscribe = func(streamID string, sub *sse.Subscriber) {
		stream.Eventlog.Replay(sub)
	}
	a.events = events
	a.mux.Handle("/events", events)
	a.mux.HandleFunc("/pre-signed", a.CreatePresigned)
	a.mux.HandleFunc("/pre-signed/", a.Presigned)
	a.mux.HandleFunc("/objects/", a.Objects)
	a.mux.HandleFunc("/publish/", a.PublishCreated)
}

func (a *api) Objects(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.GetObject(w, r)
		return
	} else if r.Method == http.MethodPost {
		if strings.HasSuffix(r.URL.Path, "/copy") {
			a.CopyObject(w, r)
			return
		}
		a.CreateObject(w, r)
		return
	} else if r.Method == http.MethodPut {
		a.OverwriteObject(w, r)
		return
	} else if r.Method == http.MethodPatch {
		a.UpdateFile(w, r)
		return
	} else if r.Method == http.MethodDelete {
		a.DeleteObject(w, r)
		return
	}
	status := http.StatusMethodNotAllowed
	w.WriteHeader(status)
	fmt.Fprint(w, http.StatusText(status))
	return
}

func (a *api) CopyObject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/objects/"), "/copy")
	if id == "" {
		http.NotFound(w, r)
		return
	}
	newFile, err := CopyFile(id)
	if err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
		return
	}
	w.Header().Add("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.Encode(newFile)
	return
}

func (a *api) GetObject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/objects/")
	// List files
	if len(id) < 1 {
		files, err := ListFiles()
		if err != nil {
			internalError(err, w, r)
			return
		}
		w.Header().Add("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.Encode(files)
		return
	}

	if err := ReadFile(id, w, w.Header()); err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
		return
	}
}

type CreateObjectResponse struct {
	ID string `json:"id"`
}

func (a *api) CreateObject(w http.ResponseWriter, r *http.Request) {
	if len(strings.TrimPrefix(r.URL.Path, "/objects/")) > 0 {
		methodNotAllowed(w, r)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

	if err := r.ParseMultipartForm(maxUploadSize); err != nil {
		badRequest(w, r, "Error while parsing multipart: '%s'", err)
		return
	}

	file, fileHeader, err := r.FormFile("file")

	if err != nil {
		badRequest(w, r, "Malformed request payload due to: '%s'", err)
		return
	}

	defer file.Close()
	fileName := path.Base(fileHeader.Filename)
	storedFile, err := CreateFile(path.Join(a.path, fileName), file)

	if err != nil {
		if errors.Is(err, errExist) {
			badRequest(w, r, "File with name '%s' already exists", fileName)
			return
		}
		internalError(err, w, r)
		return
	}
	a.publishCreated(storedFile.ID)

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	enc := json.NewEncoder(w)
	enc.Encode(CreateObjectResponse{
		ID: storedFile.ID,
	})
}

func (a *api) PublishCreated(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/publish/")
	if len(id) < 1 {
		http.NotFound(w, r)
		return
	}

	_, err := GetFileMetadata(id)
	if err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
		return
	}
	a.publishCreated(id)
}

func (a *api) OverwriteObject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/objects/")
	if len(id) < 1 {
		methodNotAllowed(w, r)
		return
	}
	if err := UpdateFile(id, r.Body, true); err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
	}
	w.WriteHeader(200)
}

func (a *api) UpdateFile(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/objects/")
	if len(id) < 1 {
		methodNotAllowed(w, r)
		return
	}
	if err := UpdateFile(id, r.Body, false); err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
	}
	w.WriteHeader(200)
}

func (a *api) DeleteObject(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/objects/")
	if len(id) < 1 {
		methodNotAllowed(w, r)
		return
	}

	if err := DeleteFile(id); err != nil {
		if errors.Is(err, errNotExist) {
			http.NotFound(w, r)
			return
		}
		internalError(err, w, r)
	}
}

type CreateSignedURLRequest struct {
	Path         string `json:"path"`
	ExpiryLength string `json:"expiryLength"`
}

func (a *api) CreatePresigned(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		status := http.StatusMethodNotAllowed
		w.WriteHeader(status)
		fmt.Fprint(w, http.StatusText(status))
		return
	}

	var req CreateSignedURLRequest
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&req); err != nil {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "%s"}`, err)
		return
	}

	dur, err := time.ParseDuration(req.ExpiryLength)
	if err != nil {
		badRequest(w, r, "Failed to parse expiryLength due to '%s'", err)
		return
	}
	url := &signedURL{
		Path:   req.Path,
		Expiry: time.Now().Add(dur),
	}
	fmt.Fprintf(w, `{"url": "%s"}`, string(toURL(a.secret, url)))
}

func (a *api) Presigned(w http.ResponseWriter, r *http.Request) {
	url := strings.TrimPrefix(r.URL.Path, "/pre-signed/")
	// Create pre-signed URL
	if r.Method == http.MethodPost {
		if len(url) > 0 {
			status := http.StatusMethodNotAllowed
			w.WriteHeader(status)
			fmt.Fprint(w, http.StatusText(status))
			return
		}
		a.CreatePresigned(w, r)
		return
	}

	// Basic Parsing & Verification
	payload, err := verify(a.secret, []byte(url))
	if err != nil {
		log.Println(err.Error())
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "invalid signature"}`)
		return
	}
	if payload.Expiry.Before(time.Now()) {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprint(w, `{"error": "link expired"}`)
		return
	}
	filePath := filepath.Join(a.path, payload.Path)

	// Handlers
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		fmt.Fprintf(w, `{"error": "%s"}`, http.StatusText(http.StatusMethodNotAllowed))
		return
	}

	result, created, err := UpsertFile(filePath, r.Body)
	if err != nil {
		internalError(err, w, r)
		return
	}

	w.Header().Add("Content-Type", "application/json")
	if created {
		a.publishCreated(result.ID)
		w.WriteHeader(http.StatusCreated)
	}
	enc := json.NewEncoder(w)
	enc.Encode(CreateObjectResponse{
		ID: result.ID,
	})
	return
}

func internalError(err error, w http.ResponseWriter, r *http.Request) {
	log.Printf("Internal Server Error: '%s'\n", err)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	enc := json.NewEncoder(w)
	enc.Encode(ErrorResponse{
		Error: "Internal Error",
	})
}

func badRequest(w http.ResponseWriter, r *http.Request, message string, extras ...any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	enc := json.NewEncoder(w)
	enc.Encode(ErrorResponse{
		Error: fmt.Sprintf(message, extras...),
	})
}

func methodNotAllowed(w http.ResponseWriter, r *http.Request) {
	status := http.StatusMethodNotAllowed
	w.WriteHeader(status)
	fmt.Fprint(w, http.StatusText(status))
}
