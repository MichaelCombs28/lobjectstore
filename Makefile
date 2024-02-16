APP_NAME := lobjectstore
VERSION := latest
PLATFORMS := linux/amd64,linux/arm64

all: build

setup-buildx:
	docker buildx create --name lobjectstore-builder --use

build: setup-buildx build-cached

build-cached:
	docker buildx build --platform $(PLATFORMS) -t michaelcombs831/$(APP_NAME):$(VERSION) --push .

clean-buildx:
	docker buildx rm lobjectstore-builder

.PHONY: all setup-buildx build clean-buildx
