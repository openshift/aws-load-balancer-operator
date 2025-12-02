# Build the manager binary
FROM registry.access.redhat.com/ubi9/go-toolset:1.24 as builder

WORKDIR /opt/app-root/src

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY pkg/ pkg/
COPY vendor/ vendor/
COPY go.mod go.mod
COPY go.sum go.sum

# Build
RUN GOOS=linux GOARCH=amd64 go build -tags strictfipsruntime -a -o manager main.go

WORKDIR /
FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
COPY --from=builder /opt/app-root/src/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
