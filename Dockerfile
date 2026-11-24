# Images are built locally with `docker compose build` (design section 3).
FROM golang:1.27.4-alpine@sha256:70bd7ddea8b6cbe5d6943cde279d6b67feb10cc924fb9100d1d8757c1575b280 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY schemas ./schemas
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/nightshift ./cmd/nightshift

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab AS nightshift
COPY --from=build /out/nightshift /usr/local/bin/nightshift
USER 10001
ENTRYPOINT ["/usr/local/bin/nightshift"]
CMD ["serve"]
