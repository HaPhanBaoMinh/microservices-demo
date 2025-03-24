# paymentservice

## Description

- This is a simple payment service that allows users to make payments and view their payment history.

## Prepare DB

- Create a table in RDS

```sql
psql -h localhost -p 5432 -U postgres -d final
```

```sql
CREATE TABLE tracking_info (
    id SERIAL PRIMARY KEY,
    tracking_id TEXT NOT NULL,
    address TEXT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);
```

## Postgres

- Run the following command to start the postgres container

```bash
docker run -e POSTGRES_DB=final \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=postgres \
  -p 5432:5432 \
  -d postgres
```
