FROM golang:1.20.3-alpine3.17 as builder

WORKDIR /workspace

# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY internal/ internal/
COPY log/ log/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GO111MODULE=on go build -a -o log-socket cmd/service/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM gcr.io/distroless/static:latest

WORKDIR /

COPY --from=builder /workspace/log-socket .

ENTRYPOINT ["/log-socket"]
