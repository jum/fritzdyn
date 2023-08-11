# syntax = docker/dockerfile:1-experimental
# Dockerfile for building the project with static assets
FROM golang:1.21-bullseye as build

WORKDIR /goapp
ADD . /goapp

RUN go mod download

RUN --mount=type=cache,target=/root/.cache/go-build  GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o app -tags server

# Now copy it into our base image.
FROM gcr.io/distroless/base-debian11
EXPOSE 8090
WORKDIR /goapp
COPY --from=build /goapp/app fritzdyn
CMD ["/goapp/fritzdyn"]
