# Replication Guide

Complete guide to PostgreSQL logical replication setup, verification, and troubleshooting.

---

## Overview

This project uses **PostgreSQL logical replication** to replicate data from a primary database to multiple replica databases. Unlike physical (streaming) replication, logical replication allows:

- **Selective replication:** Choose specific tables to replicate
- **Different PostgreSQL versions:** Primary and replicas can run different versions
- **Granular control:** Per-table replication configuration
- **Bidirectional replication:** Possible with proper conflict resolution

---

## Architecture

### Logical Replication Components

```
┌─────────────────────────────────────────┐
│ Primary Database                        │
│                                         │
│  ┌──────────────────────────────────┐  │
│  │ Publication: pgbench_pub         │  │
│  │  • Tables: pgbench_accounts      │  │
│  │           pgbench_branches       │  │
│  │           pgbench_tellers        │  │
│  │           pgbench_history        │  │
│  └──────────────────────────────────┘  │
│                                         │
│  WAL Sender (logical decoding) ────────┼─┐
└─────────────────────────────────────────┘ │
                                            │
                  ┌─────────────────────────┤
                  │                         │
        ┌─────────▼──────────┐   ┌─────────▼──────────┐
        │ Replica 1          │   │ Replica 2          │
        │                    │   │                    │
        │  Subscription:     │   │  Subscription:     │
        │  pgbench_sub_r-1   │   │  pgbench_sub_r-2   │
        │                    │   │                    │
        │  Applies changes   │   │  Applies changes   │
        └────────────────────┘   └────────────────────┘
```

### Key Concepts

**Publication (Primary):**
- Defines which tables to replicate
- Created once on the primary
- Multiple subscriptions can subscribe to one publication

**Subscription (Replica):**
- Connects to a publication on the primary
- Pulls changes and applies them locally
- Each replica needs its own subscription

**Replication Slot:**
- Ensures WAL segments aren't deleted before replicas read them
- One slot per subscription
- Automatically created by subscriptions (if `create_slot: true`)

**LSN (Log Sequence Number):**
- Position in the WAL (Write-Ahead Log)
- Format: `segment/offset` (e.g., `0/75A2F3C8`)
- Used to track replication progress

---

## Setup Process

### Automated Setup

```bash
./telemetryctl replication setup
```

**What it does:**

1. **Provisions cluster** (if not running)
2. **Initializes schema on primary** (creates pgbench tables)
3. **Creates publication on primary**
4. **Initializes schema on replicas** (creates empty tables)
5. **Creates subscriptions on replicas**
6. **Waits for LSN sync** (ensures replicas catch up)
7. **Verifies replication** (inserts test data, checks replicas)

### Manual Setup (Step-by-Step)

For educational purposes or custom setup:

#### 1. Enable Logical Replication

**On primary:** (automatically configured in Docker container)
```sql
-- Check settings
SHOW wal_level;           -- Should be: logical
SHOW max_wal_senders;     -- Should be: 32
SHOW max_replication_slots; -- Should be: 32
```

#### 2. Create Publication

**On primary:**
```sql
CREATE PUBLICATION pgbench_pub FOR TABLE
  public.pgbench_accounts,
  public.pgbench_branches,
  public.pgbench_tellers,
  public.pgbench_history;
```

**Verify:**
```sql
SELECT * FROM pg_publication;
```

#### 3. Create Tables on Replicas

**On each replica:**
```bash
# Initialize pgbench schema (creates empty tables)
docker exec pgbench-runner pgbench \
  -h pg-replica-1 \
  -p 5432 \
  -U postgres \
  -d pgbench \
  -i --no-data
```

**Or manually:**
```sql
-- Create the same tables as on primary (structure only)
CREATE TABLE pgbench_accounts (...);
CREATE TABLE pgbench_branches (...);
-- etc.
```

#### 4. Create Subscription

**On each replica:**
```sql
CREATE SUBSCRIPTION pgbench_sub_replica-1
  CONNECTION 'host=pg-primary port=5432 dbname=pgbench user=postgres password=XXX'
  PUBLICATION pgbench_pub
  WITH (
    copy_data = false,    -- Don't copy existing data
    create_slot = true     -- Auto-create replication slot
  );
```

**Note:** `copy_data = false` because we already initialized empty tables. Set to `true` if you want initial data copy.

#### 5. Verify Replication

**On primary:**
```sql
-- Check replication connections
SELECT
  application_name,
  client_addr,
  state,
  sent_lsn,
  replay_lsn,
  pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes
FROM pg_stat_replication;
```

**On replica:**
```sql
-- Check subscription status
SELECT
  subname,
  subenabled,
  received_lsn,
  latest_end_lsn
FROM pg_stat_subscription;
```

---

## Configuration

### Config File: `configs/local.docker.yaml`

```yaml
replication:
  enabled: true
  publication_name: "pgbench_pub"           # Publication name on primary
  subscription_prefix: "pgbench_sub_"       # Subscription name prefix (+ replica label)

  tables:                                    # Tables to replicate
    - "public.pgbench_accounts"
    - "public.pgbench_branches"
    - "public.pgbench_tellers"
    - "public.pgbench_history"

  copy_data: false                          # Copy existing data on subscription
  create_slot: true                         # Auto-create replication slots

  verify:
    poll_interval: "500ms"                  # How often to check LSN sync
    timeout: "2m"                           # Max wait for sync
    strict_lsn_match: true                  # Require exact LSN match
```

### Options Explained

**copy_data:**
- `false`: Don't copy existing data (use if tables already initialized)
- `true`: Copy all existing data from primary to replica

**create_slot:**
- `true`: Subscription auto-creates replication slot (recommended)
- `false`: Manually create slot before subscription

**strict_lsn_match:**
- `true`: Wait until replica LSN exactly matches primary (safer)
- `false`: Accept close-enough LSN (faster, but might miss changes)

---

## Verification

### Check Replication Status

**Primary view:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  SELECT
    application_name,
    state,
    sync_state,
    pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) / 1024 AS lag_kb
  FROM pg_stat_replication
  ORDER BY application_name;
"
```

Expected output:
```
 application_name | state     | sync_state | lag_kb
------------------+-----------+------------+--------
 replica-1        | streaming | async      |      0
 replica-2        | streaming | async      |      0
```

**Replica view:**
```bash
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  SELECT
    subname,
    subenabled,
    received_lsn,
    latest_end_lsn
  FROM pg_stat_subscription;
"
```

Expected output:
```
      subname           | subenabled | received_lsn | latest_end_lsn
------------------------+------------+--------------+----------------
 pgbench_sub_replica-1  | t          | 0/75A2F3C8   | 0/75A2F3C8
```

### Test Data Replication

**Insert on primary:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  INSERT INTO pgbench_accounts (aid, bid, abalance)
  VALUES (999999, 1, 5000);
"
```

**Check on replica (wait 1 second):**
```bash
sleep 1
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  SELECT * FROM pgbench_accounts WHERE aid = 999999;
"
```

Expected: Same row appears on replica.

**Update on primary:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  UPDATE pgbench_accounts SET abalance = 6000 WHERE aid = 999999;
"
```

**Check on replica:**
```bash
sleep 1
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  SELECT abalance FROM pgbench_accounts WHERE aid = 999999;
"
```

Expected: `6000`

**Delete on primary:**
```bash
docker exec pg-primary psql -U postgres -d pgbench -c "
  DELETE FROM pgbench_accounts WHERE aid = 999999;
"
```

**Check on replica:**
```bash
sleep 1
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  SELECT COUNT(*) FROM pgbench_accounts WHERE aid = 999999;
"
```

Expected: `0` (row deleted)

---

## Monitoring

### LSN Tracking

**Current primary LSN:**
```sql
SELECT pg_current_wal_lsn();
-- Example: 0/75A2F3C8
```

**Replica received LSN:**
```sql
-- On replica
SELECT received_lsn FROM pg_stat_subscription WHERE subname = 'pgbench_sub_replica-1';
```

**Calculate lag:**
```sql
-- On primary
SELECT
  application_name,
  pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes
FROM pg_stat_replication;
```

### Continuous Monitoring

**Watch replication lag (updates every second):**
```bash
# On macOS/Linux with watch
watch -n 1 'docker exec pg-primary psql -U postgres -d pgbench -c "SELECT application_name, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes FROM pg_stat_replication"'

# Or with while loop (macOS without watch)
while true; do
  clear
  docker exec pg-primary psql -U postgres -d pgbench -c \
    "SELECT application_name, pg_wal_lsn_diff(pg_current_wal_lsn(), replay_lsn) AS lag_bytes FROM pg_stat_replication"
  sleep 1
done
```

**Use telemetry command:**
```bash
./telemetryctl telemetry collect --interval 1s --duration 5m --output lag-monitor.jsonl
```

---

## Troubleshooting

### Publication Doesn't Exist

**Symptom:** `ERROR: publication "pgbench_pub" does not exist`

**Solution:**
```sql
-- On primary
CREATE PUBLICATION pgbench_pub FOR TABLE
  public.pgbench_accounts,
  public.pgbench_branches,
  public.pgbench_tellers,
  public.pgbench_history;
```

### Subscription Already Exists

**Symptom:** `ERROR: subscription "pgbench_sub_replica-1" already exists`

**Solution:**
```sql
-- On replica
DROP SUBSCRIPTION pgbench_sub_replica-1;

-- Then recreate
CREATE SUBSCRIPTION pgbench_sub_replica-1 ...;
```

### Replication Slot Full

**Symptom:** WAL files accumulating, disk space running out

**Cause:** Replica disconnected, slot retaining WAL

**Solution:**
```sql
-- On primary: Check slots
SELECT slot_name, active, restart_lsn FROM pg_replication_slots;

-- Drop inactive slot
SELECT pg_drop_replication_slot('pgbench_sub_replica-1');

-- Recreate subscription on replica
DROP SUBSCRIPTION pgbench_sub_replica-1;
CREATE SUBSCRIPTION pgbench_sub_replica-1 ...;
```

### Tables Don't Exist on Replica

**Symptom:** `ERROR: relation "pgbench_accounts" does not exist`

**Cause:** Forgot to initialize schema on replica

**Solution:**
```bash
# Initialize pgbench schema on replica
docker exec pg-replica-1 psql -U postgres -d pgbench -c "
  -- Run pgbench -i or create tables manually
"
```

### Replication Not Progressing

**Symptom:** LSN on replica doesn't increase

**Checks:**
```sql
-- On primary: Check if WAL sender is running
SELECT * FROM pg_stat_replication;
-- Should show replica connected

-- On replica: Check if subscription is enabled
SELECT subname, subenabled FROM pg_subscription;
-- Should show 't' (true)

-- Check for errors
SELECT * FROM pg_stat_subscription;
-- Check 'last_msg_recv_time' is recent
```

**Solution:**
```sql
-- On replica: Restart subscription
ALTER SUBSCRIPTION pgbench_sub_replica-1 DISABLE;
ALTER SUBSCRIPTION pgbench_sub_replica-1 ENABLE;
```

### Lag Keeps Growing

**Symptom:** Lag in bytes continuously increases

**Causes:**
1. Replica CPU constrained (expected in testing)
2. Network issues
3. Long-running transactions on primary

**Solutions:**
```bash
# 1. Check replica resources
docker stats pg-replica-1

# 2. Increase replica CPU
# Edit configs/local.docker.yaml: replica_cpu: 1.0
./telemetryctl destroy local
./telemetryctl replication setup

# 3. Reduce write load
# Stop benchmark or reduce client count
```

### Wrong Password

**Symptom:** `FATAL: password authentication failed`

**Solution:**
```bash
# Make sure PG_PASSWORD is set
echo $PG_PASSWORD

# Or set it
export PG_PASSWORD=your-password

# Connection string in subscription must use correct password
```

---

## Advanced Topics

### Adding Tables to Replication

**Add table to publication:**
```sql
-- On primary
ALTER PUBLICATION pgbench_pub ADD TABLE public.new_table;
```

**Replicas automatically start receiving changes** (no subscription change needed).

### Removing Tables from Replication

**Remove table from publication:**
```sql
-- On primary
ALTER PUBLICATION pgbench_pub DROP TABLE public.pgbench_history;
```

### Sync vs Async Replication

**Default:** Async (primary doesn't wait for replica)

**Change to synchronous:**
```sql
-- On primary
ALTER SYSTEM SET synchronous_standby_names = 'replica-1,replica-2';
SELECT pg_reload_conf();
```

**Note:** Synchronous replication requires physical replication setup, not just logical.

### Conflict Resolution

If both primary and replica are writable (bidirectional replication), conflicts can occur.

**Conflict types:**
- INSERT: Same primary key
- UPDATE: Row doesn't exist or changed
- DELETE: Row doesn't exist

**Default behavior:** Last write wins (based on timestamp)

**View conflicts:**
```sql
-- On replica
SELECT * FROM pg_stat_subscription_stats;
```

---

## Best Practices

1. **Monitor lag:** Set up alerts for lag > 1s (warning) or > 5s (critical)
2. **Replication slots:** Always use `create_slot: true` to prevent WAL buildup
3. **copy_data:** Use `false` if tables are pre-populated, `true` for initial sync
4. **Resource allocation:** Give replicas sufficient CPU, especially during initialization
5. **Verification:** Always test replication after setup with manual insert/update/delete
6. **Cleanup:** Drop subscriptions before dropping publications
7. **Passwords:** Don't hardcode passwords in connection strings (use .env)

---

## Next Steps

- [Benchmarking Guide](BENCHMARKING.md) - Generate load to test replication
- [Telemetry Guide](TELEMETRY.md) - Monitor replication lag metrics
- [Command Reference](USAGE.md) - Replication setup command options
