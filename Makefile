.PHONY: build run test

build:
	docker build -t apex ./web

run:
	docker run --rm -p 3000:3000 \
		-v "$(PWD)/data:/data" \
		apex \
		--addr 0.0.0.0:3000 \
		--token-file /data/.iracing_token \
		--data-dir /data

test:
	cd web && go test ./...
