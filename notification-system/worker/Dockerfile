# Built from the directory above, so that the commons module is in reach:
#   docker build -f worker/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY commons ./commons
COPY worker/go.mod worker/go.sum ./worker/
WORKDIR /src/worker
RUN go mod download
COPY worker/cmd ./cmd
COPY worker/internal ./internal
RUN CGO_ENABLED=0 go build -o /out/ ./cmd/worker ./cmd/mockprovider

FROM gcr.io/distroless/static-debian12:nonroot
# One image for both binaries; the mock provider is run by overriding the entrypoint.
COPY --from=build /out/worker /out/mockprovider /
EXPOSE 9090
ENTRYPOINT ["/worker"]
