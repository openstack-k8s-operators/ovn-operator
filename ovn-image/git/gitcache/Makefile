%:
	@if [ -d $@.git ]; then \
		cd $@.git; git fetch; \
	else \
		git clone --bare https://github.com/openvswitch/$@.git; \
	fi
