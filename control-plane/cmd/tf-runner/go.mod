module github.com/libops/control-plane/cmd/tf-runner

go 1.25.5

require (
	github.com/go-sql-driver/mysql v1.9.3
	github.com/google/uuid v1.6.0
	github.com/libops/api/db v0.0.0-00010101000000-000000000000
)

require filippo.io/edwards25519 v1.1.0 // indirect

replace github.com/libops/api/db => ../../../db
