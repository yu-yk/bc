.PHONY: build
build:
	go build

.PHONY: build-win64
build-win64:
	CGO_ENABLED=1 CC=x86_64-w64-mingw32-gcc CXX=x86_64-w64-mingw32-g++ GOOS=windows GOARCH=amd64 go build -ldflags -H=windowsgui -o bgui.exe