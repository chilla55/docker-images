VERSION ?= $(shell cat VERSION)
REGISTRY ?= ghcr.io/chilla55
IMAGE_NAME = vaultwarden
FULL_IMAGE = $(REGISTRY)/$(IMAGE_NAME):$(VERSION)

.PHONY: build push test clean

build:
	docker build -t $(IMAGE_NAME):$(VERSION) .
	docker tag $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest

push:
	docker tag $(IMAGE_NAME):$(VERSION) $(FULL_IMAGE)
	docker tag $(IMAGE_NAME):$(VERSION) $(REGISTRY)/$(IMAGE_NAME):latest
	docker push $(FULL_IMAGE)
	docker push $(REGISTRY)/$(IMAGE_NAME):latest

test:
	docker run -d --rm --name vaultwarden-test \
		-e DOMAIN=https://test.example.com \
		-e ADMIN_TOKEN=testtoken123 \
		$(IMAGE_NAME):$(VERSION)
	sleep 5
	docker logs vaultwarden-test
	docker stop vaultwarden-test

clean:
	docker rmi $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest || true
	docker rmi $(FULL_IMAGE) $(REGISTRY)/$(IMAGE_NAME):latest || true
