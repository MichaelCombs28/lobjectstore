# lobjectstore

Local object store server for testing. Not useful to run with a bunch of replicas or using a
stateful set.

This is a toy.

## Usage

```bash
Usage of lobjectstore:
  -config string
      Server configuration file (default "./config.json")
  -generate-secret
      Ignores all secret paths and generates the secret using urandom
  -secret string
      Path where secrets are located, this will override the secretpath in the config
```
