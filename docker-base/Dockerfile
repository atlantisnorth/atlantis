# This Dockerfile builds our base image with gosu, dumb-init and the atlantis
# user. We split this from the main Dockerfile because this base doesn't change
# and also because it kept breaking the build due to flakiness.
FROM alpine:3.13
LABEL authors="Anubhav Mishra, Luke Kysow"

# We use gosu to step down from root and run as the atlantis user so we need
# to create that user and group.
# We add the atlantis user to the root group and make its home directory
# owned by root so that OpenShift users can use /home/atlantis as their
# data dir because OpenShift runs containers as a random uid that's part of
# the root group.
RUN addgroup atlantis && \
    adduser -S -G atlantis atlantis && \
    adduser atlantis root && \
    chown atlantis:root /home/atlantis/ && \
    chmod g=u /home/atlantis/ && \
    chmod g=u /etc/passwd

# Install dumb-init and gosu.
ENV DUMB_INIT_VERSION=1.2.5
ENV GOSU_VERSION=1.12
RUN apk add --no-cache ca-certificates gnupg curl git git-lfs unzip bash openssh libcap openssl && \
    curl -L -s --output /bin/dumb-init "https://github.com/Yelp/dumb-init/releases/download/v${DUMB_INIT_VERSION}/dumb-init_${DUMB_INIT_VERSION}_$(arch)" && \
    chmod +x /bin/dumb-init && \
    mkdir -p /tmp/build && \
    cd /tmp/build && \
    ARCH="amd64" && \
    case "$(arch)" in \
    "aarch64") \
        ARCH="arm64" \
        ;; \
    "armv7l") \
        ARCH="arm" \
        ;; \
    esac && \
    curl -L -s --output gosu "https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-${ARCH}" && \
    curl -L -s --output gosu.asc "https://github.com/tianon/gosu/releases/download/${GOSU_VERSION}/gosu-${ARCH}.asc" && \
    for server in $(shuf -e ipv4.pool.sks-keyservers.net \
                            hkp://p80.pool.sks-keyservers.net:80 \
                            keyserver.ubuntu.com \
                            hkp://keyserver.ubuntu.com:80 \
                            pgp.mit.edu) ; do \
        gpg --keyserver "$server" --recv-keys B42F6819007F00F88E364FD4036A9C25BF357DD4 && break || : ; \
    done && \
    gpg --batch --verify gosu.asc gosu && \
    chmod +x gosu && \
    cp gosu /bin && \
        cd /tmp && \
        rm -rf /tmp/build && \
        apk del gnupg openssl && \
        rm -rf /root/.gnupg && rm -rf /var/cache/apk/*

# Set up nsswitch.conf for Go's "netgo" implementation
# - https://github.com/golang/go/blob/go1.9.1/src/net/conf.go#L194-L275
RUN [ ! -e /etc/nsswitch.conf ] && echo 'hosts: files dns' > /etc/nsswitch.conf
