.PHONY: build test docker-test
build:
	go build -o dwbc ./cmd/dwbc
test:
	go test -race ./...
	go vet ./...
docker-test:
	docker build -f tests/Dockerfile -t dwbc-test .
	docker run --rm --cap-add SYS_ADMIN --device /dev/fuse dwbc-test
