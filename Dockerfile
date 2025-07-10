# Build the manager binary
FROM registry.access.redhat.com/ubi8/go-toolset:1.23.9-1751469143 as builder

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY pkg/ pkg/
COPY vendor/ vendor/
COPY go.mod go.mod
COPY go.sum go.sum

# Build
RUN GOOS=linux GOARCH=amd64 go build -tags strictfipsruntime -a -o /usr/bin/manager main.go

FROM registry.redhat.io/rhel8-6-els/rhel:latest
WORKDIR /
COPY --from=builder /usr/bin/manager .

USER 65532:65532

ENTRYPOINT ["/manager"]
