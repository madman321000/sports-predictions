# sports-predictions
## Tests

Run unit tests and static checks:

```sh
go test ./...
go vet ./...
```

PostgreSQL integration tests skip when `TEST_DATABASE_URL` is unset. To run
all tests, including the real SQL upsert checks:

```sh
export TEST_DATABASE_URL='postgres://sports:sports@localhost:5432/sportsdb?sslmode=disable'
go test -race -count=1 -v ./...
```

Start the development database with `docker compose up -d db` if needed.
Use a local development or dedicated test database. The test user needs permission
to create schemas. Each integration test creates a uniquely named schema, applies
`migrations/000001_create_leagues.up.sql` there, and drops that schema on cleanup,
even when assertions fail. Existing application tables are not changed, and no
manual migration is required for these tests. A configured but unreachable database
fails the tests rather than skipping them.

Coverage includes seed records and context forwarding, wrapped write failures and
stopping on error, repeated seeding without duplicates, updates preserving row
identity and creation time, timestamp refresh, and wrapped PostgreSQL errors.
