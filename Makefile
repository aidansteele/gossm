all:
	go test ./...
	gox -arch="amd64" -os="windows linux darwin" ./...
