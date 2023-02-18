all: bin/namazu bin/namazu2nostr bin/namazu2mastodon

bin/namazu: cmd/namazu/main.go eew/*.go
	go build -o bin/namazu cmd/namazu/main.go

bin/namazu2nostr: cmd/namazu2nostr/main.go nostr/*.go eew/*.go
	go build -o bin/namazu2nostr cmd/namazu2nostr/main.go

bin/namazu2mastodon: cmd/namazu2mastodon/main.go mastodon/*.go eew/*.go
	go build -o bin/namazu2mastodon cmd/namazu2mastodon/main.go
