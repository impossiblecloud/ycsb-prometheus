#
# Pingcap tidb benchmark tools
#

FROM golang:1.24.3 AS build
WORKDIR /build
ENV GOPATH=/go
ENV PATH="$PATH:$GOPATH/bin"
COPY . ./
RUN make build

# FROM gcr.io/distroless/base-debian11
FROM alpine:3.21
WORKDIR /
COPY --from=build /build/output/tidb-ycsb /tidb-ycsb
ENTRYPOINT ["/tidb-ycsb"]
