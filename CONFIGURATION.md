# MySQL Manager Configuration Guide

## Overview

MySQL Manager provides extensive configuration options through environment variables and configuration settings. This guide covers all available configuration parameters, their usage, and common setup scenarios.

## Configuration Modes

MySQL Manager supports two primary operational modes:

1. **Standalone Mode**: Single MySQL instance
   - Ideal for single-server deployments
   - Simplified configuration
   - Local backup and management

2. **Cluster Mode**: Multi-node MySQL cluster with replication and high availability
   - Supports primary and replica nodes
   - Distributed backup and storage
   - Enhanced resilience and failover capabilities

## Configuration Principles

- Environment variables take precedence over default values
- Configuration is validated on startup
- Sensitive information should be managed securely (e.g., using secrets management)
- Dynamic reconfiguration is supported for many settings

## Environment Variable Configuration Categories

### 1. Global MySQL Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `MYSQL_MODE` | Operational mode | `standalone` | No | `standalone` or `cluster` |
| `MYSQL_ROLE` | Node role in cluster | `primary` | No | `primary` or `replica` |
| `MYSQL_ROOT_PASSWORD` | MySQL root password | None | Yes | `mysecretpassword` |
| `MYSQL_PORT` | MySQL server port | `3306` | No | `3307` |
| `MYSQL_MAX_CONNECTIONS` | Maximum simultaneous connections | `151` | No | `500` |
| `MYSQL_INNODB_BUFFER_POOL_SIZE` | InnoDB buffer pool size | System-dependent | No | `4G` |
| `MYSQL_QUERY_CACHE_SIZE` | Query cache size | `0` | No | `64M` |
| `MYSQL_SLOW_QUERY_LOG` | Enable slow query log | `0` | No | `1` |
| `MYSQL_PERFORMANCE_SCHEMA` | Enable Performance Schema | `1` | No | `0` |

### 2. Cluster Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `CLUSTER_NAME` | Unique cluster identifier | None | Yes (Cluster Mode) | `production-cluster` |
| `CLUSTER_SERVER_ID` | Unique server identifier | None | Yes (Cluster Mode) | `1` |
| `CLUSTER_JOIN_ADDR` | Primary node address for replica | None | Yes (Replica Nodes) | `10.0.0.1` |
| `CLUSTER_JOIN_TIMEOUT` | Timeout for cluster join | `5m` | No | `10m` |
| `CLUSTER_STATE_SYNC` | State synchronization interval | `10s` | No | `15s` |
| `CLUSTER_FAILOVER_DELAY` | Failover detection delay | `30s` | No | `45s` |
| `CLUSTER_REPLICATION_USER` | Replication user | `repl` | No | `replication_user` |
| `CLUSTER_NETWORK_TIMEOUT` | Network connection timeout | `60s` | No | `120s` |

### 3. Backup Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `BACKUP_ENABLED` | Enable backup functionality | `true` | No | `true` or `false` |
| `BACKUP_FULL_SCHEDULE` | Cron schedule for full backups | `0 0 * * *` | No | `0 2 * * *` (Daily at 2 AM) |
| `BACKUP_INCREMENTAL_SCHEDULE` | Cron schedule for incremental backups | `0 */6 * * *` | No | `0 */4 * * *` |
| `BACKUP_RETENTION_DAYS` | Days to retain backups | `7` | No | `30` |
| `BACKUP_COMPRESSION_LEVEL` | Backup compression (0-9) | `6` | No | `9` |
| `BACKUP_CONTINUOUS` | Enable real-time backup | `false` | No | `true` |
| `BACKUP_MAX_BANDWIDTH` | Backup bandwidth limit (bytes/sec) | `0` (Unlimited) | No | `1048576` (1MB/s) |
| `BACKUP_ENCRYPTION_ENABLED` | Enable backup encryption | `false` | No | `true` |
| `BACKUP_BINLOG_RETENTION_HOURS` | Binary log retention hours | `24` | No | `48` |

### 4. Storage Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `STORAGE_ENDPOINT` | S3-compatible storage endpoint | None | Yes (Cluster Mode) | `https://s3.amazonaws.com` |
| `STORAGE_BUCKET` | Storage bucket name | None | Yes (Cluster Mode) | `mysql-backups` |
| `STORAGE_ACCESS_KEY` | Storage access key | None | Yes (Cluster Mode) | `AKIAIOSFODNN7EXAMPLE` |
| `STORAGE_SECRET_KEY` | Storage secret key | None | Yes (Cluster Mode) | `wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` |
| `STORAGE_REGION` | Storage region | None | Yes (Cluster Mode) | `us-east-1` |
| `STORAGE_PREFIX` | Backup storage prefix/path | `backups/` | No | `production/backups/` |

### 5. Monitoring Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `METRICS_PORT` | Prometheus metrics port | `9100` | No | `9200` |
| `METRICS_USERNAME` | Metrics endpoint username | `admin` | No | `metrics-user` |
| `METRICS_PASSWORD` | Metrics endpoint password | Randomly generated | No | `secure-metrics-pass` |
| `METRICS_COLLECTION_INTERVAL` | Metrics collection interval | `15s` | No | `30s` |
| `METRICS_SLOW_QUERY_THRESHOLD` | Slow query threshold (ms) | `1000` | No | `500` |

### 6. API Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `API_PORT` | Management API port | `8080` | No | `8090` |
| `API_USERNAME` | API username | Randomly generated | No | `admin` |
| `API_PASSWORD` | API password | Randomly generated | No | `secure-api-pass` |
| `API_TLS_ENABLED` | Enable TLS for API | `false` | No | `true` |

### 7. Process Resilience Configuration

| Variable | Description | Default | Required | Example |
|----------|-------------|---------|----------|---------|
| `PROCESS_CHECK_INTERVAL` | Health check interval | `30s` | No | `15s` |
| `PROCESS_MEMORY_THRESHOLD` | Memory usage threshold (bytes) | `1GB` | No | `2147483648` |
| `PROCESS_RESTART_STRATEGY` | Restart strategy | `fixed` | No | `disabled` |
| `PROCESS_MAX_RESTARTS` | Maximum restart attempts | `5` | No | `3` |

## Security Recommendations

1. Use environment-specific configurations
2. Utilize secret management systems
3. Rotate credentials regularly
4. Implement network-level access controls
5. Enable encryption for sensitive communications

## Example Configurations

### Standalone Mode with Local Backups

