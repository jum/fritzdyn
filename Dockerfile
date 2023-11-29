# Dockerfile for building the project with static assets
FROM --platform=$BUILDPLATFORM golang:1.21-bullseye as build
WORKDIR /goapp
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg go mod download
COPY . ./
ARG TARGETOS TARGETARCH
ENV CGO_ENABLED=0
RUN --mount=type=cache,target=/root/.cache/go-build --mount=type=cache,target=/go/pkg GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o app -tags server
FROM gcr.io/distroless/base-debian11
EXPOSE 8090
WORKDIR /goapp
COPY --from=build /goapp/app fritzdyn
CMD ["/goapp/fritzdyn"]
