TAG=2.11-dev

.imagebuild.$(TAG):
	buildah bud -t ovn-central:$(TAG) && touch $@

.PHONY: build
build: .imagebuild.$(TAG)

.PHONY: push
push:
	buildah push ovn-central:$(TAG) quay.io/mbooth/ovn-central:$(TAG)
