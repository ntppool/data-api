generate: sqlc
	go generate ./...

sqlc:
	go tool sqlc compile
	go tool sqlc generate
	go tool gowrap gen -g -t opentelemetry -i QuerierTx -p ./ntpdb -o ./ntpdb/otel.go

sign:
	drone sign --save ntppool/data-api
