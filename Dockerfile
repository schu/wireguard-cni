FROM alpine

RUN apk add --no-cache bash

COPY bin/wg-cni /opt/cni/bin/wg-cni
COPY scripts/install /install

ENTRYPOINT ["/install"]
