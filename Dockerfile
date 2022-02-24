# Build the manager binary
FROM golang:1.17 as builder
# TODO: Switch to UBI golang image when 1.17 is released
# FROM registry.access.redhat.com/ubi8/go-toolset:latest

WORKDIR /workspace

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY pkg/ pkg/
COPY vendor/ vendor/
COPY go.mod go.mod
COPY go.sum go.sum

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -a -o manager main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM registry.access.redhat.com/ubi8-minimal:latest
WORKDIR /
COPY --from=builder /workspace/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
