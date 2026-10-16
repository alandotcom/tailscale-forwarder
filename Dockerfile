FROM golang:1.27.2-alpine@sha256:f92b6ef800e499660581efdabdf25d9d817a9d124eaf900924f0504e7e27e12d AS builder

WORKDIR /app

COPY go.mod go.sum ./

ENV GOTOOLCHAIN=local
ENV CGO_ENABLED=0

RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . ./

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -mod=readonly -trimpath -o tailscale_fwdr -ldflags="-w -s" . \
    && sh scripts/third-party-notices.sh > THIRD_PARTY_NOTICES.txt

FROM gcr.io/distroless/static-debian13:latest@sha256:58133991db06659feaabe0f4e97a35cebf15ef4ea08f8a4c6d2ee5f75e4aa6a0

WORKDIR /app

COPY --from=builder /app/tailscale_fwdr /usr/local/bin/tailscale_fwdr
COPY --from=builder /app/THIRD_PARTY_NOTICES.txt /usr/share/doc/tailscale-forwarder/THIRD_PARTY_NOTICES.txt
COPY LICENSE README.md /usr/share/doc/tailscale-forwarder/

USER 0

ENTRYPOINT ["/usr/local/bin/tailscale_fwdr"]
