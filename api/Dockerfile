# syntax=docker/dockerfile:1.7
FROM golang:1.23-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./

ARG VERSION=0.0.0-dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

RUN CGO_ENABLED=0 GOOS=linux go build \
  -trimpath \
  -ldflags="-s -w \
    -X main.version=${VERSION} \
    -X main.commit=${COMMIT} \
    -X main.buildDate=${BUILD_DATE}" \
  -o /out/stigman \
  ./cmd/stigman

FROM gcr.io/distroless/static-debian12:nonroot AS run
WORKDIR /app
COPY --from=build /out/stigman /app/stigman
USER nonroot:nonroot
EXPOSE 54001
ENTRYPOINT ["/app/stigman"]
