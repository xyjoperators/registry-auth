# Build the manager binary
FROM golang:1.23 AS builder
WORKDIR /workspace
ARG TARGETOS
ARG TARGETARCH

COPY . .


# Build
# the GOARCH has not a default value to allow the binary be built according to the host where the command
# was called. For example, if we call make docker-build in a local env which has the Apple Silicon M1 SO
# the docker BUILDPLATFORM arg will be linux/arm64 when for Apple x86 it will be linux/amd64. Therefore,
# by leaving it empty we can ensure that the container and binary shipped on it will have the same platform.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -ldflags="-s -w"  -mod=vendor -a -o manager cmd/main.go

# Use distroless as minimal base image to package the manager binary
# Refer to https://github.com/GoogleContainerTools/distroless for more details
FROM  alpine:3.18
WORKDIR /
COPY --from=builder /workspace/manager .

ENTRYPOINT ["/manager"]
