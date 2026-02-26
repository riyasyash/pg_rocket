.PHONY: build test clean install help

BINARY_NAME=pg_rocket
VERSION=0.0.1

help:
	@echo "pg_rocket Makefile"
	@echo ""
	@echo "Available targets:"
	@echo "  build       - Build the binary"
	@echo "  test        - Run integration tests"
	@echo "  clean       - Remove build artifacts"
	@echo "  install     - Install binary to GOPATH/bin"
	@echo "  help        - Show this help message"

build:
	@echo "Building $(BINARY_NAME)..."
	go build -buildvcs=false -ldflags="-X 'github.com/riyasyash/pg_rocket/cmd.Version=$(VERSION)'" -o $(BINARY_NAME)
	@echo "Build complete: ./$(BINARY_NAME)"

test: build
	@echo "Running integration tests..."
	./test/integration/run_tests.sh

clean:
	@echo "Cleaning up..."
	rm -f $(BINARY_NAME)
	rm -rf test/fixtures/*.sql test/fixtures/*.json
	@echo "Clean complete"

install: build
	@echo "Installing $(BINARY_NAME)..."
	cp $(BINARY_NAME) $(GOPATH)/bin/
	@echo "Installed to $(GOPATH)/bin/$(BINARY_NAME)"
