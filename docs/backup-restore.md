# OVN Database Backup and Restore

This document describes how to back up and restore OVN Northbound and Southbound databases using the `OVNDBBackup` and `OVNDBRestore` custom resources.

## Backup

### Creating a Scheduled Backup

Create an `OVNDBBackup` CR to set up automated, periodic backups of an OVN database:

```yaml
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBBackup
metadata:
  name: ovndbbackup-nb
spec:
  databaseInstance: ovndbcluster-nb
  schedule: "@daily"
  storageRequest: "10G"
  retention: "168h"
```

| Field              | Required | Description |
|--------------------|----------|-------------|
| `databaseInstance` | Yes      | Name of the `OVNDBCluster` CR to back up (e.g. `ovndbcluster-nb` or `ovndbcluster-sb`). |
| `schedule`         | Yes      | Cron schedule expression (default: `@daily`). Examples: `@hourly`, `0 */6 * * *`. |
| `storageRequest`   | No       | Size of the backup PVC. Defaults to the `OVNDBCluster`'s `storageRequest`. |
| `storageClass`     | No       | Storage class for the backup PVC. Defaults to the `OVNDBCluster`'s `storageClass`. |
| `retention`        | No       | Duration after which old backups are deleted from disk (e.g. `168h` = 7 days). If unset, backups are kept indefinitely. |

### How It Works

The controller creates:

1. **A PersistentVolumeClaim** to store backup files. This PVC is intentionally *not* owned by the CR, so backup data survives if the `OVNDBBackup` resource is deleted.
2. **A CronJob** that runs on the specified schedule. Each job connects to the OVN database service endpoint and runs `ovsdb-client backup` to produce a standalone OVSDB file. If `retention` is set, old backup files are cleaned up afterward.

### Backing Up Both Databases

To back up both NB and SB databases, create two `OVNDBBackup` resources:

```yaml
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBBackup
metadata:
  name: ovndbbackup-nb
spec:
  databaseInstance: ovndbcluster-nb
  schedule: "@daily"
  storageRequest: "10G"
  retention: "168h"
---
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBBackup
metadata:
  name: ovndbbackup-sb
spec:
  databaseInstance: ovndbcluster-sb
  schedule: "@daily"
  storageRequest: "10G"
  retention: "168h"
```

### Triggering an On-Demand Backup

The CronJob runs on a schedule, but you can trigger an immediate backup at any time by creating a one-off Job from the CronJob:

```bash
CRONJOB_NAME=$(oc get cronjob -l app=ovndbbackup,ovndbbackup=ovndbbackup-nb -o name)
oc create job ovndbbackup-nb-manual --from="${CRONJOB_NAME}"
```

#### Using a Coordinated Timestamp

When performing a full environment backup (OVN + Galera + OADP), all components should use the same timestamp so that a restore can target a single consistent point in time. Set the `BACKUP_TIMESTAMP` environment variable to override the auto-generated timestamp:

```bash
BACKUP_TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# Trigger OVN NB backup
NB_CRONJOB=$(oc get cronjob -l app=ovndbbackup,ovndbbackup=ovndbbackup-nb -o name)
oc create job ovndbbackup-nb-${BACKUP_TIMESTAMP} --from="${NB_CRONJOB}"
oc set env job/ovndbbackup-nb-${BACKUP_TIMESTAMP} BACKUP_TIMESTAMP=${BACKUP_TIMESTAMP}

# Trigger OVN SB backup
SB_CRONJOB=$(oc get cronjob -l app=ovndbbackup,ovndbbackup=ovndbbackup-sb -o name)
oc create job ovndbbackup-sb-${BACKUP_TIMESTAMP} --from="${SB_CRONJOB}"
oc set env job/ovndbbackup-sb-${BACKUP_TIMESTAMP} BACKUP_TIMESTAMP=${BACKUP_TIMESTAMP}
```

The backup files will be named `<BACKUP_TIMESTAMP>-ovn<nb|sb>_db.backup`. If `BACKUP_TIMESTAMP` is not set, the job generates its own timestamp at runtime.

### Monitoring Backup Status

```bash
oc get ovndbbackup
oc describe ovndbbackup ovndbbackup-nb
```

The CR is `Ready` when the CronJob, PVC, and ConfigMap are all created successfully. To check recent backup job runs:

```bash
oc get jobs -l app=ovndbbackup
```

## Restore

> **Warning**: Restoring a database is a disruptive operation. The OVN DB cluster will be scaled down to zero during the process, causing a temporary loss of OVN control plane availability.

### Creating a Restore

Create an `OVNDBRestore` CR referencing the backup to restore from:

```yaml
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBRestore
metadata:
  name: ovndbrestore-nb
spec:
  backupSource: ovndbbackup-nb
```

To restore from a specific point in time, set `backupTimestamp` to the timestamp prefix of the backup file (format: `YYYYMMDD-HHMMSS`):

```yaml
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBRestore
metadata:
  name: ovndbrestore-nb
spec:
  backupSource: ovndbbackup-nb
  backupTimestamp: "20260422-120000"
```

| Field             | Required | Description |
|-------------------|----------|-------------|
| `backupSource`    | Yes      | Name of the `OVNDBBackup` CR to restore from. The backup must be in `Ready` state. |
| `backupTimestamp` | No       | Timestamp of the specific backup to restore (format: `YYYYMMDD-HHMMSS`). Must match the prefix of a backup file on the backup PVC. If omitted, the most recent backup is used. |

### Restore Phases

The restore proceeds through a state machine:

| Phase        | Description |
|--------------|-------------|
| `Validating` | Validates the backup source and saves the current replica count. |
| `ScalingDown`| Sets a restore annotation on the `OVNDBCluster` to override replicas to 0, force-deletes all pods (preStop hooks hang when all RAFT members terminate simultaneously), and deletes non-pod-0 PVCs to prevent stale RAFT state. |
| `Restoring`  | Runs a Job that copies a standalone backup onto pod-0's PVC. If `backupTimestamp` is set, the exact matching backup file is used; otherwise the most recent backup is selected. When pod-0 starts, `ovn-ctl` automatically converts the standalone file to a RAFT cluster. |
| `ScalingUp`  | Scales to 1 replica first (pod-0 bootstraps the restored DB), verifies the DB, then removes the restore annotation so the cluster scales to the original replica count. Remaining pods join the cluster with fresh PVCs. |
| `Completed`  | Restore finished successfully. |
| `Failed`     | Restore job failed. Check the job logs for details. |

### Monitoring Restore Progress

```bash
oc get ovndbrestore
oc describe ovndbrestore ovndbrestore-nb
```

The `Phase` field shows the current step. To check the restore job:

```bash
oc get jobs -l app=ovndbrestore
oc logs job/<restore-job-name>
```

### What Happens During Restore

1. A finalizer is added to the `OVNDBBackup` CR to prevent its deletion during the restore.
2. A restore-in-progress annotation is set on the `OVNDBCluster` to override the StatefulSet replica count to 0, preventing higher-level operators (e.g. OpenStackControlPlane) from interfering.
3. All pods are force-deleted (graceful RAFT shutdown hangs when all members terminate simultaneously). Non-pod-0 PVCs are deleted to prevent stale RAFT membership state on restart.
4. A Job mounts pod-0's PVC and the backup PVC, removes the old database file, and copies the selected backup in its place. When `backupTimestamp` is set, the job looks for a file named `<timestamp>-ovn<nb|sb>_db.backup`; otherwise it picks the most recent backup by modification time.
5. The annotation is updated to allow 1 replica. When pod-0 starts, `ovn-ctl` detects the standalone database file and automatically converts it to a RAFT cluster. After pod-0 is ready, the DB schema version is verified via `ovsdb-client get-schema-version`.
6. The annotation is removed, allowing the cluster to scale to its original replica count. The remaining pods start with fresh PVCs and join the RAFT cluster.
7. The finalizer on the `OVNDBBackup` CR is removed when the `OVNDBRestore` is deleted.

### Cleanup After Restore

The `OVNDBRestore` CR can be deleted after the restore completes. Deleting it removes the finalizer from the referenced `OVNDBBackup` CR:

```bash
oc delete ovndbrestore ovndbrestore-nb
```

## Coordinated Backup and Restore

When backing up an entire OpenStack control plane, all components (Galera, OVN, OADP) should share a single timestamp so that any restore targets a consistent point in time.

### Coordinated Backup

```bash
# Generate a shared timestamp
BACKUP_TIMESTAMP=$(date +%Y%m%d-%H%M%S)

# 1. Trigger Galera database dumps with the shared timestamp
#    (see dev-docs/backup-restore/user-guide.md, Step 2)

# 2. Trigger OVN database backups with the same timestamp
NB_CRONJOB=$(oc get cronjob -l app=ovndbbackup,ovndbbackup=ovndbbackup-nb -o name)
oc create job ovndbbackup-nb-${BACKUP_TIMESTAMP} --from="${NB_CRONJOB}"
oc set env job/ovndbbackup-nb-${BACKUP_TIMESTAMP} BACKUP_TIMESTAMP=${BACKUP_TIMESTAMP}

SB_CRONJOB=$(oc get cronjob -l app=ovndbbackup,ovndbbackup=ovndbbackup-sb -o name)
oc create job ovndbbackup-sb-${BACKUP_TIMESTAMP} --from="${SB_CRONJOB}"
oc set env job/ovndbbackup-sb-${BACKUP_TIMESTAMP} BACKUP_TIMESTAMP=${BACKUP_TIMESTAMP}

# Wait for OVN backup jobs to complete
oc wait --for=condition=complete job/ovndbbackup-nb-${BACKUP_TIMESTAMP} --timeout=120s
oc wait --for=condition=complete job/ovndbbackup-sb-${BACKUP_TIMESTAMP} --timeout=120s

# 3. Trigger OADP backups
#    (see dev-docs/backup-restore/user-guide.md, Steps 3-4)
```

### Coordinated Restore

Use the same `BACKUP_TIMESTAMP` from the backup step to restore all components from the same point in time:

```bash
BACKUP_TIMESTAMP=<timestamp-from-backup-step>

# Restore OVN NB
cat <<EOF | oc apply -f -
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBRestore
metadata:
  name: ovndbrestore-nb
spec:
  backupSource: ovndbbackup-nb
  backupTimestamp: "${BACKUP_TIMESTAMP}"
EOF

# Restore OVN SB
cat <<EOF | oc apply -f -
apiVersion: ovn.openstack.org/v1beta1
kind: OVNDBRestore
metadata:
  name: ovndbrestore-sb
spec:
  backupSource: ovndbbackup-sb
  backupTimestamp: "${BACKUP_TIMESTAMP}"
EOF

# Monitor progress
oc get ovndbrestore -w
```
