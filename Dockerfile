# Build on the runner's own architecture and cross-compile for the target one (no QEMU emulation):
# the GitHub runner is amd64, kind on an Apple Silicon Mac is arm64.
FROM --platform=$BUILDPLATFORM golang:1.27 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ ./
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /wiki .

# distroless: no shell and no package manager — smaller image, fewer CVEs; runs as uid 65532.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /wiki /wiki
COPY content/ /content/
EXPOSE 8080
ENTRYPOINT ["/wiki"]
