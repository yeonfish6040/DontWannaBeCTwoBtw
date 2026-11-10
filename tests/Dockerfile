FROM golang:1.23-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends fuse3 && apt-get clean
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
CMD ["sh", "-c", "go test -race ./... && go vet ./... && tests/integration.sh"]
