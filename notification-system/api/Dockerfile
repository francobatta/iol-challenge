# Built from the directory above, so that the commons module is in reach:
#   docker build -f api/Dockerfile .
FROM golang:1.26 AS build
WORKDIR /src
COPY commons ./commons
COPY api/go.mod api/go.sum ./api/
WORKDIR /src/api
RUN go mod download
COPY api/cmd ./cmd
COPY api/internal ./internal
RUN CGO_ENABLED=0 go build -o /out/ ./cmd/server

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/server /
EXPOSE 8080 9090
ENTRYPOINT ["/server"]
