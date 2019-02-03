LDFLAGS := "-extldflags '-static'"

.PHONY: all
all: build

.PHONY: build
build:
	CGO_ENABLED=0 GOOS=linux go build \
		-ldflags $(LDFLAGS) \
		-o bin/wg-cni \
		github.com/schu/wireguard-cni/cmd/wg-cni
