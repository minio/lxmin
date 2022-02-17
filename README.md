# lxmin

Backup and restore LXC instances with MinIO

## Install

### Binary Releases

| OS       | ARCH    | Binary                                                                                           |
|:--------:|:-------:|:------------------------------------------------------------------------------------------------:|
| Linux    | amd64   | [linux-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-amd64)         |
| Linux    | arm64   | [linux-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-arm64)         |
| Linux    | ppc64le | [linux-ppc64le](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-ppc64le)     |
| Linux    | s390x   | [linux-s390x](https://github.com/minio/lxmin/releases/latest/download/lxmin-linux-s390x)         |
| Apple M1 | arm64   | [darwin-arm64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-arm64)       |
| Apple    | amd64   | [darwin-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-darwin-amd64)       |
| Windows  | amd64   | [windows-amd64](https://github.com/minio/lxmin/releases/latest/download/lxmin-windows-amd64.exe) |

### Usage

```sh
lxmin --help

NAME:
  lxmin - backup and restore LXC instances with MinIO

USAGE:
  lxmin [FLAGS] COMMAND [COMMAND FLAGS | -h] [ARGUMENTS...]

COMMANDS:
  backup      backup an instance image to MinIO
  restore     restore an instance image from MinIO
  info        pretty print tags on an instance image on MinIO
  list, ls    list all backups from MinIO
  delete, rm  deletes a specific backup by 'name' for an instance from MinIO

GLOBAL FLAGS:
  --endpoint value    endpoint for MinIO server [$LXMIN_ENDPOINT]
  --bucket value      bucket to save/restore backup(s) [$LXMIN_BUCKET]
  --access-key value  access key credential [$LXMIN_ACCESS_KEY]
  --secret-key value  secret key credential [$LXMIN_SECRET_KEY]
  --help, -h          show help

ENVIRONMENT VARIABLES:
  LXMIN_ENDPOINT      endpoint for MinIO server
  LXMIN_BUCKET        bucket to save/restore backup(s)
  LXMIN_ACCESS_KEY    access key credential
  LXMIN_SECRET_KEY    secret key credential
```

### Create a backup

```sh
export LXMIN_ENDPOINT=http://147.75.71.77:9000
export LXMIN_BUCKET="backups"
export LXMIN_ACCESS_KEY="minioadmin"
export LXMIN_SECRET_KEY="minioadmin"

lxmin backup u2 --tags "OS=Ubuntu&Version=20.04&Build=10"
Preparing backup for (u2) instance: success
Uploading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 100.12 MiB/s
```

### Create an optimized backup

Optimize flag enables faster restore time. It is only supported for ZFS, BTRFS, RBD based storage pools.

```sh
lxmin backup u2 --optimize
Preparing backup for (u2) instance: success
Uploading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 100.12 MiB/s
```

### List all backups

```sh
lxmin list
┌──────────┐┌──────────────────────────────────┐┌─────────────────────────┐┌─────────┐┌───────────┐
│ Instance ││ Name                             ││ Created                 ││ Size    ││ Optimized │
│          ││                                  ││                         ││         ││           │
│ u2       ││ backup_2022-02-17-08-3732.tar.gz ││ 2022-02-17 08:38:47 UTC ││ 872 MiB ││ ✔         │
│ u2       ││ backup_2022-02-17-09-0524.tar.gz ││ 2022-02-17 09:06:39 UTC ││ 872 MiB ││ ✔         │
│ u2       ││ backup_2022-02-17-09-3329.tar.gz ││ 2022-02-17 09:34:44 UTC ││ 872 MiB ││ ✔         │
└──────────┘└──────────────────────────────────┘└─────────────────────────┘└─────────┘└───────────┘
```

### Restore a backup

```sh
lxmin restore u2 backup_2022-02-17-09-3329.tar.gz
Downloading backup_2022-02-17-09-3329.tar.gz ┃▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓▓┃ 209.10 MiB/s
Launching instance (u2) from backup: success
```

### Display backup info

```sh
Name      : backup_2022-02-17-09-3329.tar.gz
Date      : 2022-02-17 09:34:44 UTC
Size      : 872 MiB
Tags      :
  Version   : 20.04
  Build     : 10
  OS        : Ubuntu
Metadata  :
  Optimized : ✔
  Compressed: ✔
```

### Delete a backup

Delete a backup by name `backup_2022-02-16-04-1040.tar.gz`

```sh
lxmin delete u2 backup_2022-02-16-04-1040.tar.gz
Backup backup_2022-02-16-04-1040.tar.gz deleted successfully
```

Delete all backups (dangerous operation)

```sh
lxmin delete u2 --all --force
All backups for u2 deleted successfully
```
