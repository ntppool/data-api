generate: sqlc
	go generate ./...

sqlc:
	go tool sqlc compile
	go tool sqlc generate
	go tool gowrap gen -t opentelemetry -i QuerierTx -p ./ntpdb -o ./ntpdb/otel.go
	#go tool mockery --dir ntpdb --name QuerierTx --config /dev/null

sign:
	drone sign --save ntppool/data-api
