OVSREPO=https://github.com/openvswitch/ovs.git
OVSBRANCH=master
OVNREPO=https://github.com/openvswitch/ovn.git
OVNBRANCH=master

default: build

.PHONY: gitcache-update
gitcache-update:
	$(MAKE) -C gitcache ovs ovn

.PHONY: build
build:
	buildah bud --layers --tag ovn-central:trunk \
		-v $$(pwd)/gitcache:/gitcache:ro \
		--build-arg OVSREPO=$(OVSREPO) --build-arg OVSBRANCH=$(OVSBRANCH) \
		--build-arg OVNREPO=$(OVNREPO) --build-arg OVNBRANCH=$(OVNBRANCH)

.PHONY: push
push:
	buildah push ovn-central:trunk quay.io/mbooth/ovn-central:trunk
