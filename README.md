# lobjectstore

Local object store server for testing. Not useful to run with a bunch of replicas or using a
stateful set.

This is a toy.

## Usage

```bash
Usage of lobjectstore:
  -host string
    	Host address where to run server (default ":8080")
  -path string
    	Path where files are written (default "/var/data")
  -secret string
    	Secret used to sign URLs
```

All Values can also be passed via env variables

| Var       | Description                      |
| --------- | -------------------------------- |
| HOST_ADDR | Host address where to run server |
| FILE_PATH | Path where files are written     |
| SECRET    | Secret used to sign URLs         |
