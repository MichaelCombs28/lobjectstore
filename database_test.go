package main

import (
	"bytes"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFreshInitialize(t *testing.T) {
	storageDir := t.TempDir()
	err := InitializeDB(path.Join(storageDir, "_db"))
	require.NoError(t, err)
}

func TestInitializeExistingDB(t *testing.T) {
	storageDir := t.TempDir()
	filepath := path.Join(storageDir, "_db")

	require.NoError(t, os.WriteFile(filepath, []byte(`
ADD 739375fe-ac9d-41e8-9360-357c7575d866 {}
	`), 0600))

	err := InitializeDB(filepath)
	require.NoError(t, err)
}

func TestOperations(t *testing.T) {
	storageDir := t.TempDir()
	manifestPath := path.Join(storageDir, "db")
	err := InitializeDB(manifestPath)
	require.NoError(t, err)

	t.Run("create file", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`{"foo": "bar"}`))
		require.NoError(t, err)
		assert.NotNil(t, storedFile)
	})

	t.Run("create test get metadata", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`{"foo": "bar"}`))
		require.NoError(t, err)
		fetched, err := GetFileMetadata(storedFile.ID)
		require.NoError(t, err)
		assert.Equal(t, storedFile, fetched)
	})

	t.Run("read file", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`1`))
		require.NoError(t, err)

		buf := bytes.NewBuffer(nil)
		require.NoError(t, ReadFile(storedFile.ID, buf))
		assert.Equal(t, "1", buf.String())
	})

	t.Run("update file append", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`1`))
		require.NoError(t, err)
		require.NoError(t, UpdateFile(storedFile.ID, strings.NewReader("\n2"), false))

		buf := bytes.NewBuffer(nil)
		require.NoError(t, ReadFile(storedFile.ID, buf))
		assert.Equal(t, "1\n2", buf.String())
	})

	t.Run("update file overwrite", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`1`))
		require.NoError(t, err)
		require.NoError(t, UpdateFile(storedFile.ID, strings.NewReader("\n2"), true))

		buf := bytes.NewBuffer(nil)
		require.NoError(t, ReadFile(storedFile.ID, buf))
		assert.Equal(t, "\n2", buf.String())
	})

	t.Run("copy file", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`1`))
		require.NoError(t, err)

		copiedFile, err := CopyFile(storedFile.ID, fname(t, storageDir, "2"))
		require.NoError(t, err)

		buf := bytes.NewBuffer(nil)
		require.NoError(t, ReadFile(copiedFile.ID, buf))
		assert.Equal(t, "1", buf.String())
	})

	t.Run("upsert file new", func(t *testing.T) {
		storedFile, err := UpsertFile(fname(t, storageDir), strings.NewReader(`{"foo": "bar"}`))
		require.NoError(t, err)
		assert.NotNil(t, storedFile)
	})

	t.Run("upsert file existing", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`{"foo": "bar"}`))
		require.NoError(t, err)

		upsertedFile, err := UpsertFile(storedFile.Path, strings.NewReader("1"))
		require.NoError(t, err)
		assert.Equal(t, storedFile, upsertedFile)

		buf := bytes.NewBuffer(nil)
		require.NoError(t, ReadFile(storedFile.ID, buf))
		assert.Equal(t, "1", buf.String())
	})

	t.Run("delete file", func(t *testing.T) {
		storedFile, err := CreateFile(fname(t, storageDir), strings.NewReader(`{"foo": "bar"}`))
		require.NoError(t, err)

		require.NoError(t, DeleteFile(storedFile.ID))
		_, err = GetFileMetadata(storedFile.ID)
		assert.ErrorIs(t, err, errNotExist)

		_, err = os.Stat(storedFile.Path)
		assert.ErrorIs(t, err, os.ErrNotExist)
	})

	t.Run("list files", func(t *testing.T) {
		files, err := ListFiles()
		require.NoError(t, err)

		m := make(map[string]storedFile, len(files))
		for _, file := range files {
			m[file.ID] = file
		}
		assert.Equal(t, storedFiles, m)
	})

	t.Run("test reload", func(t *testing.T) {
		aoFile.Close()
		aoFile = nil

		currentMap := storedFiles
		for key, sf := range currentMap {
			sf.Created = sf.Created.Round(0)
			currentMap[key] = sf
		}
		require.NoError(t, InitializeDB(manifestPath))

		// Ensure append only file generates the same map of objects
		assert.Equal(t, currentMap, storedFiles)
	})
}

func fname(t *testing.T, storageDir string, extras ...string) string {
	n := filepath.Base(t.Name())
	for _, extra := range extras {
		n = n + extra
	}
	return path.Join(storageDir, n)
}
