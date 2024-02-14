package main

import (
	"errors"
	"sync"
	"time"
)

var (
	errNotInitialized = errors.New("Server DB hsa not been initialized")
	errNotExist       = errors.New("File does not exist")
)

var (
	rwlock      = new(sync.RWMutex)
	storedFiles map[string]storedFile
)

type storedFile struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Created time.Time `json:"created"`
}

func getFileMetadata(id string) (s storedFile, err error) {
	rwlock.RLock()
	if storedFiles == nil {
		err = errNotInitialized
		return
	}

	f, ok := storedFiles[id]
	if !ok {
		err = errNotExist
		return
	}
	return f, nil
}
