generate: sqlc
	go generate ./...

sqlc:
	@which gowrap  >& /dev/null || (echo "Run 'go install github.com/hexdigest/gowrap/cmd/gowrap@v1.3.2'" && exit 1)
	@which mockery >& /dev/null || (echo "Run 'go install github.com/vektra/mockery/v2@v2.35.4'" && exit 1)
	sqlc compile
	sqlc generate
	gowrap gen -t opentelemetry -i QuerierTx -p ./ntpdb -o ./ntpdb/otel.go
	mockery --dir ntpdb --name QuerierTx --config /dev/null

sign:
	drone sign --save ntppool/data-api
