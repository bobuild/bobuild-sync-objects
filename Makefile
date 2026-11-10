default: build

build:
	go build -o bobuild-sync-objects ./cmd/sync

clean:
	rm -f bobuild-sync-objects
