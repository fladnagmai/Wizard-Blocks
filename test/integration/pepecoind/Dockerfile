# Pepecoin Core for the regtests, from the official release binaries.
# The tarball is downloaded by the harness and must match the SHA-256 in
# the release's SHA256SUMS.asc, signed by David Eichel (key
# 18250EC92E527E9723E49116FD415169D2691927, in the Pepecoin repository's
# contrib/gitian-keys); it is checked again here.
ARG BASE=mirror.gcr.io/library/debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
FROM ${BASE}
ARG PEP_VERSION=1.1.0
ARG PEP_SHA256=9d7ef948e5726c9941cbc5307b4a0b725edc715bc10ed5515154485faecd710b
COPY pepecoin-${PEP_VERSION}-x86_64-linux-gnu.tar.gz /tmp/pep.tar.gz
RUN echo "${PEP_SHA256}  /tmp/pep.tar.gz" | sha256sum -c - \
 && tar -xzf /tmp/pep.tar.gz -C /tmp \
 && install -m 0755 /tmp/pepecoin-${PEP_VERSION}/bin/pepecoind /tmp/pepecoin-${PEP_VERSION}/bin/pepecoin-cli /usr/local/bin/ \
 && rm -rf /tmp/pep.tar.gz /tmp/pepecoin-${PEP_VERSION}
ENTRYPOINT ["pepecoind"]
