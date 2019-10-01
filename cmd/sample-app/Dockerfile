# Build sample-app
FROM golang:alpine as builder

RUN apk add --no-cache gcc git make bash

ARG DIR=/go/src/build
WORKDIR $DIR
ADD . $DIR

ENV GO111MODULE=on GOFLAGS=-mod=vendor
RUN make BUILD_DIRS=sample-app

# Create image without the build deps.
FROM alpine

RUN apk add --no-cache libc6-compat bash

ARG DIR=/go/src/build
COPY --from=builder $DIR/bin/sample-app /bin/sample-app
