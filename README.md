# lxmin

Backup and restore LXC instances from MinIO

## Install

### Binary Releases

| OS       | ARCH    | Binary                                                                                       |
|:--------:|:-------:|:--------------------------------------------------------------------------------------------:|
| Linux    | amd64   | [linux-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-amd64)         |
| Linux    | arm64   | [linux-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-arm64)         |
| Linux    | ppc64le | [linux-ppc64le](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-ppc64le)     |
| Linux    | s390x   | [linux-s390x](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-s390x)         |
| Apple M1 | arm64   | [darwin-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-arm64)       |
| Apple    | amd64   | [darwin-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-amd64)       |
| Windows  | amd64   | [windows-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-windows-amd64.exe) |

### Setup

Enable MinIO endpoint, bucket and access credentials.

```sh
export LXMIN_ENDPOINT=http://localhost:9000
export LXMIN_BUCKET="backups"
export LXMIN_ACCESS_KEY="minioadmin"
export LXMIN_SECRET_KEY="minioadmin"
```

### Usage

```sh
lxmin --help

NAME:
  lxmin - backup and restore LXC instances from MinIO

USAGE:
  lxmin [FLAGS] COMMAND [COMMAND FLAGS | -h] [ARGUMENTS...]

COMMANDS:
  backup      backup an instance image to MinIO
  restore     restore an instance image from MinIO
  info        pretty print tags on an instance image on MinIO
  list, ls    list all backups from MinIO
  delete, rm  deletes a specific backup by 'name' for an instance from MinIO

GLOBAL FLAGS:
  --endpoint value    endpoint for S3 API call(s) [$LXMIN_ENDPOINT]
  --bucket value      bucket on MinIO to save/restore backup(s) [$LXMIN_BUCKET]
  --access-key value  access key credential for S3 API [$LXMIN_ACCESS_KEY]
  --secret-key value  secret key credential for S3 API [$LXMIN_SECRET_KEY]
  --help, -h          show help
```

### Create a backup

```sh
lxmin backup u2
Exporting backup for instance (u2) -> (backup_2022-02-17-03-1003.tar.gz): success
Uploading backup_2022-02-17-03-1003.tar.gz [======================================================================================] 95.70 MiB/s
```

### Create a backup (optimized supported for ZFS and BTRFS storage pools)

Creating optimized backups when possible is a good practice, allows for faster restore times for your backups during disaster recovery.

```sh
lxmin backup u2 --optimized
Exporting backup for instance (u2) -> (backup_2022-02-17-03-1003.tar.gz): success
Uploading backup_2022-02-17-03-1003.tar.gz [======================================================================================] 95.70 MiB/s
```

### List all backups

```sh
lxmin list
┌──────────┐┌──────────────────────────────────┐┌───────────────────────────────┐┌─────────┐┌───────────┐
│ Instance ││ Name                             ││ Created                       ││ Size    ││ Optimized │
│          ││                                  ││                               ││         ││           │
│ u2       ││ backup_2022-02-17-03-1003.tar.gz ││ Thu, 17 Feb 2022 03:11:19 GMT ││ 889 MiB ││ ✔         │
│ u2       ││ backup_2022-02-18-03-1003.tar.gz ││ Thu, 17 Feb 2022 03:11:19 GMT ││ 889 MiB ││ ✔         │
└──────────┘└──────────────────────────────────┘└───────────────────────────────┘└─────────┘└───────────┘
```

### Restore a backup

```sh
lxmin restore u2 backup_2022-02-17-03-1003.tar.gz
Downloading backup_2022-02-17-03-1003.tar.gz [======================================================================================================================] 209.10 MiB/s
Importing backup for instance (u2) <- (backup_2022-02-17-03-1003.tar.gz): success
Starting imported instance (u2): success
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

Delete all backups (dangerous operation)

```sh
lxmin delete u2 --all --force
All backups for u2 deleted successfully
```
