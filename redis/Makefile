VERSION ?= $(shell cat VERSION)
REGISTRY ?= ghcr.io/chilla55
IMAGE_NAME = redis
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
	docker run -d --rm --name redis-test -e REDIS_PASSWORD=testpass $(IMAGE_NAME):$(VERSION)
	sleep 3
	docker exec redis-test redis-cli -a testpass ping
	docker stop redis-test

clean:
	docker rmi $(IMAGE_NAME):$(VERSION) $(IMAGE_NAME):latest || true
	docker rmi $(FULL_IMAGE) $(REGISTRY)/$(IMAGE_NAME):latest || true
