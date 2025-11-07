# build the binary
FROM golang:1.25 AS go-build
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . /app
RUN CGO_ENABLED=0 GOOS=linux go build -o /api

# ensure tests pass
FROM go-build AS test
RUN go test -v ./...

# create the release artifact
FROM gcr.io/distroless/static-debian12 AS release
WORKDIR /
COPY --from=go-build /api /app/api
EXPOSE 3000
USER nonroot:nonroot
ENTRYPOINT ["/app/api"]
