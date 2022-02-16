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

### List all backups

```sh
lxmin list u2
+----------+----------------------------------+-------------------------------+---------+
| INSTANCE |               NAME               |            CREATED            |  SIZE   |
+----------+----------------------------------+-------------------------------+---------+
| u2       | backup_2022-02-15-18-2642.tar.gz | Tue, 15 Feb 2022 18:28:04 GMT | 653 MiB |
| u2       | backup_2022-02-15-18-4259.tar.gz | Tue, 15 Feb 2022 18:44:21 GMT | 653 MiB |
| u2       | backup_2022-02-15-21-0930.tar.gz | Tue, 15 Feb 2022 21:10:53 GMT | 654 MiB |
| u2       | backup_2022-02-16-04-0841.tar.gz | Wed, 16 Feb 2022 04:10:05 GMT | 654 MiB |
| u2       | backup_2022-02-16-04-1040.tar.gz | Wed, 16 Feb 2022 04:12:04 GMT | 654 MiB |
+----------+----------------------------------+-------------------------------+---------+
```

### Restore a backup

```sh
lxmin restore u2 backup_2022-02-16-04-1040.tar.gz
Downloading backup_2022-02-16-04-1040.tar.gz [========================================================================================================================] 212.41 MiB/s
Importing instance 'u2', from 'backup_2022-02-16-04-1040.tar.gz'... Done
Starting instance 'u2'... Done
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
