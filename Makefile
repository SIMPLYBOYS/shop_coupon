postgres:
	docker run --name postgresql -e POSTGRES_USER=root -e POSTGRES_PASSWORD=mysecretpassword -p 5433:5432 -d postgres:9.5.10
	
createdb:
	docker exec -it postgresql createdb --username=root --owner=root shopcoupon

dropdb:
	docker exec -it postgresql dropdb shopcoupon

migrationup:
	migrate -path db/migration -database "postgresql://root:mysecretpassword@localhost:5433/shopcoupon?sslmode=disable" -verbose up

migrationdown:
	migrate -path db/migration -database "postgresql://root:mysecretpassword@localhost:5433/shopcoupon?sslmode=disable" -verbose down

sqlc:
	sqlc generate

server:
	go run main.go

.PHONY: createdb dropdb postgres migrationup migrationdown