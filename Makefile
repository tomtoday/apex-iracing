.PHONY: build run run-docker test

run:
	cd web && go run ./cmd/apex/ --data-dir ../data --token-file ../data/.iracing_token

build:
	docker build -t apex ./web

run-docker:
	docker run --rm -p 3000:3000 \
		-v "$(PWD)/data:/data" \
		apex \
		--addr 0.0.0.0:3000 \
		--token-file /data/.iracing_token \
		--data-dir /data

test:
	cd web && go test ./...
