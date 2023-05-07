generate: sqlc
	go generate ./...

sqlc:
	sqlc compile
	sqlc generate

sign:
	drone sign --save ntppool/data-api
