# syntax=docker/dockerfile:1
# Pepecoin Core v1.1.0 for Umbrel, from the official release binaries.
# The build refuses to continue unless SHA256SUMS.asc carries a good
# signature from the pinned key (david2278.asc, David Eichel, fingerprint
# 18250EC92E527E9723E49116FD415169D2691927, from the Pepecoin repository's
# contrib/gitian-keys; the key has expired since, the signature was made
# with it on 2024-12-16, while valid: gpg reports VALIDSIG either way)
# and the tarball for this platform matches the signed checksum.
# Build context: nodes/ (the key and memwrap.sh).
# Runtime base, pinned by digest (docker.io and mirror.gcr.io serve the same).
ARG BASE=debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
ARG VERSION=1.1.0

# Verify stage (runs on the build machine; it only picks the tarball for
# TARGETARCH): a Debian image that already has gpg and sha256sum
# (golang:1.24.7-bookworm, pinned by digest).
FROM --platform=$BUILDPLATFORM golang:1.24.7-bookworm@sha256:b8bae5bd9ba9b1f89b635c91c24cc75cea335a16fb5076310f38566fc674b1ec AS verify
ARG VERSION
ARG TARGETARCH
ADD --checksum=sha256:88d7b70c62a16ed22d754cdc2d4851701900e790cbd8590c532695ae240050e3 https://github.com/pepecoinppc/pepecoin/releases/download/v${VERSION}/SHA256SUMS.asc /rel/SHA256SUMS.asc
ADD https://github.com/pepecoinppc/pepecoin/releases/download/v${VERSION}/pepecoin-${VERSION}-x86_64-linux-gnu.tar.gz /rel/
ADD https://github.com/pepecoinppc/pepecoin/releases/download/v${VERSION}/pepecoin-${VERSION}-aarch64-linux-gnu.tar.gz /rel/
COPY pepecoin/david2278.asc /rel/key.asc
RUN set -eu; cd /rel; \
    export GNUPGHOME=/rel/gnupg; mkdir -m 700 $GNUPGHOME; \
    gpg --batch --import key.asc; \
    gpg --batch --status-fd 1 --verify SHA256SUMS.asc 2>/dev/null | grep -q "^\[GNUPG:\] VALIDSIG 18250EC92E527E9723E49116FD415169D2691927 "; \
    case "$TARGETARCH" in amd64) t=x86_64-linux-gnu ;; arm64) t=aarch64-linux-gnu ;; *) echo "unsupported $TARGETARCH"; exit 1 ;; esac; \
    f=pepecoin-${VERSION}-$t.tar.gz; \
    gpg --batch --decrypt SHA256SUMS.asc 2>/dev/null | grep " $f$" | sha256sum -c -; \
    tar -xzf "$f"; mkdir /out; \
    install -m 0755 pepecoin-${VERSION}/bin/pepecoind pepecoin-${VERSION}/bin/pepecoin-cli /out/

FROM ${BASE}
COPY --from=verify /out/ /usr/local/bin/
COPY --chmod=0755 memwrap.sh /usr/local/bin/memwrap
RUN useradd --uid 1000 --create-home --home-dir /home/node node && mkdir /data && chown 1000:1000 /data
USER 1000:1000
VOLUME /data
# memwrap reports the node's memory to the app page (MEM_FILE).
ENTRYPOINT ["memwrap", "pepecoind"]
