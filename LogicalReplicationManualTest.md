# Logical Replication – Manual Verification Steps

This document captures **all manual commands executed** to validate PostgreSQL **logical replication** for `pg-telemetry-lab` using Docker and Postgres 16.

---

## 1. Provision containers (via telemetryctl)

```bash
./telemetryctl provision local --config local.config.example.yaml
```

This resulted in the following containers:

* `pg-primary` (logical WAL enabled)
* `pg-replica-1`
* `pg-replica-2`

Primary started with:

```bash
docker run -d \
  --name pg-primary \
  --network pgnet \
  -e POSTGRES_USER=postgres \
  -e POSTGRES_PASSWORD=*** \
  -e POSTGRES_DB=pgbench \
  -p 5432:5432 \
  postgres:16 \
  postgres -c wal_level=logical -c max_wal_senders=10 -c max_replication_slots=10
```

Replicas started as normal Postgres containers:

```bash
docker run -d --name pg-replica-1 --network pgnet -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=*** -e POSTGRES_DB=pgbench -p 5540:5432 postgres:16
docker run -d --name pg-replica-2 --network pgnet -e POSTGRES_USER=postgres -e POSTGRES_PASSWORD=*** -e POSTGRES_DB=pgbench -p 5541:5432 postgres:16
```

---

## 2. Wait for Postgres readiness

```bash
docker exec pg-primary pg_isready -U postgres -d pgbench
docker exec pg-replica-1 pg_isready -U postgres -d pgbench
docker exec pg-replica-2 pg_isready -U postgres -d pgbench
```

---

## 3. Verify logical WAL on primary

```bash
docker exec pg-primary psql -U postgres -d pgbench -c "SHOW wal_level;"
```

Expected output:

```
 logical
```

---

## 4. Initialize pgbench schema on all nodes

Creates **tables, indexes, and sequences** (required for logical replication).

```bash
docker exec pg-primary   pgbench -i -s 10 -U postgres -d pgbench
docker exec pg-replica-1 pgbench -i -s 10 -U postgres -d pgbench
docker exec pg-replica-2 pgbench -i -s 10 -U postgres -d pgbench
```

---

## 5. Create replication user on primary

```bash
docker exec pg-primary psql -U postgres -d pgbench -c "CREATE ROLE replicator WITH LOGIN REPLICATION PASSWORD 'replicator_pw';"
docker exec pg-primary psql -U postgres -d pgbench -c "GRANT CONNECT ON DATABASE pgbench TO replicator;"
docker exec pg-primary psql -U postgres -d pgbench -c "GRANT USAGE ON SCHEMA public TO replicator;"
docker exec pg-primary psql -U postgres -d pgbench -c "GRANT SELECT ON ALL TABLES IN SCHEMA public TO replicator;"
docker exec pg-primary psql -U postgres -d pgbench -c "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO replicator;"
```

---

## 6. Create publication on primary

```bash
docker exec pg-primary psql -U postgres -d pgbench -c \
"CREATE PUBLICATION pgbench_pub FOR TABLE pgbench_accounts, pgbench_branches, pgbench_tellers, pgbench_history;"
```

---

## 7. Create subscriptions on replicas

```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
"CREATE SUBSCRIPTION pgbench_sub_1 CONNECTION 'host=pg-primary port=5432 dbname=pgbench user=replicator password=replicator_pw' PUBLICATION pgbench_pub;"

docker exec pg-replica-2 psql -U postgres -d pgbench -c \
"CREATE SUBSCRIPTION pgbench_sub_2 CONNECTION 'host=pg-primary port=5432 dbname=pgbench user=replicator password=replicator_pw' PUBLICATION pgbench_pub;"
```

---

## 8. Verify replication on primary

```bash
docker exec pg-primary psql -U postgres -d pgbench -c "SELECT slot_name, active FROM pg_replication_slots;"

docker exec pg-primary psql -U postgres -d pgbench -c "SELECT application_name, state FROM pg_stat_replication;"
```

---

## 9. Verify subscription workers on replicas (Postgres 16)

```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
"SELECT subname, pid, received_lsn, latest_end_lsn FROM pg_stat_subscription;"

docker exec pg-replica-2 psql -U postgres -d pgbench -c \
"SELECT subname, pid, received_lsn, latest_end_lsn FROM pg_stat_subscription;"
```

Observed LSN advancing:

* `0/94813E0` → `0/9481A70`

---

## 10. Check per-table replication state

```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
"SELECT srrelid::regclass AS table, srsubstate, srsublsn FROM pg_subscription_rel;"

docker exec pg-replica-2 psql -U postgres -d pgbench -c \
"SELECT srrelid::regclass AS table, srsubstate, srsublsn FROM pg_subscription_rel;"
```

Observed states:

* `d` = initial data copy in progress
* `r` = ready / streaming

---

## 11. Data replication proof

Insert on primary:

```bash
docker exec pg-primary psql -U postgres -d pgbench -c \
"INSERT INTO pgbench_history(tid,bid,aid,delta,mtime) VALUES (1,1,1,7,now());"
```

Read on replica:

```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c \
"SELECT delta, mtime FROM pgbench_history ORDER BY mtime DESC LIMIT 3;"
```

Confirmed: inserted row appears on replica.

---

## Result

✅ Logical replication is successfully configured and verified:

* WAL decoding enabled
* Publications and subscriptions active
* Initial table sync in progress
* Streaming replication confirmed via LSN movement and data visibility
