# lxmin

Backup and restore LXC instances from MinIO

## Usage

```sh
NAME:
  lxmin - backup and restore LXC instances from MinIO

USAGE:
  lxmin [FLAGS] COMMAND [COMMAND FLAGS | -h] [ARGUMENTS...]

COMMANDS:
  backup      backup an instance image to MinIO
  restore     restore an instance image from MinIO
  list, ls    list all backups from MinIO
  delete, rm  deletes a specific backup by 'name' for an instance from MinIO

GLOBAL FLAGS:
  --endpoint value    endpoint for S3 API call(s) [$LXMIN_ENDPOINT]
  --bucket value      bucket on MinIO to save/restore backup(s) [$LXMIN_BUCKET]
  --access-key value  access key credential for S3 API [$LXMIN_ACCESS_KEY]
  --secret-key value  secret key credential for S3 API [$LXMIN_SECRET_KEY]
  --help, -h          show help
```

### Setup

Enable MinIO endpoint, bucket and access credentials.

```sh
export LXMIN_ENDPOINT=http://localhost:9000
export LXMIN_BUCKET="backups"
export LXMIN_ACCESS_KEY="minioadmin"
export LXMIN_SECRET_KEY="minioadmin"
```

### Create a backup

```sh
lxmin backup u2
Exporting backup from lxc backup_2022-02-16-04-1040.tar.gz... Done
Uploading backup_2022-02-16-04-1040.tar.gz [==========================================================================================================================] 101.25 MiB/s
```

### Create a backup (optimized supported for ZFS and BTRFS storage pools)

Creating optimized backups when possible is a good practice, allows for faster restore times for your backups during disaster recovery.

```sh
lxmin backup u2 --optimized
Exporting backup from lxc backup_2022-02-16-04-1040.tar.gz... Done
Uploading backup_2022-02-16-04-1040.tar.gz [==========================================================================================================================] 101.25 MiB/s
```

### List all backups

```sh
lxmin list
┌──────────┐┌──────────────────────────────────┐┌───────────────────────────────┐┌─────────┐┌───────────┐
│ Instance ││ Name                             ││ Created                       ││ Size    ││ Optimized │
│          ││                                  ││                               ││         ││           │
│ u2       ││ backup_2022-02-16-05-0102.tar.gz ││ Wed, 16 Feb 2022 05:01:56 GMT ││ 666 MiB ││ ✗         │
│ u2       ││ backup_2022-02-16-22-4725.tar.gz ││ Wed, 16 Feb 2022 22:49:07 GMT ││ 871 MiB ││ ✗         │
│ u2       ││ backup_2022-02-16-23-4340.tar.gz ││ Wed, 16 Feb 2022 23:44:55 GMT ││ 890 MiB ││ ✔         │
└──────────┘└──────────────────────────────────┘└───────────────────────────────┘└─────────┘└───────────┘
```

### Restore a backup

```sh
lxmin restore u2 backup_2022-02-16-04-1040.tar.gz
Downloading backup_2022-02-16-04-1040.tar.gz [========================================================================================================================] 212.41 MiB/s
Importing instance 'u2', from 'backup_2022-02-16-04-1040.tar.gz'... Done
Starting instance 'u2'... Done
```

### Pretty print any tags on a backup

```sh
lxmin info u2 backup_2022-02-16-22-4725.tar.gz
┌──────┐┌────────┐
│ Key  ││ Value  │
│      ││        │
│ key1 ││ value1 │
│ key2 ││ value2 │
│ key3 ││ value3 │
└──────┘└────────┘
```

### Delete a backup

Delete a specific backup by name `backup_2022-02-16-04-1040.tar.gz`

```sh
lxmin delete u2 backup_2022-02-16-04-1040.tar.gz
Backup backup_2022-02-16-04-1040.tar.gz deleted successfully
```

Delete all backups

```sh
lxmin delete u2 --all --force
All backups for u2 deleted successfully
```
