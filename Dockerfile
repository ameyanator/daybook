# Go server image for Render (or any container host). Build from the repo root.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY server/go.mod server/go.sum ./
RUN go mod download
COPY server/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /daybook .

FROM alpine:3.22
# ca-certificates: fetching Google/Apple signing keys and talking to Neon over TLS.
RUN apk add --no-cache ca-certificates && adduser -D -H daybook
COPY --from=build /daybook /usr/local/bin/daybook
USER daybook
CMD ["daybook"]
