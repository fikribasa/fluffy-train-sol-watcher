BINARY=solscan-watcher
BUILD_DIR=.
 
.PHONY: build clean tidy
 
build: tidy
	CGO_ENABLED=0 go build -o $(BUILD_DIR)/$(BINARY) ./main.go
 
tidy:
	go mod tidy
 
clean:
	rm -f $(BUILD_DIR)/$(BINARY)
 