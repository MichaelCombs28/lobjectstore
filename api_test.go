package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"sync"
	"testing"

	"github.com/r3labs/sse/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPI(t *testing.T) {
	storageDir := t.TempDir()

	storedFiles = make(map[string]storedFile)
	aoFile, _ = os.OpenFile(path.Join(storageDir, "_db"), os.O_CREATE|os.O_WRONLY, 0600)
	defer aoFile.Close()

	api := NewAPI(storageDir, []byte("testing"))
	server := httptest.NewServer(api)
	url := server.URL

	var wg sync.WaitGroup
	wg.Add(2)
	client := sse.NewClient(url + "/events")
	go client.Subscribe("updates", func(msg *sse.Event) {
		fmt.Println("firsto " + string(msg.Data))
		wg.Done()
	})

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", "test.txt")
	require.NoError(t, err)
	_, err = fmt.Fprint(fw, "some file with testing info")
	require.NoError(t, err)

	mw.Close()
	req, err := http.NewRequest(http.MethodPost, url+"/objects/", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	if !assert.Equal(t, 201, resp.StatusCode) {
		s, _ := io.ReadAll(resp.Body)
		require.Fail(t, string(s))
	}
	var i struct {
		ID string `json:"id"`
	}
	dec := json.NewDecoder(resp.Body)
	require.NoError(t, dec.Decode(&i))

	resp2, err := http.Post(url+"/publish/"+i.ID, "application/json", nil)
	require.NoError(t, err)
	defer resp2.Body.Close()
	require.Equal(t, 200, resp2.StatusCode)

	// Ensure 2 calls
	wg.Wait()
}
