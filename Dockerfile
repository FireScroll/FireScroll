FROM golang:1.20.3 as build

WORKDIR /app

COPY go.* /app/

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=ssh \
    go mod download

COPY . .

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build $GO_ARGS -o /app/outbin

FROM ubuntu

ARG CRDB_VERSION="v22.2.8"
ARG ARCH="amd64"

# install CRDB
RUN apt update
RUN apt install tar curl xz-utils -y
ADD https://binaries.cockroachdb.com/cockroach-$CRDB_VERSION.linux-$ARCH.tgz cockroach-$CRDB_VERSION.linux-$ARCH.tgz
RUN tar -xvf cockroach-$CRDB_VERSION.linux-$ARCH.tgz
RUN cp cockroach-$CRDB_VERSION.linux-$ARCH/cockroach /usr/local/bin/cockroach

# handle CRDB depenencies
RUN mkdir -p /usr/local/lib/cockroach
RUN cp -i cockroach-$CRDB_VERSION.linux-$ARCH/lib/libgeos.so /usr/local/lib/cockroach/
RUN cp -i cockroach-$CRDB_VERSION.linux-$ARCH/lib/libgeos_c.so /usr/local/lib/cockroach/

# install redpanda
RUN curl -1sLf 'https://dl.redpanda.com/nzc4ZYQK3WRGd9sy/redpanda/cfg/setup/bash.deb.sh' | bash && apt install redpanda -y
# install redpanda console
RUN curl -1sLf 'https://dl.redpanda.com/nzc4ZYQK3WRGd9sy/redpanda/cfg/setup/bash.deb.sh' | bash && apt-get install redpanda-console -y

ARG S6_OVERLAY_VERSION=3.1.4.1
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-noarch.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-noarch.tar.xz
ADD https://github.com/just-containers/s6-overlay/releases/download/v${S6_OVERLAY_VERSION}/s6-overlay-x86_64.tar.xz /tmp
RUN tar -C / -Jxpf /tmp/s6-overlay-x86_64.tar.xz

COPY services.d /etc/services.d
RUN chmod +x /etc/services.d/cockroach/run
RUN chmod +x /etc/services.d/fanout/run
RUN chmod +x /etc/services.d/redpanda/run

COPY --from=build /app/outbin /app/
ENTRYPOINT ["/init"]
