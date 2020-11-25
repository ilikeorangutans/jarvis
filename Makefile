SHA=$(shell git rev-parse HEAD)
NOW=$(shell date +%FT%T%z)
DIST_LD_FLAGS="-X github.com/ilikeorangutans/remind-me-bot/pkg/version.SHA=$(SHA) -X github.com/ilikeorangutans/remind-me-bot/pkg/version.BuildTime=$(NOW)"

SOURCES=$(shell find ./ -type f -iname '*.go')

.PHONY: run
run: target/linux-amd64/bot
	./target/linux-amd64/bot


.PHONY: dist-all
dist-all: target/linux-arm/bot target/linux-amd64/bot

target/%/:
	mkdir -p $(@)

target/linux-arm/bot: target/linux-arm/ $(SOURCES)
	GOOS=linux GOARCH=arm go build -ldflags $(DIST_LD_FLAGS) -o target/linux-arm/bot ./cmd/bot/main.go

target/linux-amd64/bot: target/linux-amd64/ $(SOURCES)
	GOOS=linux GOARCH=amd64 go build -ldflags $(DIST_LD_FLAGS) -o target/linux-amd64/bot ./cmd/bot/main.go

.PHONY: clean
clean:
	-rm -rf target

.PHONY: docker
docker: target/linux-arm/bot
	docker buildx build -f Dockerfile . -t registry.ilikeorangutans.me/apps/reminder-bot:$(SHA) -t registry.ilikeorangutans.me/apps/reminder-bot:latest --platform  linux/arm/v7 --load
	docker push registry.ilikeorangutans.me/apps/reminder-bot:$(SHA)
	docker push registry.ilikeorangutans.me/apps/reminder-bot:latest
