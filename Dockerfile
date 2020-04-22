FROM alpine:3.11

RUN apk add --no-cache bash

COPY bin/wg-cni /opt/cni/bin/wg-cni
COPY scripts/install /install

ENTRYPOINT ["/install"]
