# Perform a build
FROM golang:alpine AS build
RUN mkdir /build
ADD . /build
WORKDIR /build
RUN apk update && apk add --no-cache git gcc build-base linux-headers

ARG VERSION=dev
ENV VERSION=${VERSION}
ARG GIT_COMMIT
ENV GIT_COMMIT=${GIT_COMMIT}
ARG NAME=docker
ENV NAME=${NAME}

RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -ldflags "-extldflags \"-static\" -s -w -X github.com/johanvandegriff/hugowncast/config.GitCommit=$GIT_COMMIT -X github.com/johanvandegriff/hugowncast/config.VersionNumber=$VERSION -X github.com/johanvandegriff/hugowncast/config.BuildPlatform=$NAME" -o owncast .

# Create the image by copying the result of the build into a new alpine image
FROM alpine
RUN apk update && apk add --no-cache ffmpeg ffmpeg-libs ca-certificates && update-ca-certificates

# Copy owncast assets
WORKDIR /app
COPY --from=build /build/owncast /app/owncast
COPY --from=build /build/webroot /app/webroot
COPY --from=build /build/hugo-template /app/hugo-template
RUN mkdir /app/data
RUN mkdir /app/hugo
ENTRYPOINT ["/app/owncast"]
EXPOSE 8080 1935
