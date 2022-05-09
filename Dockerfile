FROM --platform=${BUILDPLATFORM} docker.shiyou.kingsoft.com/library/golang:1.18.1 AS base

WORKDIR /app

# Convert go test output to junit xml
RUN go install github.com/jstemmer/go-junit-report@v1.0.0

COPY go.mod go.mod
COPY go.sum go.sum

RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download -x

COPY  . .

FROM base AS build
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o snowflake-service .

FROM base AS unit-test
ARG REPORT_OUTPUT=reports
RUN --mount=type=cache,target=/go/pkg/mod \
    mkdir -p ${REPORT_OUTPUT} \
    && CGO_ENABLED=1 go test ./... -race -v -coverprofile=${REPORT_OUTPUT}/coverage.out | go-junit-report > ${REPORT_OUTPUT}/test.xml \
    && CGO_ENABLED=1 go test ./... -race -v -json > ${REPORT_OUTPUT}/test.json \
    || true

FROM scratch AS reports
COPY --from=unit-test /app/reports .

FROM docker.shiyou.kingsoft.com/library/alpine:3.14.0 AS release
COPY --from=build /app/snowflake-service .
ENTRYPOINT  ["./snowflake-service"]