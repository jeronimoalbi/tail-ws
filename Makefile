.PHONY: tailws
all: build

build: tailws

install: install_tailws

tailws:
	@echo "Building tail-ws"
	go build -o build/tail-ws ./cmd/tail-ws

install_tailws:
	@echo "Installing tail-ws"
	go install ./cmd/tail-ws

clean:
	rm -rf build
