package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path"
)

func main() {
	host := flag.String("host", getEnvWithDefault("HOST_ADDR", ":8080"), "Host address where to run server")
	filePath := flag.String("path", getEnvWithDefault("FILE_PATH", "/var/data"), "Path where files are written")
	secretEnv := getEnvWithDefault("SECRET", "")
	secret := flag.String("secret", "", "Secret used to sign URLs")

	flag.Parse()

	sec := *secret
	if sec == "" {
		if secretEnv != "" {
			sec = secretEnv
		} else {
			log.Fatal("Must provide a secret either via SECRET env or -secret flag")
		}
	}

	p := *filePath
	if err := os.MkdirAll(p, 0700); err != nil {
		log.Fatalf("Failed to create directory: %s", err)
	}

	appendFilePath := path.Join(p, dbFileName)
	if err := InitializeDB(appendFilePath); err != nil {
		log.Fatalf("Error while initializing db due to '%s'", err)
	}

	api := NewAPI(p, []byte(*secret))
	if err := http.ListenAndServe(*host, api); err != nil {
		log.Fatal(err.Error())
	}
}

func getEnvWithDefault(varName, def string) string {
	if result := os.Getenv(varName); result != "" {
		return result
	}
	return def
}
