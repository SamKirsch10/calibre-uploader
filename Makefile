BINARY     := calibre-uploader
CMD_DIR    := ./cmd/calibre-uploader
BUILD_DIR  := build
GOOS       := linux
GOARCH     := amd64

.PHONY: all build linux clean install

all: build

build: linux

linux:
	mkdir -p $(BUILD_DIR)
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) go build -trimpath -ldflags="-s -w" -o $(BUILD_DIR)/$(BINARY)-$(GOOS)-$(GOARCH) $(CMD_DIR)

# Installs the binary and systemd unit onto the local machine (must be run
# as root, on the target Linux host).
install: linux
	install -Dm755 $(BUILD_DIR)/$(BINARY)-$(GOOS)-$(GOARCH) /usr/local/bin/$(BINARY)
	install -Dm644 deploy/$(BINARY).service /etc/systemd/system/$(BINARY).service
	systemctl daemon-reload

clean:
	rm -rf $(BUILD_DIR)
