# The runatlantis/atlantis-base is created by docker-base/Dockerfile.
FROM runatlantis/atlantis-base:v3.5
LABEL authors="Anubhav Mishra, Luke Kysow"

# This gets injected by Docker buildx
ARG TARGETARCH

# install terraform binaries
ENV DEFAULT_TERRAFORM_VERSION=1.0.0

# In the official Atlantis image we only have the latest of each Terraform version.
RUN AVAILABLE_TERRAFORM_VERSIONS="0.8.8 0.9.11 0.10.8 0.11.15 0.12.31 0.13.7 0.14.11 0.15.5 ${DEFAULT_TERRAFORM_VERSION}" && \
    for VERSION in ${AVAILABLE_TERRAFORM_VERSIONS}; do \
        curl -LOs https://releases.hashicorp.com/terraform/${VERSION}/terraform_${VERSION}_linux_${TARGETARCH}.zip && \
        curl -LOs https://releases.hashicorp.com/terraform/${VERSION}/terraform_${VERSION}_SHA256SUMS && \
        sed -n "/terraform_${VERSION}_linux_${TARGETARCH}.zip/p" terraform_${VERSION}_SHA256SUMS | sha256sum -c && \
        mkdir -p /usr/local/bin/tf/versions/${VERSION} && \
        unzip terraform_${VERSION}_linux_${TARGETARCH}.zip -d /usr/local/bin/tf/versions/${VERSION} && \
        ln -s /usr/local/bin/tf/versions/${VERSION}/terraform /usr/local/bin/terraform${VERSION} && \
        rm terraform_${VERSION}_linux_${TARGETARCH}.zip && \
        rm terraform_${VERSION}_SHA256SUMS; \
    done && \
    ln -s /usr/local/bin/tf/versions/${DEFAULT_TERRAFORM_VERSION}/terraform /usr/local/bin/terraform

ENV DEFAULT_CONFTEST_VERSION=0.25.0

RUN AVAILABLE_CONFTEST_VERSIONS="${DEFAULT_CONFTEST_VERSION}" && \
    for VERSION in ${AVAILABLE_CONFTEST_VERSIONS}; do \
        ARCH="x86_64" && \
        case "$(arch)" in \
        "aarch64") \
            ARCH="arm64" \
            ;; \
        esac && \
        curl -LOs https://github.com/open-policy-agent/conftest/releases/download/v${VERSION}/conftest_${VERSION}_Linux_${ARCH}.tar.gz && \
        curl -LOs https://github.com/open-policy-agent/conftest/releases/download/v${VERSION}/checksums.txt && \
        sed -n "/conftest_${VERSION}_Linux_${ARCH}.tar.gz/p" checksums.txt | sha256sum -c && \
        mkdir -p /usr/local/bin/cft/versions/${VERSION} && \
        tar -C  /usr/local/bin/cft/versions/${VERSION} -xzf conftest_${VERSION}_Linux_${ARCH}.tar.gz && \
        ln -s /usr/local/bin/cft/versions/${VERSION}/conftest /usr/local/bin/conftest${VERSION} && \
        rm conftest_${VERSION}_Linux_${ARCH}.tar.gz && \
        rm checksums.txt; \
    done

RUN ln -s /usr/local/bin/cft/versions/${DEFAULT_CONFTEST_VERSION}/conftest /usr/local/bin/conftest

# copy binary
COPY ./atlantis_${TARGETARCH} /usr/local/bin/atlantis

# copy docker entrypoint
COPY docker-entrypoint.sh /usr/local/bin/docker-entrypoint.sh

ENTRYPOINT ["docker-entrypoint.sh"]
CMD ["server"]
