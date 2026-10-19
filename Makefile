generate: sqlc
	go generate ./...

sqlc:
	go tool sqlc compile
	go tool sqlc generate
	go generate ./ntpdb

sign:
	drone sign --save ntppool/data-api
