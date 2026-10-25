# syntax=docker/dockerfile:1
FROM golang:1.27.1-bookworm AS build
WORKDIR /workspace/app
COPY app/go.mod app/go.sum ./
RUN go mod download
COPY app/ ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/setu ./cmd/setu

FROM build AS debug
RUN go install github.com/go-delve/delve/cmd/dlv@v1.27.0
RUN go build -gcflags='all=-N -l' -o /out/setu-debug ./cmd/setu
EXPOSE 3000 2345
ENTRYPOINT ["/go/bin/dlv", "exec", "/out/setu-debug", "--listen=:2345", "--headless=true", "--api-version=2", "--accept-multiclient", "--continue", "--"]
CMD ["api"]

FROM debian:bookworm-slim AS runtime
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/setu /usr/local/bin/setu
COPY frontend /srv/frontend
ENV FRONTEND_DIR=/srv/frontend
# Fargate copies mount-point permissions only for paths declared as image volumes.
RUN chmod 1777 /tmp
VOLUME ["/tmp"]
USER 65532:65532
EXPOSE 3000
ENTRYPOINT ["/usr/local/bin/setu"]
CMD ["api"]
