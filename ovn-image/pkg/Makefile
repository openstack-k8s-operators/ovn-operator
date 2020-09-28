TAG=2.11-dev

default: build

.PHONY: scripts
scripts:
	mkdir -p scripts
	cp ../scripts-common/* scripts/

.PHONY: build
build: scripts
	buildah bud --layers -t ovn-central:$(TAG)

.PHONY: push
push:
	buildah push ovn-central:$(TAG) quay.io/mbooth/ovn-central:$(TAG)
