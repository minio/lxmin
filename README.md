# lxmin

Backup and restore LXC instances to object storage

## Usage

```sh
NAME:
  lxmin - backup and restore LXC instances to object storage

USAGE:
  lxmin COMMAND INSTANCENAME [FLAGS]

COMMAND:
  backup  backup an instance image to MinIO
  restore restore an instance from MinIO
  list    list all backups from MinIO 
  delete  deletes a specific backup by 'name' for an instance from MinIO

FLAGS:
  --endpoint value    endpoint for S3 API call(s) [$LXMIN_ENDPOINT]
  --bucket value      bucket on MinIO to save/restore backup(s) [$LXMIN_BUCKET]
  --access-key value  access key credential for S3 API [$LXMIN_ACCESS_KEY]
  --secret-key value  secret key credential for S3 API [$LXMIN_SECRET_KEY]
  --all               delete all backups for an instance, only valid for 'delete' command
  --force             allow all backups to be deleted, only valid when '--all' is specified
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
Exporting backup from lxc backup_20220215100258.tar.gz... Done
Uploading backup_20220215100258.tar.gz 652.80 MiB / 652.80 MiB 100.00% 103.70 MiB/s
```

### List all backups

```sh
lxmin list u2
INSTANCE        NAME                            CREATED                         SIZE 
u2              backup_20220215092457.tar.gz    Tue, 15 Feb 2022 09:26:18 GMT   652 MiB
u2              backup_20220215100258.tar.gz    Tue, 15 Feb 2022 10:04:22 GMT   653 MiB
```

### Restore a backup

```sh
lxmin restore u2 backup_20220215092457.tar.gz
Downloading backup_20220215092457.tar.gz 652.52 MiB / 652.52 MiB 100.00% 217.97 MiB/s
Importing instance u2 backup from backup_20220215092457.tar.gz... Done
Starting instance u2... Done
```

### Delete a backup

Delete a specific backup by name `backup_20220215092457.tar.gz`

```sh
lxmin delete u2 backup_20220215092457.tar.gz
Backup backup_20220215092457.tar.gz deleted successfully
```

Delete all backups

```sh
lxmin --all --force delete u2 
All backups for u2 deleted successfully
```
