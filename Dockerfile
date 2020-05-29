# Build binaries
FROM golang:1.14-alpine AS build
ARG version=dev
WORKDIR /src
ADD go.mod /src
ADD go.sum /src
RUN go mod download
ADD cmd /src/cmd
ADD internal /src/internal
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o telepathy -ldflags "-X 'gitlab.com/kavenc/telepathy/internal/pkg/info.version=${version}'" ./cmd/telepathy

# Start application
FROM alpine
WORKDIR /telepathy
COPY --from=build /src/telepathy .
ENTRYPOINT [ "./telepathy" ]
