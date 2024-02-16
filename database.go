package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"os/signal"
	"path"
	"path/filepath"
	"sync"
	"time"
)

const dbFileName string = "_db"

// Errors
var (
	errNotInitialized = errors.New("Server DB hsa not been initialized")
	errNotExist       = errors.New("File does not exist")
	errExist          = errors.New("File already exists")
	errExiting        = errors.New("Server is exiting")
)

// State
var (
	rwlock      = new(sync.RWMutex)
	storedFiles map[string]storedFile
	aoFile      *os.File
)

func InitializeDB(filepath string) error {
	rwlock.Lock()
	defer rwlock.Unlock()
	storedFiles = make(map[string]storedFile)
	f, err := os.OpenFile(filepath, os.O_RDONLY, 0600)

	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		aoFile, err = os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
		return err
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			continue
		}
		var (
			action string
			id     string
			js     []byte
			sf     storedFile
		)
		_, err := fmt.Sscanf(text, "%s %s %s", &action, &id, &js)
		if err != nil {
			continue
		}

		if action == "ADD" {
			if err := json.Unmarshal(js, &sf); err != nil {
				return fmt.Errorf("File is corrupt '%s'; Attempting to parse '%s'", err.Error(), js)
			}
			storedFiles[id] = sf
		} else if action == "DEL" {
			delete(storedFiles, id)
		} else {
			return fmt.Errorf("Storage file is corrupt; Received action: '%s'", action)
		}
	}
	f.Close()
	aoFile, err = os.OpenFile(filepath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return err
	}

	c := make(chan os.Signal, 1)
	go func() {
		<-c
		rwlock.Lock()
		defer rwlock.Unlock()
		aoFile.Close()
		aoFile = nil
		os.Exit(0)
	}()
	signal.Notify(c, os.Interrupt, os.Kill)
	return nil
}

type storedFile struct {
	ID      string    `json:"id"`
	Path    string    `json:"path"`
	Created time.Time `json:"created"`
}

func GetFileMetadata(id string) (s *storedFile, err error) {
	rwlock.RLock()
	defer rwlock.RUnlock()
	return getFileMetadata(id)
}

func getFileMetadata(id string) (s *storedFile, err error) {
	if storedFiles == nil {
		err = errNotInitialized
		return
	}

	f, ok := storedFiles[id]
	if !ok {
		err = errNotExist
		return
	}
	return &f, nil
}

func ListFiles() ([]storedFile, error) {
	rwlock.RLock()
	defer rwlock.RUnlock()
	if storedFiles == nil {
		return nil, errNotInitialized
	}
	results := make([]storedFile, len(storedFiles))
	i := 0
	for _, sf := range storedFiles {
		results[i] = sf
		i++
	}
	return results, nil
}

func ReadFile(id string, writer io.Writer, header ...http.Header) error {
	rwlock.RLock()
	defer rwlock.RUnlock()
	metadata, err := getFileMetadata(id)
	if err != nil {
		return err
	}

	if len(header) > 0 {
		header[0].Set("Content-Type", mime.TypeByExtension(metadata.Path))
	}

	f, err := os.Open(metadata.Path)
	if err != nil {
		return err
	}

	defer f.Close()
	_, err = io.Copy(writer, f)
	return err
}

func CreateFile(path string, reader io.Reader) (*storedFile, error) {
	rwlock.Lock()
	defer rwlock.Unlock()
	return createFile(path, reader)
}

func createFile(path string, reader io.Reader) (*storedFile, error) {
	if aoFile == nil {
		return nil, errExiting
	}
	for _, sf := range storedFiles {
		if sf.Path == path {
			return nil, errExist
		}
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	_, err = io.Copy(f, reader)
	if err != nil {
		f.Close()
		os.Remove(path)
		return nil, err
	}
	id := generateRandomUUID()
	s := storedFile{
		ID:      id,
		Path:    path,
		Created: time.Now(),
	}
	storedFiles[id] = s
	b, _ := json.Marshal(s)
	fmt.Fprintf(aoFile, "\nADD %s %s", id, b)
	aoFile.Sync()
	return &s, nil
}

func UpdateFile(id string, reader io.Reader, overwrite bool) error {
	rwlock.Lock()
	defer rwlock.Unlock()
	return updateFile(id, reader, overwrite)
}

func CopyFile(id string) (*storedFile, error) {
	rwlock.Lock()
	defer rwlock.Unlock()
	s, err := getFileMetadata(id)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	p := path.Join(filepath.Dir(s.Path), fmt.Sprintf("copy_%s_%s", generateRandomUUID(), filepath.Base(s.Path)))
	return createFile(p, f)
}

func updateFile(id string, reader io.Reader, overwrite bool) error {
	if aoFile == nil {
		return errExiting
	}

	s, err := getFileMetadata(id)
	if err != nil {
		return err
	}

	flags := os.O_WRONLY
	if overwrite {
		flags = flags | os.O_TRUNC
	} else {
		// Append
		flags = flags | os.O_APPEND
	}

	f, err := os.OpenFile(s.Path, flags, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, reader)
	return err
}

func UpsertFile(filepath string, reader io.Reader) (result *storedFile, created bool, err error) {
	rwlock.Lock()
	defer rwlock.Unlock()
	for _, sf := range storedFiles {
		if sf.Path == filepath {
			result = &sf
			err = updateFile(sf.ID, reader, true)
			return
		}
	}
	created = true
	result, err = createFile(filepath, reader)
	return
}

func DeleteFile(id string) error {
	// Could stripe, who cares right now?
	rwlock.Lock()
	defer rwlock.Unlock()
	if aoFile == nil {
		return errExiting
	}
	metadata, err := getFileMetadata(id)
	if err != nil {
		return err
	}
	err = os.Remove(metadata.Path)
	if err == nil {
		delete(storedFiles, id)
	}
	fmt.Fprintf(aoFile, "\nDEL %s %s", id, "{}")
	return err
}
